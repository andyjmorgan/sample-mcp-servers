using System.Text.Json;
using ElicitationServer;
using ModelContextProtocol.Client;
using ModelContextProtocol.Protocol;
using Xunit;

namespace SampleMcp.Tests;

public class ElicitationServerTests
{
    /// <summary>A client that answers each form from a script and records what it was asked.</summary>
    private sealed class ScriptedUser
    {
        public readonly List<ElicitRequestParams> Forms = [];
        private readonly Queue<Func<ElicitRequestParams, ElicitResult>> _answers;

        public ScriptedUser(params Func<ElicitRequestParams, ElicitResult>[] answers) => _answers = new(answers);

        public McpClientHandlers Handlers => new()
        {
            ElicitationHandler = (p, _) =>
            {
                Forms.Add(p!);
                return ValueTask.FromResult(_answers.Dequeue()(p!));
            },
        };
    }

    private static ElicitResult Accept(object content) => new()
    {
        Action = "accept",
        Content = JsonSerializer.SerializeToElement(content).EnumerateObject().ToDictionary(p => p.Name, p => p.Value),
    };

    private static Dictionary<string, object?> Args(object a) =>
        JsonSerializer.SerializeToElement(a).EnumerateObject().ToDictionary(p => p.Name, p => (object?)p.Value);

    private static string Text(CallToolResult r) => ((TextContentBlock)r.Content[0]).Text;

    [Fact]
    public async Task CompleteOrderNeedsNoForm()
    {
        var user = new ScriptedUser();
        await using var h = await Harness.StartAsync(PizzaShop.Spec, user.Handlers);
        var r = await h.Client.CallToolAsync("order_pizza", Args(new
        {
            size = "Large", crust = "thin", toppings = new[] { "pepperoni" }, quantity = 2, name = "Ada", fulfillment = "pickup",
        }));
        Assert.Empty(user.Forms);
        Assert.Contains("2 pizzas, large, thin crust, pepperoni, for pickup, for Ada", Text(r));
        Assert.Contains("$33.00", Text(r));
    }

    [Fact]
    public async Task FormAsksOnlyForWhatIsMissing()
    {
        var user = new ScriptedUser(_ => Accept(new { crust = "classic", quantity = 1, name = "Grace" }));
        await using var h = await Harness.StartAsync(PizzaShop.Spec, user.Handlers);
        var r = await h.Client.CallToolAsync("order_pizza", Args(new
        {
            size = "medium", toppings = Array.Empty<string>(), fulfillment = "delivery", address = "1 Main St",
        }));
        var form = Assert.Single(user.Forms);
        Assert.Equal(["crust", "quantity", "name"], form.RequestedSchema!.Properties.Keys);
        Assert.Contains("1 pizza, medium, classic crust, cheese, delivered to 1 Main St, for Grace", Text(r));
    }

    [Fact]
    public async Task DeliveryAddressTakesASecondRound()
    {
        var user = new ScriptedUser(
            _ => Accept(new { size = "small", crust = "thin", toppings = new[] { "olives" }, quantity = 1, name = "Lin", fulfillment = "delivery" }),
            _ => Accept(new { address = "9 Elm Rd" }));
        await using var h = await Harness.StartAsync(PizzaShop.Spec, user.Handlers);
        var r = await h.Client.CallToolAsync("order_pizza");
        Assert.Equal(2, user.Forms.Count);
        Assert.Contains("address", user.Forms[0].RequestedSchema!.Properties.Keys);
        Assert.DoesNotContain("address", user.Forms[0].RequestedSchema!.Required ?? []);
        Assert.Equal(["address"], user.Forms[1].RequestedSchema!.Properties.Keys);
        Assert.Equal(["address"], user.Forms[1].RequestedSchema!.Required);
        Assert.Contains("delivered to 9 Elm Rd", Text(r));
    }

    [Fact]
    public async Task DecliningTheFormPlacesNoOrder()
    {
        var user = new ScriptedUser(_ => new ElicitResult { Action = "decline" });
        await using var h = await Harness.StartAsync(PizzaShop.Spec, user.Handlers);
        var r = await h.Client.CallToolAsync("order_pizza", Args(new { size = "large" }));
        Assert.Contains("Order not placed", Text(r));
    }

    [Fact]
    public async Task InvalidValuesAreAskedForAgain()
    {
        var user = new ScriptedUser(_ => Accept(new { size = "large" }));
        await using var h = await Harness.StartAsync(PizzaShop.Spec, user.Handlers);
        var r = await h.Client.CallToolAsync("order_pizza", Args(new
        {
            size = "gigantic", crust = "thin", toppings = new[] { "ham" }, quantity = 1, name = "Bo", fulfillment = "pickup",
        }));
        Assert.Equal(["size"], Assert.Single(user.Forms).RequestedSchema!.Properties.Keys);
        Assert.Contains("large", Text(r));
    }
}
