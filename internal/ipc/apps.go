package ipc

import (
	"context"
	"encoding/json"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/apps"
	"time"
)

func RegisterAppMethods(d *jsonrpc.Dispatcher, registry *modules.Registry, launcher *apps.Module) {
	for _, method := range []string{"apps.getSnapshot", "apps.launch"} {
		d.RegisterContext(method, func(ctx context.Context, params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
			if !registry.IsEnabled(apps.Name) {
				return nil, jsonrpc.NewError(jsonrpc.CodeModuleUnavailable, "Application launcher is disabled", apps.Name)
			}
			if !registry.HasCapability(apps.Name, apps.Capability) {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Enable process.manage in your Omnibar config and restart the backend", apps.Name)
			}
			var p struct {
				ID string `json:"id"`
			}
			if method == "apps.launch" && (json.Unmarshal(params, &p) != nil || !apps.ValidID(p.ID)) {
				return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid application ID", apps.Name)
			}
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			var snap apps.Snapshot
			var err error
			if method == "apps.launch" {
				snap, err = launcher.Launch(ctx, p.ID)
			} else {
				snap, err = launcher.Snapshot(ctx)
			}
			if err != nil {
				return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, err.Error(), apps.Name)
			}
			out, _ := json.Marshal(snap)
			return out, nil
		})
	}
}
