import QtQuick
import QtTest
import "../components/protocol.js" as Protocol

TestCase {
    name: "Protocol"

    function test_build_request_with_params() {
        const request = JSON.parse(Protocol.buildRequest(7, "module.subscribe", { module: "core" }))
        compare(request.jsonrpc, "2.0")
        compare(request.id, 7)
        compare(request.method, "module.subscribe")
        compare(request.params.module, "core")
    }

    function test_build_request_without_params_omits_the_field() {
        const request = JSON.parse(Protocol.buildRequest(1, "client.handshake"))
        compare(request.params, undefined)
        compare(Object.keys(request).indexOf("params"), -1)
    }

    function test_event_notification_is_classified() {
        const result = Protocol.classify({
            jsonrpc: "2.0",
            method: "module.event",
            params: { module: "system_monitor", event: "cpu", data: { percent: 4.2 } }
        })
        compare(result.kind, "event")
        compare(result.module, "system_monitor")
        compare(result.event, "cpu")
        compare(result.data.percent, 4.2)
    }

    function test_event_data_may_be_null() {
        const result = Protocol.classify({
            jsonrpc: "2.0",
            method: "module.event",
            params: { module: "m", event: "e", data: null }
        })
        compare(result.kind, "event")
        compare(result.data, null)
    }

    function test_malformed_events_are_ignored() {
        compare(Protocol.classify({ method: "module.event" }).kind, "ignored")
        compare(Protocol.classify({ method: "module.event", params: {} }).kind, "ignored")
        compare(Protocol.classify({ method: "module.event", params: { module: "m" } }).kind, "ignored")
        compare(Protocol.classify({ method: "module.event", params: { module: 1, event: "e" } }).kind, "ignored")
    }

    function test_unknown_notifications_and_requests_are_ignored() {
        compare(Protocol.classify({ method: "something.else", params: {} }).kind, "ignored")
        // A request from the backend (has an id and a method) is not an event.
        compare(Protocol.classify({
            id: 3, method: "module.event", params: { module: "m", event: "e" }
        }).kind, "ignored")
    }

    function test_response_is_classified() {
        const ok = Protocol.classify({ jsonrpc: "2.0", id: 4, result: { module: "core", events: ["*"] } })
        compare(ok.kind, "response")
        compare(ok.id, 4)
        compare(ok.result.module, "core")
        compare(ok.error, undefined)

        const failed = Protocol.classify({ jsonrpc: "2.0", id: 5, error: { code: -32001, message: "no" } })
        compare(failed.kind, "response")
        compare(failed.error.code, -32001)
    }

    function test_garbage_is_ignored() {
        compare(Protocol.classify(null).kind, "ignored")
        compare(Protocol.classify(undefined).kind, "ignored")
        compare(Protocol.classify("text").kind, "ignored")
        compare(Protocol.classify(42).kind, "ignored")
        compare(Protocol.classify([]).kind, "ignored")
        compare(Protocol.classify({}).kind, "ignored")
        compare(Protocol.classify({ id: null, result: 1 }).kind, "ignored")
    }

    function test_subscription_plan_keeps_order_and_drops_junk() {
        compare(Protocol.subscriptionPlan(["core", "system_monitor"]).join(","), "core,system_monitor")
        compare(Protocol.subscriptionPlan(["a", "", "a", 5, null, "b"]).join(","), "a,b")
        compare(Protocol.subscriptionPlan([]).length, 0)
        compare(Protocol.subscriptionPlan(undefined).length, 0)
        compare(Protocol.subscriptionPlan("core").length, 0)
    }

    function test_subscription_state_from_response() {
        compare(Protocol.subscriptionState({ result: {} }), Protocol.Subscribed)
        compare(Protocol.subscriptionState({ error: { code: -32001 } }), Protocol.Denied)
        compare(Protocol.subscriptionState({ error: { code: -32002 } }), Protocol.Unavailable)
        compare(Protocol.subscriptionState({ error: { code: -32600 } }), Protocol.Failed)
        compare(Protocol.subscriptionState({ error: {} }), Protocol.Failed)
    }

    function test_with_state_returns_a_new_object() {
        const before = { core: "pending" }
        const after = Protocol.withState(before, "core", "subscribed")
        compare(before.core, "pending")
        compare(after.core, "subscribed")
        verify(before !== after)

        const more = Protocol.withState(after, "other", "denied")
        compare(more.core, "subscribed")
        compare(more.other, "denied")
    }
}
