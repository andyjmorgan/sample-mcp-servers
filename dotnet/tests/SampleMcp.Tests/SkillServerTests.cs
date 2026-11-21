using ModelContextProtocol;
using ModelContextProtocol.Extensions.Skills;
using ModelContextProtocol.Protocol;
using SkillServer;
using Xunit;

namespace SampleMcp.Tests;

public class SkillServerTests
{
    [Fact]
    public async Task AdvertisesSkillsAndResources()
    {
        await using var h = await Harness.StartAsync(Catalog.Spec);
        Assert.True(h.Client.ServerCapabilities.SupportsSkills());
        Assert.NotNull(h.Client.ServerCapabilities.Resources);
        Assert.Equal("2026-07-28", h.Client.NegotiatedProtocolVersion);
    }

    [Fact]
    public async Task ListsTheCatalogue()
    {
        await using var h = await Harness.StartAsync(Catalog.Spec);
        var uris = (await h.Client.ListSkillsAsync()).Select(s => s.Uri).ToList();
        Assert.Equal(
        [
            "skill://anthropic/brand-guidelines/SKILL.md",
            "skill://anthropic/internal-comms/SKILL.md",
            "skill://anthropic/mcp-builder/SKILL.md",
            "skill://anthropic/theme-factory/SKILL.md",
            "skill://anthropic/webapp-testing/SKILL.md",
            "skill://clock/SKILL.md",
            "skill://skillserver-guide/SKILL.md",
            "skill://skillserver-guide/verify-digests/SKILL.md",
        ], uris.Order(StringComparer.Ordinal));
    }

    [Fact]
    public async Task EveryListedFileMatchesItsDigest()
    {
        await using var h = await Harness.StartAsync(Catalog.Spec);
        var checkedFiles = 0;
        foreach (var skill in await h.Client.ListSkillsAsync())
        {
            if (skill.Resources.IsDynamic) continue;
            foreach (var file in skill.Resources.Entries!)
            {
                var read = await h.Client.ReadResourceAsync(file.Uri);
                SkillVerifier.Verify(skill, file.Uri, read); // throws on a size or digest mismatch
                checkedFiles++;
            }
        }
        Assert.True(checkedFiles > 40, $"only {checkedFiles} files verified");
    }

    [Fact]
    public async Task PdfComesBackAsBlob()
    {
        await using var h = await Harness.StartAsync(Catalog.Spec);
        var read = await h.Client.ReadResourceAsync("skill://anthropic/theme-factory/theme-showcase.pdf");
        var blob = Assert.IsType<BlobResourceContents>(Assert.Single(read.Contents));
        Assert.Equal("application/pdf", blob.MimeType);
    }

    [Fact]
    public async Task NestedSkillFilesAppearInBothManifests()
    {
        await using var h = await Harness.StartAsync(Catalog.Spec);
        var outer = await h.Client.GetSkillAsync("skill://skillserver-guide/SKILL.md");
        var inner = await h.Client.GetSkillAsync("skill://skillserver-guide/verify-digests/SKILL.md");
        Assert.Equal("verify-digests", inner.Name);
        Assert.Contains(outer.Resources.Entries!, r => r.Uri == inner.Uri);
    }

    [Fact]
    public async Task ClockIsDynamicAndChanges()
    {
        await using var h = await Harness.StartAsync(Catalog.Spec);
        var clock = await h.Client.GetSkillAsync(Catalog.ClockUri);
        Assert.True(clock.Resources.IsDynamic);
        var text = Assert.IsType<TextResourceContents>(Assert.Single((await h.Client.ReadResourceAsync(Catalog.ClockUri)).Contents)).Text;
        Assert.StartsWith("---\nname: clock\n", text);
    }

    [Fact]
    public async Task UnknownSkillIsInvalidParams()
    {
        await using var h = await Harness.StartAsync(Catalog.Spec);
        var ex = await Assert.ThrowsAsync<McpProtocolException>(() => h.Client.GetSkillAsync("skill://nope/SKILL.md").AsTask());
        Assert.Equal(McpErrorCode.InvalidParams, ex.ErrorCode);
    }

    [Fact]
    public async Task ToolSurfaceMatches()
    {
        await using var h = await Harness.StartAsync(Catalog.Spec);
        var list = await h.Client.CallToolAsync("list_skills");
        Assert.NotEqual(true, list.IsError);
        var file = await h.Client.CallToolAsync("read_skill_file", new Dictionary<string, object?> { ["uri"] = "skill://anthropic/mcp-builder/SKILL.md" });
        Assert.NotEqual(true, file.IsError);
        Assert.Contains("name: mcp-builder", ((TextContentBlock)file.Content[0]).Text);
        var bad = await h.Client.CallToolAsync("read_skill_file", new Dictionary<string, object?> { ["uri"] = "skill://../../etc/passwd" });
        Assert.True(bad.IsError);
    }
}
