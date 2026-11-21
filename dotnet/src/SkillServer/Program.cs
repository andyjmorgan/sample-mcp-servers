// skillserver speaks only MCP 2026-07-28 (stateless) and publishes Agent Skills over the Skills
// extension (SEP-2640, io.modelcontextprotocol/skills): skills/list, skills/get, and every skill
// file as a skill:// resource read with resources/read. The same catalogue is also reachable through
// two tools, list_skills and read_skill_file, for clients that only call tools.
//
// The Skills directory mirrors the URI space: Skills/anthropic/mcp-builder/SKILL.md is served at
// skill://anthropic/mcp-builder/SKILL.md. The catalogue covers the cases a gateway has to handle:
//   - a skill with no prefix (skillserver-guide) and skills under a prefix (anthropic/*)
//   - a nested skill (skillserver-guide/verify-digests), listed flat, its files in both manifests
//   - a binary file (anthropic/theme-factory/theme-showcase.pdf), returned as a blob
//   - a generated skill (clock) whose entry carries "resources": "dynamic"

using System.ComponentModel;
using System.Text.Json.Nodes;
using ModelContextProtocol;
using ModelContextProtocol.Extensions.Skills;
using ModelContextProtocol.Protocol;
using ModelContextProtocol.Server;
using SampleMcp.Hosting;
using SkillServer;

SampleHost.Run(Catalog.Spec, args);

namespace SkillServer
{
    public static class Catalog
    {
        public const string Scheme = "skill://";
        public static readonly string Root = Path.Combine(AppContext.BaseDirectory, "Skills");

        public const string ClockUri = "skill://clock/SKILL.md";
        public const string ClockDescription =
            "Report the current UTC time as rendered by the server. A generated skill whose content changes on every read.";

        // Skills whose files are wholly contained in an enclosing skill. Their files are already
        // registered as resources by the enclosing package, so only their entries are added.
        private static readonly string[] Nested = ["skillserver-guide/verify-digests"];

        /// <summary>Every directory under Skills that holds a SKILL.md, as a path relative to Skills.</summary>
        public static IEnumerable<string> SkillDirs() =>
            Directory.EnumerateFiles(Root, "SKILL.md", SearchOption.AllDirectories)
                .Select(f => Path.GetRelativePath(Root, Path.GetDirectoryName(f)!).Replace('\\', '/'))
                .Order(StringComparer.Ordinal);

        public static McpServerSkillPackage Package(string dir) =>
            McpServerSkillPackage.CreateFromDirectory(Path.Combine(Root, dir), new() { Uri = $"{Scheme}{dir}/SKILL.md" });

        public static string RenderClock()
        {
            var now = DateTimeOffset.UtcNow;
            return $"""
                ---
                name: clock
                description: {ClockDescription}
                ---

                # Clock

                This file was generated at {now:yyyy-MM-ddTHH:mm:ssZ} (Unix {now.ToUnixTimeSeconds()}).

                Tell the user the time above. It is the server's clock at the moment this SKILL.md was
                read, so it will differ on the next read. Because the content is generated, this
                skill's entry lists its resources as "dynamic" and offers no digest to verify against.

                """;
        }

        public static void Configure(IMcpServerBuilder mcp)
        {
            var dirs = SkillDirs().ToList();
            mcp.WithSkillPackages(dirs.Where(d => !Nested.Contains(d)).Select(Package));
            mcp.WithSkills(dirs.Where(d => Nested.Contains(d)).Select(d => Package(d).Skill));

            var clockEntry = new Skill
            {
                Uri = ClockUri,
                Frontmatter = new JsonObject { ["name"] = "clock", ["description"] = ClockDescription },
                Resources = SkillResources.Dynamic,
            };
            mcp.WithSkills([McpServerSkill.Create(clockEntry)]);
            mcp.WithResources([McpServerResource.Create(RenderClock, new McpServerResourceCreateOptions
            {
                UriTemplate = ClockUri, Name = "clock", Description = ClockDescription, MimeType = "text/markdown",
            })]);
            mcp.WithTools<SkillTools>();
        }

