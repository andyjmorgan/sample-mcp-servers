using Microsoft.AspNetCore.Builder;
using ModelContextProtocol.Client;
using SampleMcp.Hosting;

namespace SampleMcp.Tests;

/// <summary>Runs a sample server on a loopback port and connects the official client to it over HTTP.</summary>
public sealed class Harness : IAsyncDisposable
{
    private readonly WebApplication _app;
    public McpClient Client { get; }
    public string Endpoint { get; }

    private Harness(WebApplication app, McpClient client, string endpoint) => (_app, Client, Endpoint) = (app, client, endpoint);

    public static async Task<Harness> StartAsync(ServerSpec spec, McpClientHandlers? handlers = null)
    {
        var app = SampleHost.Build(spec, new HostConfig("0", "http://127.0.0.1", null, null, null));
        app.Urls.Add("http://127.0.0.1:0");
        await app.StartAsync();
        var endpoint = app.Urls.First() + "/mcp";
        var client = await McpClient.CreateAsync(
            new HttpClientTransport(new HttpClientTransportOptions { Endpoint = new Uri(endpoint) }),
            new McpClientOptions { Handlers = handlers ?? new McpClientHandlers() });
        return new Harness(app, client, endpoint);
    }

    public async ValueTask DisposeAsync()
    {
        await Client.DisposeAsync();
        await _app.StopAsync();
        await _app.DisposeAsync();
    }
}
