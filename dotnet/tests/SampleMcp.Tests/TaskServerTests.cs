using System.Text.Json;
using ModelContextProtocol;
using ModelContextProtocol.Client;
using ModelContextProtocol.Extensions.Tasks;
using ModelContextProtocol.Protocol;
using TaskServer;
using Xunit;

namespace SampleMcp.Tests;

public class TaskServerTests
{
    private static CallToolRequestParams Call(string name, object args) => new()
    {
        Name = name,
        Arguments = JsonSerializer.SerializeToElement(args).EnumerateObject().ToDictionary(p => p.Name, p => p.Value),
    };

    private static async Task<GetTaskResult> WaitFor(McpClient c, string taskId, Func<GetTaskResult, bool> done)
    {
        var deadline = DateTime.UtcNow.AddSeconds(30);
        while (DateTime.UtcNow < deadline)
        {
            var t = await c.GetTaskAsync(taskId);
            if (done(t)) return t;
            await Task.Delay(200);
        }
        throw new TimeoutException($"task {taskId} did not reach the expected state");
    }

    private static bool Terminal(GetTaskResult t) => t is CompletedTaskResult or FailedTaskResult or CancelledTaskResult;

    [Fact]
    public async Task ReportCompletesAsATask()
    {
        await using var h = await Harness.StartAsync(TaskCatalog.Spec);
        var r = await h.Client.CallToolAsTaskAsync(Call("generate_report", new { topic = "gateways", seconds = 1 }));
        Assert.True(r.IsTask);
        var done = Assert.IsType<CompletedTaskResult>(await WaitFor(h.Client, r.TaskCreated!.TaskId, Terminal));
        var result = done.Result.Deserialize<CallToolResult>()!;
        Assert.Contains("Report on gateways", ((TextContentBlock)result.Content[0]).Text);
    }

    [Fact]
    public async Task CountCanBeCancelled()
    {
        await using var h = await Harness.StartAsync(TaskCatalog.Spec);
        var r = await h.Client.CallToolAsTaskAsync(Call("count", new { n = 100, delay_ms = 1000 }));
        await h.Client.CancelTaskAsync(r.TaskCreated!.TaskId);
        Assert.IsType<CancelledTaskResult>(await WaitFor(h.Client, r.TaskCreated.TaskId, Terminal));
    }

    [Fact]
    public async Task FailJobEndsFailed()
    {
        await using var h = await Harness.StartAsync(TaskCatalog.Spec);
        var r = await h.Client.CallToolAsTaskAsync(Call("fail_job", new { seconds = 1 }));
        var failed = Assert.IsType<FailedTaskResult>(await WaitFor(h.Client, r.TaskCreated!.TaskId, Terminal));
        Assert.Contains("on purpose", failed.Error.GetRawText());
    }

    [Fact]
    public async Task CrashJobCompletesWithErrorResult()
    {
        await using var h = await Harness.StartAsync(TaskCatalog.Spec);
        var r = await h.Client.CallToolAsTaskAsync(Call("crash_job", new { seconds = 1 }));
        var done = Assert.IsType<CompletedTaskResult>(await WaitFor(h.Client, r.TaskCreated!.TaskId, Terminal));
        Assert.True(done.Result.Deserialize<CallToolResult>()!.IsError);
    }

    [Fact]
    public async Task ApproveJobWaitsForInput()
    {
        await using var h = await Harness.StartAsync(TaskCatalog.Spec);
        var r = await h.Client.CallToolAsTaskAsync(Call("approve_job", new { job = "rotate keys" }));
        var waiting = Assert.IsType<InputRequiredTaskResult>(
            await WaitFor(h.Client, r.TaskCreated!.TaskId, t => t is InputRequiredTaskResult || Terminal(t)));
        var (key, request) = Assert.Single(waiting.InputRequests!);
        Assert.Contains("rotate keys", request.ElicitationParams!.Message);

        await h.Client.UpdateTaskAsync(new UpdateTaskRequestParams
        {
            TaskId = r.TaskCreated.TaskId,
            InputResponses = new Dictionary<string, InputResponse>
            {
                [key] = InputResponse.FromElicitResult(new ElicitResult
                {
                    Action = "accept",
                    Content = new Dictionary<string, JsonElement> { ["approve"] = JsonSerializer.SerializeToElement(true) },
                }),
            },
        });
        var done = Assert.IsType<CompletedTaskResult>(await WaitFor(h.Client, r.TaskCreated.TaskId, Terminal));
        Assert.Contains("approved", ((TextContentBlock)done.Result.Deserialize<CallToolResult>()!.Content[0]).Text);
    }

    [Fact]
    public async Task AddNeverBecomesATask()
    {
        await using var h = await Harness.StartAsync(TaskCatalog.Spec);
        var r = await h.Client.CallToolAsTaskAsync(Call("add", new { a = 2, b = 3 }));
        Assert.False(r.IsTask);
        Assert.Contains("5", ((TextContentBlock)r.Result!.Content[0]).Text);
    }

    [Fact]
    public async Task BatchExportRequiresTasks()
    {
        await using var h = await Harness.StartAsync(TaskCatalog.Spec);
        var r = await h.Client.CallToolAsTaskAsync(Call("batch_export", new { records = 10 }));
        Assert.True(r.IsTask);
        // Without the opt-in the server refuses with -32021 (missing client capability). The 2.4.0
        // client surfaces that HTTP 400 as an HttpRequestException carrying the JSON-RPC error.
        var ex = await Assert.ThrowsAnyAsync<Exception>(() =>
            h.Client.CallToolAsync("batch_export", new Dictionary<string, object?> { ["records"] = 10 }).AsTask());
        Assert.Contains("-32021", ex.ToString());
    }
}
