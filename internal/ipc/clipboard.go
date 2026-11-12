package ipc

import (
	"context"
	"encoding/json"
	"time"

	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/clipboard"
)

func RegisterClipboardMethods(d *jsonrpc.Dispatcher, registry *modules.Registry, history *clipboard.Module) {
	for _, method := range []string{"clipboard.getSnapshot", "clipboard.setCollecting", "clipboard.copy", "clipboard.delete", "clipboard.clear", "clipboard.setFavorite", "clipboard.transformCopy"} {
		d.RegisterContext(method, func(ctx context.Context, params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
			if !registry.IsEnabled(clipboard.Name) {
				return nil, jsonrpc.NewError(jsonrpc.CodeModuleUnavailable, "Clipboard history is disabled", clipboard.Name)
			}
			if !registry.HasCapability(clipboard.Name, clipboard.ReadCapability) {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Permission denied: clipboard.read", clipboard.Name)
			}
			canCopy := registry.HasCapability(clipboard.Name, clipboard.WriteCapability)
			if (method == "clipboard.copy" || method == "clipboard.transformCopy") && !canCopy {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Permission denied: clipboard.write", clipboard.Name)
			}
			var p struct {
				ID         string `json:"id"`
				Collecting *bool  `json:"collecting"`
				Favorite   *bool  `json:"favorite"`
				Operation  string `json:"operation"`
			}
			if method != "clipboard.getSnapshot" && method != "clipboard.clear" {
				if err := json.Unmarshal(params, &p); err != nil {
					return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid clipboard parameters", clipboard.Name)
				}
				if (method == "clipboard.setCollecting" && p.Collecting == nil) || (method != "clipboard.setCollecting" && p.ID == "") || (method == "clipboard.setFavorite" && p.Favorite == nil) {
					return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Missing clipboard parameters", clipboard.Name)
				}
				if method == "clipboard.transformCopy" && !clipboard.IsTransform(p.Operation) {
					return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Unsupported clipboard transform", clipboard.Name)
				}
			}
			var snap clipboard.Snapshot
			var err error
			switch method {
			case "clipboard.getSnapshot":
				snap = history.Snapshot()
			case "clipboard.setCollecting":
				snap, err = history.SetCollecting(*p.Collecting)
			case "clipboard.clear":
				snap = history.Clear()
			case "clipboard.delete":
				snap, err = history.Delete(p.ID)
			case "clipboard.setFavorite":
				snap, err = history.SetFavorite(p.ID, *p.Favorite)
			case "clipboard.copy", "clipboard.transformCopy":
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				if method == "clipboard.transformCopy" {
					snap, err = history.TransformCopy(ctx, p.ID, p.Operation)
				} else {
					snap, err = history.Copy(ctx, p.ID)
				}
			}
			if err != nil {
				return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, err.Error(), clipboard.Name)
			}
			snap.CanCopy = canCopy
			out, _ := json.Marshal(snap)
			return out, nil
		})
	}
}
