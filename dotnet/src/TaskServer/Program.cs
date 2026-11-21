// taskserver speaks only MCP 2026-07-28 (stateless) and supports the Tasks extension (SEP-2663,
// io.modelcontextprotocol/tasks). When a client opts in on a tools/call, the server answers at once
// with a task, runs the tool in the background, and the client polls tasks/get, answers input
// requests with tasks/update, or stops it with tasks/cancel.
//
// One tool per task outcome a gateway has to relay:
//   generate_report  completes after a delay (optional task)
//   count            completes after n steps, cancellable mid-run (optional task)
//   fail_job         ends in the failed state with a JSON-RPC error (optional task)
//   crash_job        completes with an isError tool result (optional task)
//   approve_job      parks in input_required until the client answers an elicitation (optional task)
//   batch_export     refuses to run unless the client opts in to tasks (required task)
//   add              never becomes a task (synchronous)
//
// Tasks live in one in-memory store shared by every request, so the deployment runs one replica.

using System.ComponentModel;
using ModelContextProtocol;
using ModelContextProtocol.Extensions.Tasks;
using ModelContextProtocol.Protocol;
using ModelContextProtocol.Server;
using SampleMcp.Hosting;
using TaskServer;

SampleHost.Run(TaskCatalog.Spec, args);

namespace TaskServer
{
    public static class TaskCatalog
    {
        public static void Configure(IMcpServerBuilder mcp)
        {
            var store = new InMemoryMcpTaskStore { DefaultPollIntervalMs = 1000, DefaultTimeToLive = TimeSpan.FromHours(1) };
            mcp.WithTools<TaskTools>().WithTasks(store, o => o.ExecutionModeSelector = ctx => ctx.Params?.Name switch
            {
                "add" => McpTaskExecutionMode.Synchronous,
                "batch_export" => McpTaskExecutionMode.Required,
                _ => McpTaskExecutionMode.Optional,
            });
        }

        public static readonly ServerSpec Spec = new("taskserver", "1.0.0",
            "Long-running tools that run as MCP tasks (io.modelcontextprotocol/tasks) when the client opts in. " +
            "generate_report and count finish after a delay; count can be cancelled with tasks/cancel. fail_job ends " +
            "failed, crash_job ends with an error result, and approve_job waits in input_required until you answer its " +
            "confirmation through tasks/update. batch_export only runs as a task. add always answers immediately. " +
            "Poll tasks/get at the pollIntervalMs the server returns. Task results are kept for one hour.",
            Configure);
    }

    [McpServerToolType]
    public sealed class TaskTools
    {
        [McpServerTool(Name = "generate_report", Title = "Generate Report", ReadOnly = true, Destructive = false, Idempotent = true, OpenWorld = false)]
        [Description("Write a short report on a topic after a simulated delay. Runs as a task when the client opts in.")]
        public static async Task<string> GenerateReport(
            [Description("Report topic.")] string topic,
            [Description("Seconds of simulated work, 1-60. Default 5.")] int seconds = 5,
            CancellationToken ct = default)
        {
            seconds = Math.Clamp(seconds, 1, 60);
            await Task.Delay(TimeSpan.FromSeconds(seconds), ct);
            return $"Report on {topic}: generated after {seconds}s at {DateTimeOffset.UtcNow:O}.";
        }

        [McpServerTool(Name = "count", Title = "Count", ReadOnly = true, Destructive = false, Idempotent = true, OpenWorld = false)]
        [Description("Count from 1 to n, pausing delay_ms between numbers. Cancel it mid-run with tasks/cancel to see the cancelled state.")]
        public static async Task<string> Count(
            [Description("Count to this number, 1-100.")] int n,
            [Description("Pause between numbers in milliseconds, 0-10000. Default 1000.")] int delay_ms = 1000,
            CancellationToken ct = default)
        {
            n = Math.Clamp(n, 1, 100);
            delay_ms = Math.Clamp(delay_ms, 0, 10000);
            for (var i = 1; i <= n; i++)
            {
                await Task.Delay(delay_ms, ct);
            }
            return string.Join(" ", Enumerable.Range(1, n));
        }

