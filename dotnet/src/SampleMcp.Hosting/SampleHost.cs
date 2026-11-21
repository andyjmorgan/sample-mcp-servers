using System.Diagnostics;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Microsoft.AspNetCore.Authentication;
using Microsoft.AspNetCore.Authentication.JwtBearer;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;
using Microsoft.IdentityModel.Tokens;
using ModelContextProtocol.Protocol;

namespace SampleMcp.Hosting;

/// <summary>Describes one sample server. Mirrors mcpserve.Spec in the Go servers.</summary>
public sealed record ServerSpec(string Name, string Version, string Instructions, Action<IMcpServerBuilder> Configure)
{
    /// <summary>The only protocol revision the server accepts.</summary>
    public string ProtocolVersion { get; init; } = "2026-07-28";
}

/// <summary>
/// Environment configuration, the same variables the Go servers read.
/// </summary>
/// <remarks>
/// PORT (default 8080), PUBLIC_URL, OAUTH_ISSUER, OAUTH_SCOPE and API_KEY.
/// When OAUTH_ISSUER is unset, OAuth is off (local development).
/// </remarks>
public sealed record HostConfig(string Port, string PublicUrl, string? Issuer, string? Scope, string? ApiKey)
{
    public string Resource => PublicUrl + "/mcp";
    public string ResourceMetadataUrl => PublicUrl + "/.well-known/oauth-protected-resource/mcp";

    public static HostConfig FromEnvironment()
    {
        static string? Env(string k) => Environment.GetEnvironmentVariable(k) is { Length: > 0 } v ? v : null;
        var port = Env("PORT") ?? "8080";
        var publicUrl = (Env("PUBLIC_URL") ?? $"http://localhost:{port}").TrimEnd('/');
        return new HostConfig(port, publicUrl, Env("OAUTH_ISSUER"), Env("OAUTH_SCOPE"), Env("API_KEY"));
    }
}

public static class SampleHost
{
    public static void Run(ServerSpec spec, string[] args)
    {
        var config = HostConfig.FromEnvironment();
        var app = Build(spec, config, args);
        app.Urls.Add($"http://0.0.0.0:{config.Port}");
        RequestLog.Write(spec.Name, "listening", new()
        {
            ["port"] = config.Port, ["protocol"] = spec.ProtocolVersion, ["resource"] = config.Resource,
            ["oauth"] = config.Issuer is not null, ["scope"] = config.Scope, ["apiKey"] = config.ApiKey is not null,
        });
        app.Run();
    }

    /// <summary>Builds the app without binding a port, so tests can choose their own.</summary>
    public static WebApplication Build(ServerSpec spec, HostConfig config, string[]? args = null)
    {
        var builder = WebApplication.CreateBuilder(args ?? []);
        builder.Logging.ClearProviders();
        builder.Logging.AddJsonConsole();
        builder.Logging.SetMinimumLevel(LogLevel.Warning);
        // JwtBearer pulls in DataProtection, which warns about ephemeral keys at startup. Nothing
        // here protects data with it, so the warning is noise.
        builder.Logging.AddFilter("Microsoft.AspNetCore.DataProtection", LogLevel.Error);

        var mcp = builder.Services.AddMcpServer(o =>
        {
            o.ServerInfo = new Implementation { Name = spec.Name, Version = spec.Version };
            o.ServerInstructions = spec.Instructions;
            o.ProtocolVersion = spec.ProtocolVersion;
        }).WithHttpTransport();
        spec.Configure(mcp);

        if (config.Issuer is not null)
        {
            builder.Services.AddAuthentication(JwtBearerDefaults.AuthenticationScheme).AddJwtBearer(o =>
            {
                o.Authority = config.Issuer;
                o.MapInboundClaims = false;
                o.TokenValidationParameters = new TokenValidationParameters
                {
                    ValidIssuer = config.Issuer,
                    ValidAudience = config.Resource, // RFC 8707: the token must name this server
                };
            });
            builder.Services.AddAuthorization();
        }

        var app = builder.Build();
        app.Use(RequestLog.Middleware(spec.Name));
        app.MapGet("/healthz", () => "ok");

        if (config.Issuer is not null)
        {
            var prm = new Dictionary<string, object?>
            {
                ["resource"] = config.Resource,
                ["authorization_servers"] = new[] { config.Issuer },
                ["scopes_supported"] = config.Scope is null ? null : new[] { config.Scope },
                ["bearer_methods_supported"] = new[] { "header" },
                ["resource_name"] = spec.Name,
            };
            // RFC 9728 path-aware location first, root location for older clients.
            app.MapGet("/.well-known/oauth-protected-resource/mcp", () => Results.Json(prm));
            app.MapGet("/.well-known/oauth-protected-resource", () => Results.Json(prm));
            app.UseAuthentication();
            app.UseWhen(c => c.Request.Path.StartsWithSegments("/mcp"), a => a.Use(RequireBearer(config)));
        }
        if (config.ApiKey is not null)
        {
            app.UseWhen(c => c.Request.Path.StartsWithSegments("/mcp"), a => a.Use(RequireApiKey(config.ApiKey)));
        }

        app.MapMcp("/mcp");
        return app;
    }

