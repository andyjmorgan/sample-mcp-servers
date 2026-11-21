// elicitationserver speaks only MCP 2026-07-28 (stateless) and has one tool, order_pizza. Anything the
// order is missing is asked for with a form elicitation, using multi round-trip requests (MRTR): the
// tool returns an input_required result carrying the form and the partial order as requestState, and
// the client retries tools/call with the form answer in inputResponses and the state echoed back. No
// state is held on the server between rounds.
//
// The address is only needed for delivery, so an order that is missing both the fulfilment method
// and the address can take two rounds: the first form asks for the method (with an optional address),
// and a second asks for the address if the user picked delivery and left it blank.

using System.ComponentModel;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using ModelContextProtocol.Protocol;
using ModelContextProtocol.Server;
using SampleMcp.Hosting;
using ElicitationServer;
using static ModelContextProtocol.Protocol.ElicitRequestParams;

SampleHost.Run(PizzaShop.Spec, args);

namespace ElicitationServer
{
    public sealed record Order
    {
        [JsonPropertyName("size")] public string? Size { get; init; }
        [JsonPropertyName("crust")] public string? Crust { get; init; }
        [JsonPropertyName("toppings")] public List<string>? Toppings { get; init; }
        [JsonPropertyName("quantity")] public int? Quantity { get; init; }
        [JsonPropertyName("name")] public string? Name { get; init; }
        [JsonPropertyName("fulfillment")] public string? Fulfillment { get; init; }
        [JsonPropertyName("address")] public string? Address { get; init; }
    }

    public sealed record Confirmation(string OrderId, string Summary, decimal Total, Order Order);

    public static class PizzaShop
    {
        public const string FormKey = "order_details";
        public static readonly string[] Sizes = ["small", "medium", "large"];
        public static readonly string[] Crusts = ["thin", "classic", "deep-dish", "stuffed"];
        public static readonly string[] Toppings =
            ["pepperoni", "mushrooms", "onions", "sausage", "peppers", "olives", "ham", "pineapple", "extra cheese", "jalapenos"];
        public static readonly string[] Fulfillments = ["delivery", "pickup"];
        private static readonly Dictionary<string, decimal> BasePrice = new() { ["small"] = 9m, ["medium"] = 12m, ["large"] = 15m };

        public static readonly ServerSpec Spec = new("elicitationserver", "1.0.0",
            "A pizza shop with one tool, order_pizza. Pass whatever the user has already told you (size, crust, toppings, " +
            "quantity, name, delivery or pickup, address). Do not guess the rest: the server asks the user for anything " +
            "missing through a form elicitation, possibly over two rounds, and returns the confirmed order.",
            mcp => mcp.WithTools<PizzaTools>());

        /// <summary>Keeps only valid values, normalised to the menu's spelling.</summary>
        public static Order Clean(Order o)
        {
            static string? Pick(string? v, string[] allowed) =>
                v is null ? null : allowed.FirstOrDefault(a => string.Equals(a, v.Trim(), StringComparison.OrdinalIgnoreCase));
            static string? Text(string? v) => string.IsNullOrWhiteSpace(v) ? null : v.Trim();
            return new Order
            {
                Size = Pick(o.Size, Sizes),
                Crust = Pick(o.Crust, Crusts),
                Toppings = o.Toppings?.Select(t => Pick(t, Toppings)).OfType<string>().Distinct().ToList(),
                Quantity = o.Quantity is >= 1 and <= 10 ? o.Quantity : null,
                Name = Text(o.Name),
                Fulfillment = Pick(o.Fulfillment, Fulfillments),
                Address = Text(o.Address),
            };
        }

        /// <summary>Fields still to ask for, in form order.</summary>
        public static List<string> Missing(Order o)
        {
            var m = new List<string>();
            if (o.Size is null) m.Add("size");
            if (o.Crust is null) m.Add("crust");
            if (o.Toppings is null) m.Add("toppings");
            if (o.Quantity is null) m.Add("quantity");
            if (o.Name is null) m.Add("name");
            if (o.Fulfillment is null) m.Add("fulfillment");
            if (o.Fulfillment != "pickup" && o.Address is null) m.Add("address");
            return m;
        }