        [McpServerTool(Name = "fail_job", Title = "Fail Job", ReadOnly = true, Destructive = false, Idempotent = true, OpenWorld = false)]
        [Description("Work for a few seconds, then fail with a JSON-RPC error. As a task, it ends in the failed state.")]
        public static async Task<string> FailJob(
            [Description("Seconds before failing, 1-60. Default 3.")] int seconds = 3,
            CancellationToken ct = default)
        {
            await Task.Delay(TimeSpan.FromSeconds(Math.Clamp(seconds, 1, 60)), ct);
            throw new McpProtocolException("fail_job failed on purpose", McpErrorCode.InternalError);
        }

        [McpServerTool(Name = "crash_job", Title = "Crash Job", ReadOnly = true, Destructive = false, Idempotent = true, OpenWorld = false)]
        [Description("Work for a few seconds, then throw an ordinary exception. As a task, it completes with an isError tool result rather than failing.")]
        public static async Task<string> CrashJob(
            [Description("Seconds before crashing, 1-60. Default 3.")] int seconds = 3,
            CancellationToken ct = default)
        {
            await Task.Delay(TimeSpan.FromSeconds(Math.Clamp(seconds, 1, 60)), ct);
            throw new McpException("crash_job crashed on purpose");
        }

        [McpServerTool(Name = "approve_job", Title = "Approve Job", ReadOnly = true, Destructive = false, Idempotent = false, OpenWorld = false)]
        [Description("Ask the user to approve a job, then report the decision. As a task, it waits in input_required until the client answers the elicitation with tasks/update.")]
        public static async Task<string> ApproveJob(
            McpServer server,
            [Description("What the job does, shown in the approval prompt.")] string job,
            CancellationToken ct = default)
        {
            var answer = await server.ElicitAsync(new ElicitRequestParams
            {
                Message = $"Approve this job? {job}",
                RequestedSchema = new()
                {
                    Properties = new Dictionary<string, ElicitRequestParams.PrimitiveSchemaDefinition>
                    {
                        ["approve"] = new ElicitRequestParams.BooleanSchema { Title = "Approve", Description = "Run the job." },
                        ["note"] = new ElicitRequestParams.StringSchema { Title = "Note", Description = "Optional note for the log." },
                    },
                    Required = ["approve"],
                },
            }, ct);
            if (!answer.IsAccepted)
            {
                return $"Job not run: the user chose {answer.Action}.";
            }
            var approved = answer.Content?.TryGetValue("approve", out var a) == true && a.ValueKind == System.Text.Json.JsonValueKind.True;
            var note = answer.Content?.TryGetValue("note", out var n) == true ? n.GetString() : null;
            return (approved ? $"Job approved and run: {job}." : $"Job rejected: {job}.") + (string.IsNullOrEmpty(note) ? "" : $" Note: {note}");
        }

        [McpServerTool(Name = "batch_export", Title = "Batch Export", ReadOnly = true, Destructive = false, Idempotent = true, OpenWorld = false)]
        [Description("Export a batch of records. Only runs as a task: a call without the tasks opt-in is refused.")]
        public static async Task<string> BatchExport(
            [Description("Number of records, 1-10000. Default 1000.")] int records = 1000,
            CancellationToken ct = default)
        {
            records = Math.Clamp(records, 1, 10000);
            await Task.Delay(TimeSpan.FromMilliseconds(records), ct);
            return $"Exported {records} records.";
        }

        [McpServerTool(Name = "add", Title = "Add", ReadOnly = true, Destructive = false, Idempotent = true, OpenWorld = false)]
        [Description("Add two integers. Always answers immediately, even when the client opts in to tasks.")]
        public static long Add(long a, long b) => a + b;
    }
}
