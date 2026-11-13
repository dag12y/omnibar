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
	for _, method := range []string{"windows.getSnapshot", "windows.focus", "windows.getPreview", "windows.moveWorkspace", "windows.setFloating", "windows.setMaximized"} {
		d.RegisterContext(method, func(ctx context.Context, params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
			if !registry.IsEnabled(windows.Name) {
				return nil, jsonrpc.NewError(jsonrpc.CodeModuleUnavailable, "Window switcher is disabled", windows.Name)
			}
			if !registry.HasCapability(windows.Name, windows.Capability) {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Enable window.manage in your Omnibar config and restart the backend", windows.Name)
			}
			if method == "windows.getPreview" && !registry.HasCapability(windows.Name, windows.PreviewCapability) {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Enable screen.capture in your Omnibar config and restart the backend", windows.Name)
			}
			var p struct {
				Address   string `json:"address"`
				Workspace int    `json:"workspace"`
				Enabled   *bool  `json:"enabled"`
			}
			if method != "windows.getSnapshot" && (json.Unmarshal(params, &p) != nil || !windows.ValidAddress(p.Address)) {
				return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid window address", windows.Name)
			}
			var change windows.Change
			switch method {
			case "windows.moveWorkspace":
				change = windows.Change{Action: "workspace", Workspace: p.Workspace}
			case "windows.setFloating", "windows.setMaximized":
				if p.Enabled == nil {
					return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Missing enabled state", windows.Name)
				}
				action := "floating"
				if method == "windows.setMaximized" {
					action = "maximized"
				}
				change = windows.Change{Action: action, Enabled: *p.Enabled}
			}
			if change.Action != "" && !change.Valid() {
				return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid window control parameters", windows.Name)
			}
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			if method == "windows.getPreview" {
				preview, err := switcher.Preview(ctx, p.Address)
				if err != nil {
					return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, err.Error(), windows.Name)
				}
				out, _ := json.Marshal(preview)
				return out, nil
			}
			var snap windows.Snapshot
			var err error
			if method == "windows.focus" {
				snap, err = switcher.Focus(ctx, p.Address)
			} else if change.Action != "" {
				snap, err = switcher.Change(ctx, p.Address, change)
			} else {
				snap, err = switcher.Snapshot(ctx)
			}
			if err != nil {
				return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, err.Error(), windows.Name)
			}
			out, _ := json.Marshal(struct {
				windows.Snapshot
				PreviewsAllowed   bool `json:"previewsAllowed"`
				ControlsAvailable bool `json:"controlsAvailable"`
			}{snap, registry.HasCapability(windows.Name, windows.PreviewCapability), true})
			return out, nil
		})
	}
}