        /// <summary>Overlays b's fields onto a wherever b has one.</summary>
        public static Order Merge(Order a, Order b) => new()
        {
            Size = b.Size ?? a.Size,
            Crust = b.Crust ?? a.Crust,
            Toppings = b.Toppings ?? a.Toppings,
            Quantity = b.Quantity ?? a.Quantity,
            Name = b.Name ?? a.Name,
            Fulfillment = b.Fulfillment ?? a.Fulfillment,
            Address = b.Address ?? a.Address,
        };

        public static ElicitRequestParams Form(Order o, List<string> missing)
        {
            var props = new Dictionary<string, PrimitiveSchemaDefinition>();
            var required = new List<string>();
            foreach (var f in missing)
            {
                switch (f)
                {
                    case "size":
                        props[f] = new UntitledSingleSelectEnumSchema { Title = "Size", Enum = Sizes, Default = "medium" };
                        required.Add(f);
                        break;
                    case "crust":
                        props[f] = new UntitledSingleSelectEnumSchema { Title = "Crust", Enum = Crusts, Default = "classic" };
                        required.Add(f);
                        break;
                    case "toppings":
                        // Optional in the form: submitting with none selected means a plain cheese pizza.
                        props[f] = new UntitledMultiSelectEnumSchema
                        {
                            Title = "Toppings", Description = "Pick any number, or none for plain cheese.",
                            Items = new UntitledEnumItemsSchema { Enum = Toppings }, MaxItems = 5,
                        };
                        break;
                    case "quantity":
                        props[f] = new NumberSchema { Type = "integer", Title = "How many pizzas", Minimum = 1, Maximum = 10, Default = 1 };
                        required.Add(f);
                        break;
                    case "name":
                        props[f] = new StringSchema { Title = "Name for the order", MinLength = 1, MaxLength = 60 };
                        required.Add(f);
                        break;
                    case "fulfillment":
                        props[f] = new UntitledSingleSelectEnumSchema { Title = "Delivery or pickup", Enum = Fulfillments };
                        required.Add(f);
                        break;
                    case "address":
                        // When the method is still unknown the address is optional; a second round asks
                        // for it if the user picks delivery and leaves it blank.
                        props[f] = new StringSchema
                        {
                            Title = "Delivery address",
                            Description = o.Fulfillment is null ? "Leave blank if you choose pickup." : null,
                            MaxLength = 200,
                        };
                        if (o.Fulfillment == "delivery") required.Add(f);
                        break;
                }
            }
            var have = Describe(o);
            return new ElicitRequestParams
            {
                Message = have.Length == 0
                    ? "Let's build your pizza order."
                    : $"A few more details for your order ({have}).",
                RequestedSchema = new RequestSchema { Properties = props, Required = required },
            };
        }

        public static string Describe(Order o)
        {
            var parts = new List<string>();
            if (o.Quantity is { } q) parts.Add(q == 1 ? "1 pizza" : $"{q} pizzas");
            if (o.Size is not null) parts.Add(o.Size);
            if (o.Crust is not null) parts.Add($"{o.Crust} crust");
            if (o.Toppings is { } t) parts.Add(t.Count == 0 ? "cheese" : string.Join(", ", t));
            if (o.Fulfillment == "pickup") parts.Add("for pickup");
            if (o.Fulfillment == "delivery") parts.Add(o.Address is null ? "for delivery" : $"delivered to {o.Address}");
            if (o.Name is not null) parts.Add($"for {o.Name}");
            return string.Join(", ", parts);
        }

        public static Confirmation Confirm(Order o)
        {
            var each = BasePrice[o.Size!] + 1.5m * o.Toppings!.Count;
            var total = each * o.Quantity!.Value + (o.Fulfillment == "delivery" ? 3m : 0m);
            var id = "PZ-" + Convert.ToHexString(Guid.NewGuid().ToByteArray()[..4]);
            return new Confirmation(id, $"Order {id}: {Describe(o)}. Total ${total:0.00}.", total, o);
        }

        public static string EncodeState(Order o) => Convert.ToBase64String(JsonSerializer.SerializeToUtf8Bytes(o));

        public static Order DecodeState(string? s)
        {
            if (string.IsNullOrEmpty(s)) return new Order();
            try
            {
                return JsonSerializer.Deserialize<Order>(Convert.FromBase64String(s)) ?? new Order();
            }
            catch (Exception e) when (e is FormatException or JsonException)
            {
                return new Order(); // unreadable state: start over and ask again
            }
        }