        public static readonly ServerSpec Spec = new("skillserver", "1.0.0",
            "Publishes Agent Skills over the MCP Skills extension (io.modelcontextprotocol/skills). Start with " +
            "skill://skillserver-guide/SKILL.md, which describes the catalogue. Clients with skills support can use " +
            "skills/list and skills/get, then read files with resources/read and verify each against the entry's sha256 " +
            "digest and size. Clients that only call tools can use list_skills for the same entries and read_skill_file " +
            "with a skill:// uri for any file.",
            Configure);
    }

    [McpServerToolType]
    public sealed class SkillTools
    {
        private static readonly Dictionary<string, string> Mime = new(StringComparer.OrdinalIgnoreCase)
        {
            [".md"] = "text/markdown", [".txt"] = "text/plain", [".py"] = "text/x-python",
            [".xml"] = "application/xml", [".pdf"] = "application/pdf",
        };

        public sealed record SkillSummary(string Uri, string Name, string Description, object Files);
        public sealed record FileEntry(string Uri, string Digest, long Size);
        public sealed record FileContent(string Uri, string MimeType, string? Text, string? Blob, long Size);

        [McpServerTool(Name = "list_skills", Title = "List Skills", ReadOnly = true, Destructive = false, Idempotent = true, OpenWorld = false)]
        [Description("List every skill this server publishes, with its frontmatter name and description and a manifest of its files (sha256 digest and size). Same entries as skills/list.")]
        public static List<SkillSummary> ListSkills()
        {
            var list = Catalog.SkillDirs().Select(d =>
            {
                var s = Catalog.Package(d).Skill.ProtocolSkill!;
                object files = s.Resources.Entries!.Select(r => new FileEntry(r.Uri, r.Digest, r.Size)).ToList();
                return new SkillSummary(s.Uri, s.Name!, s.Description!, files);
            }).ToList();
            list.Add(new SkillSummary(Catalog.ClockUri, "clock", Catalog.ClockDescription, "dynamic"));
            return list.OrderBy(s => s.Uri, StringComparer.Ordinal).ToList();
        }

        [McpServerTool(Name = "read_skill_file", Title = "Read Skill File", ReadOnly = true, Destructive = false, Idempotent = false, OpenWorld = false)]
        [Description("Read one file of a skill by its skill:// URI, e.g. skill://anthropic/mcp-builder/SKILL.md. Text files come back as text; binary files as base64 in blob. Same bytes as resources/read.")]
        public static FileContent ReadSkillFile([Description("A skill:// file URI from list_skills.")] string uri)
        {
            if (uri == Catalog.ClockUri)
            {
                var text = Catalog.RenderClock();
                return new FileContent(uri, "text/markdown", text, null, System.Text.Encoding.UTF8.GetByteCount(text));
            }
            if (!uri.StartsWith(Catalog.Scheme, StringComparison.Ordinal))
            {
                throw new McpException($"not a skill:// URI: {uri}");
            }
            var rel = uri[Catalog.Scheme.Length..];
            var full = Path.GetFullPath(Path.Combine(Catalog.Root, rel));
            if (!full.StartsWith(Catalog.Root + Path.DirectorySeparatorChar, StringComparison.Ordinal) || !File.Exists(full))
            {
                throw new McpException($"no file is served at {uri} (call list_skills for valid URIs)");
            }
            var bytes = File.ReadAllBytes(full);
            var mime = Mime.GetValueOrDefault(Path.GetExtension(full), "application/octet-stream");
            return mime == "application/pdf"
                ? new FileContent(uri, mime, null, Convert.ToBase64String(bytes), bytes.Length)
                : new FileContent(uri, mime, System.Text.Encoding.UTF8.GetString(bytes), null, bytes.Length);
        }
    }
}
