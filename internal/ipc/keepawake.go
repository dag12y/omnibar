package ipc

import (
	"context"
	"encoding/json"
	"time"

	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/keepawake"
)

func RegisterKeepAwakeMethods(d *jsonrpc.Dispatcher, registry *modules.Registry, awake *keepawake.Module) {
	register := func(method string, action func(context.Context, json.RawMessage) (keepawake.Status, *jsonrpc.Error)) {
		d.RegisterContext(method, func(ctx context.Context, params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
			if !registry.IsEnabled(keepawake.Name) {
				return nil, jsonrpc.NewError(jsonrpc.CodeModuleUnavailable, "Keep awake is disabled", keepawake.Name)
			}
			if !registry.HasCapability(keepawake.Name, keepawake.Capability) {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Permission denied: power.inhibit", keepawake.Name)
			}
			status, rpcErr := action(ctx, params)
			if rpcErr != nil {
				return nil, rpcErr
			}
			out, _ := json.Marshal(status)
			return out, nil
		})
	}
	register("keepAwake.getStatus", func(context.Context, json.RawMessage) (keepawake.Status, *jsonrpc.Error) {
		return awake.Status(), nil
	})
	register("keepAwake.start", func(ctx context.Context, params json.RawMessage) (keepawake.Status, *jsonrpc.Error) {
		var p struct {
			Seconds int `json:"seconds"`
		}
		if err := json.Unmarshal(params, &p); err != nil || p.Seconds < 1 || p.Seconds > keepawake.MaxSeconds {
			return keepawake.Status{}, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "seconds must be an integer between 1 and 86400", keepawake.Name)
		}
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		status, err := awake.Begin(ctx, p.Seconds)
		if err != nil {
			return status, jsonrpc.NewError(jsonrpc.CodeInternalError, err.Error(), keepawake.Name)
		}
		return status, nil
	})
	register("keepAwake.stop", func(context.Context, json.RawMessage) (keepawake.Status, *jsonrpc.Error) {
		status, err := awake.End()
		if err != nil {
			return status, jsonrpc.NewError(jsonrpc.CodeInternalError, err.Error(), keepawake.Name)
		}
		return status, nil
	})
}