        /// <summary>Reads an accepted form's content into an Order.</summary>
        public static Order FromForm(IDictionary<string, JsonElement>? content)
        {
            if (content is null) return new Order();
            string? Str(string k) => content.TryGetValue(k, out var v) && v.ValueKind == JsonValueKind.String ? v.GetString() : null;
            int? Int(string k) => content.TryGetValue(k, out var v) && v.ValueKind == JsonValueKind.Number && v.TryGetInt32(out var i) ? i : null;
            List<string>? List(string k) => content.TryGetValue(k, out var v) && v.ValueKind == JsonValueKind.Array
                ? v.EnumerateArray().Where(e => e.ValueKind == JsonValueKind.String).Select(e => e.GetString()!).ToList()
                : null;
            return new Order
            {
                Size = Str("size"), Crust = Str("crust"), Toppings = List("toppings"), Quantity = Int("quantity"),
                Name = Str("name"), Fulfillment = Str("fulfillment"), Address = Str("address"),
            };
        }
    }

    [McpServerToolType]
    public sealed class PizzaTools
    {
        [McpServerTool(Name = "order_pizza", Title = "Order Pizza", ReadOnly = false, Destructive = false, Idempotent = false, OpenWorld = false)]
        [Description("Order pizza. Pass any details the user has given; the server asks the user for anything missing with a form and returns the confirmed order. Menu: sizes small/medium/large; crusts thin/classic/deep-dish/stuffed; toppings pepperoni, mushrooms, onions, sausage, peppers, olives, ham, pineapple, extra cheese, jalapenos (up to 5).")]
        public static CallToolResult OrderPizza(
            McpServer server,
            RequestContext<CallToolRequestParams> context,
            [Description("small, medium or large.")] string? size = null,
            [Description("thin, classic, deep-dish or stuffed.")] string? crust = null,
            [Description("Toppings from the menu. An empty list means plain cheese.")] List<string>? toppings = null,
            [Description("Number of pizzas, 1-10.")] int? quantity = null,
            [Description("Name for the order.")] string? name = null,
            [Description("delivery or pickup.")] string? fulfillment = null,
            [Description("Delivery address. Not needed for pickup.")] string? address = null)
        {
            var order = PizzaShop.Clean(new Order
            {
                Size = size, Crust = crust, Toppings = toppings, Quantity = quantity,
                Name = name, Fulfillment = fulfillment, Address = address,
            });

            // A retry carries the earlier partial order and the user's form answer.
            if (context.Params?.RequestState is { } state)
            {
                order = PizzaShop.Merge(PizzaShop.Clean(PizzaShop.DecodeState(state)), order);
            }
            if (context.Params?.InputResponses?.TryGetValue(PizzaShop.FormKey, out var response) == true)
            {
                var answer = response.Deserialize(InputResponse.ElicitResultJsonTypeInfo);
                if (answer is null || !answer.IsAccepted)
                {
                    return Text($"Order not placed: the user chose {answer?.Action ?? "cancel"} on the order form.", isError: false);
                }
                var asked = PizzaShop.Missing(order);
                order = PizzaShop.Merge(order, PizzaShop.Clean(PizzaShop.FromForm(answer.Content)));
                // Toppings are optional in the form: if this form asked for them and the user
                // picked none, that means plain cheese. A later form that didn't ask leaves them be.
                if (asked.Contains("toppings") && order.Toppings is null) order = order with { Toppings = [] };
            }

            var missing = PizzaShop.Missing(order);
            if (missing.Count == 0)
            {
                var c = PizzaShop.Confirm(order);
                return new CallToolResult
                {
                    Content = [new TextContentBlock { Text = c.Summary }],
                    StructuredContent = JsonSerializer.SerializeToElement(c),
                };
            }
            if (!server.IsMrtrSupported)
            {
                return Text($"This order is missing: {string.Join(", ", missing)}. Ask the user and call order_pizza again with every detail.", isError: true);
            }
            throw new InputRequiredException(
                inputRequests: new Dictionary<string, InputRequest> { [PizzaShop.FormKey] = InputRequest.ForElicitation(PizzaShop.Form(order, missing)) },
                requestState: PizzaShop.EncodeState(order));
        }

        private static CallToolResult Text(string s, bool isError) =>
            new() { Content = [new TextContentBlock { Text = s }], IsError = isError };
    }
}
