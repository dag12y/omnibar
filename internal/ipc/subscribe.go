package ipc

import (
	"context"
	"encoding/json"

	"omnibar/internal/events"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
)

// subscriptionParams are the params of module.subscribe and
// module.unsubscribe. Events is a pointer so an omitted (or null) list,
// which means "all events", differs from an explicit empty list, which is
// rejected.
type subscriptionParams struct {
	Module string    `json:"module"`
	Events *[]string `json:"events"`
}

type subscriptionResult struct {
	Module string   `json:"module"`
	Events []string `json:"events"`
}

// parseSubscriptionParams validates params and returns the module name and
// the event filter (nil meaning all events).
func parseSubscriptionParams(method string, params json.RawMessage) (string, []string, *jsonrpc.Error) {
	var p subscriptionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return "", nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid "+method+" params", "")
	}
	if p.Module == "" {
		return "", nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "module param is required", "")
	}
	if p.Events == nil {
		return p.Module, nil, nil
	}

	if len(*p.Events) == 0 {
		return "", nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams,
			"events must not be empty; omit it to select every event", p.Module)
	}
	for _, name := range *p.Events {
		if name == "" || name == events.All {
			return "", nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams,
				"events must be non-empty event names", p.Module)
		}
	}
	return p.Module, *p.Events, nil
}

func marshalSubscription(module string, filter []string) json.RawMessage {
	if filter == nil {
		filter = []string{}
	}
	out, _ := json.Marshal(subscriptionResult{Module: module, Events: filter})
	return out
}

func requireSession(ctx context.Context) (*Session, *jsonrpc.Error) {
	sess, ok := SessionFromContext(ctx)
	if !ok {
		return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, "No client session for this request", "")
	}
	return sess, nil
}

// handleModuleSubscribe implements module.subscribe (ADR-0009).
//
// The subscription is checked once, when it is made: the module must exist,
// be enabled and running, and every capability it declares must be
// granted. Subscribing is additive and idempotent.
func handleModuleSubscribe(
	ctx context.Context,
	params json.RawMessage,
	registry *modules.Registry,
	hub *events.Hub,
) (json.RawMessage, *jsonrpc.Error) {
	module, filter, rpcErr := parseSubscriptionParams("module.subscribe", params)
	if rpcErr != nil {
		return nil, rpcErr
	}
	sess, rpcErr := requireSession(ctx)
	if rpcErr != nil {
		return nil, rpcErr
	}
	if registry == nil || hub == nil {
		return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, "Event delivery is unavailable", module)
	}

	if _, ok := registry.Info(module); !ok {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidRequest, "Module not found", module)
	}

	state, _ := registry.State(module)
	if !registry.IsEnabled(module) || state != modules.StateRunning {
		return nil, jsonrpc.NewError(jsonrpc.CodeModuleUnavailable, "Module not available", module)
	}

	for _, capability := range registry.DeclaredCapabilities(module) {
		if !registry.HasCapability(module, capability) {
			return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied,
				"Permission denied: "+capability, module)
		}
	}

	sess.OnClose("events", func() { hub.Remove(sess) })
	return marshalSubscription(module, hub.Subscribe(sess, module, filter)), nil
}

// handleModuleUnsubscribe implements module.unsubscribe (ADR-0009).
//
// It never needs a grant or a running module, so a client can always
// unsubscribe. Removing something that is not subscribed succeeds.
func handleModuleUnsubscribe(
	ctx context.Context,
	params json.RawMessage,
	registry *modules.Registry,
	hub *events.Hub,
) (json.RawMessage, *jsonrpc.Error) {
	module, filter, rpcErr := parseSubscriptionParams("module.unsubscribe", params)
	if rpcErr != nil {
		return nil, rpcErr
	}
	sess, rpcErr := requireSession(ctx)
	if rpcErr != nil {
		return nil, rpcErr
	}
	if registry == nil || hub == nil {
		return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, "Event delivery is unavailable", module)
	}

	if _, ok := registry.Info(module); !ok {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidRequest, "Module not found", module)
	}

	return marshalSubscription(module, hub.Unsubscribe(sess, module, filter)), nil
}

// RegisterEventMethods registers module.subscribe and module.unsubscribe.
// Events themselves are published through hub by modules; the server's
// sessions deliver them to subscribed clients.
func RegisterEventMethods(d *jsonrpc.Dispatcher, registry *modules.Registry, hub *events.Hub) {
	d.RegisterContext("module.subscribe", func(ctx context.Context, params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
		return handleModuleSubscribe(ctx, params, registry, hub)
	})
	d.RegisterContext("module.unsubscribe", func(ctx context.Context, params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
		return handleModuleUnsubscribe(ctx, params, registry, hub)
	})
}
