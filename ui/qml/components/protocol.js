.pragma library

// Pure helpers for the Omnibar JSON-RPC client. They hold no state and
// touch no QML types, so they can be unit tested with Qt Test.

// buildRequest returns the JSON text of a JSON-RPC 2.0 request. params is
// omitted when undefined.
function buildRequest(id, method, params) {
    var request = { jsonrpc: "2.0", id: id, method: method }
    if (params !== undefined)
        request.params = params
    return JSON.stringify(request)
}

// classify sorts a decoded incoming message into one of:
//   { kind: "event", module, event, data }   module.event notification
//   { kind: "response", id, result, error }  response to one of our requests
//   { kind: "ignored" }                      anything else
//
// A message is a response only if it carries an id and no method.
function classify(message) {
    if (!message || typeof message !== "object" || Array.isArray(message))
        return { kind: "ignored" }

    if (message.method !== undefined) {
        if (message.method !== "module.event" || message.id !== undefined)
            return { kind: "ignored" }

        var params = message.params
        if (!params || typeof params.module !== "string" || typeof params.event !== "string")
            return { kind: "ignored" }

        return {
            kind: "event",
            module: params.module,
            event: params.event,
            data: params.data
        }
    }

    if (message.id === undefined || message.id === null)
        return { kind: "ignored" }

    return {
        kind: "response",
        id: message.id,
        result: message.result,
        error: message.error
    }
}

// subscriptionPlan returns the unique, non-empty module names to subscribe
// to after a handshake, in handshake order.
function subscriptionPlan(modules) {
    var plan = []
    if (!Array.isArray(modules))
        return plan

    for (var i = 0; i < modules.length; i++) {
        var name = modules[i]
        if (typeof name === "string" && name.length > 0 && plan.indexOf(name) < 0)
            plan.push(name)
    }
    return plan
}

// Subscription states, as shown to the user.
var Subscribed = "subscribed"
var Pending = "pending"
var Denied = "denied"
var Unavailable = "unavailable"
var Failed = "failed"

// subscriptionState maps a module.subscribe response to a state. The error
// codes are defined in ADR-0009.
function subscriptionState(response) {
    if (!response || !response.error)
        return Subscribed

    switch (response.error.code) {
    case -32001:
        return Denied
    case -32002:
        return Unavailable
    default:
        return Failed
    }
}

// withState returns a copy of states with module set to state. Returning a
// new object lets QML property bindings notice the change.
function withState(states, module, state) {
    var copy = {}
    for (var key in states)
        copy[key] = states[key]
    copy[module] = state
    return copy
}
