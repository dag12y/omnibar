package ipc

import (
	"context"
	"encoding/json"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/windows"
	"time"
)

func RegisterWindowMethods(d *jsonrpc.Dispatcher, registry *modules.Registry, switcher *windows.Module) {
	for _, method := range []string{"windows.getSnapshot", "windows.focus"} {
		d.RegisterContext(method, func(ctx context.Context, params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
			if !registry.IsEnabled(windows.Name) {
				return nil, jsonrpc.NewError(jsonrpc.CodeModuleUnavailable, "Window switcher is disabled", windows.Name)
			}
			if !registry.HasCapability(windows.Name, windows.Capability) {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Enable window.manage in your Omnibar config and restart the backend", windows.Name)
			}
			var p struct {
				Address string `json:"address"`
			}
			if method == "windows.focus" && (json.Unmarshal(params, &p) != nil || !windows.ValidAddress(p.Address)) {
				return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid window address", windows.Name)
			}
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			var snap windows.Snapshot
			var err error
			if method == "windows.focus" {
				snap, err = switcher.Focus(ctx, p.Address)
			} else {
				snap, err = switcher.Snapshot(ctx)
			}
			if err != nil {
				return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, err.Error(), windows.Name)
			}
			out, _ := json.Marshal(snap)
			return out, nil
		})
	}
}