    // Same challenge shape as the go-sdk's auth.RequireBearerToken: 401 with resource_metadata and
    // scope when the token is missing or invalid, 403 insufficient_scope when the scope is missing.
    private static Func<HttpContext, RequestDelegate, Task> RequireBearer(HostConfig config) => async (ctx, next) =>
    {
        var result = await ctx.AuthenticateAsync(JwtBearerDefaults.AuthenticationScheme);
        var challenge = $"Bearer resource_metadata=\"{config.ResourceMetadataUrl}\"" +
            (config.Scope is null ? "" : $", scope=\"{config.Scope}\"");
        if (!result.Succeeded)
        {
            ctx.Response.StatusCode = StatusCodes.Status401Unauthorized;
            ctx.Response.Headers.WWWAuthenticate = challenge;
            await ctx.Response.WriteAsync("missing or invalid bearer token");
            return;
        }
        var scopes = (result.Principal.FindFirst("scope")?.Value ?? "").Split(' ', StringSplitOptions.RemoveEmptyEntries);
        if (config.Scope is not null && !scopes.Contains(config.Scope))
        {
            ctx.Response.StatusCode = StatusCodes.Status403Forbidden;
            ctx.Response.Headers.WWWAuthenticate = challenge + ", error=\"insufficient_scope\"";
            await ctx.Response.WriteAsync("insufficient scope");
            return;
        }
        ctx.User = result.Principal;
        await next(ctx);
    };

    private static Func<HttpContext, RequestDelegate, Task> RequireApiKey(string key) => async (ctx, next) =>
    {
        var got = Encoding.UTF8.GetBytes(ctx.Request.Headers["X-Api-Key"].ToString());
        if (!CryptographicOperations.FixedTimeEquals(got, Encoding.UTF8.GetBytes(key)))
        {
            ctx.Response.StatusCode = StatusCodes.Status401Unauthorized;
            await ctx.Response.WriteAsync("missing or invalid X-Api-Key");
            return;
        }
        await next(ctx);
    };
}

/// <summary>One JSON line per request, in the same shape as the Go servers' slog output.</summary>
internal static class RequestLog
{
    private static readonly object Gate = new();

    public static void Write(string server, string msg, Dictionary<string, object?> attrs)
    {
        var line = new Dictionary<string, object?>
        {
            ["time"] = DateTime.UtcNow.ToString("O"), ["level"] = "INFO", ["msg"] = msg, ["server"] = server,
        };
        foreach (var (k, v) in attrs) line[k] = v;
        var json = JsonSerializer.Serialize(line);
        lock (Gate) Console.Out.WriteLine(json);
    }

    public static Func<HttpContext, RequestDelegate, Task> Middleware(string server) => async (ctx, next) =>
    {
        if (ctx.Request.Path == "/healthz")
        {
            await next(ctx);
            return;
        }
        var sw = Stopwatch.StartNew();
        try
        {
            await next(ctx);
        }
        finally
        {
            var attrs = new Dictionary<string, object?>
            {
                ["method"] = ctx.Request.Method, ["path"] = ctx.Request.Path.Value, ["status"] = ctx.Response.StatusCode,
                ["ms"] = sw.ElapsedMilliseconds, ["mcpProtocol"] = ctx.Request.Headers["Mcp-Protocol-Version"].ToString(),
            };
            if (ctx.User.FindFirst("sub") is { } sub) attrs["sub"] = sub.Value;
            if (ctx.User.FindFirst("azp") is { } azp) attrs["azp"] = azp.Value;
            Write(server, "request", attrs);
        }
    };
}
