package ipc

import (
	"context"
	"encoding/json"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/fileshelf"
	"time"
)

func RegisterFileShelfMethods(d *jsonrpc.Dispatcher, registry *modules.Registry, shelf *fileshelf.Module) {
	for _, method := range []string{"shelf.getSnapshot", "shelf.add", "shelf.remove", "shelf.clear", "shelf.copyPath", "shelf.openFolder"} {
		d.RegisterContext(method, func(ctx context.Context, params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
			if !registry.IsEnabled(fileshelf.Name) {
				return nil, jsonrpc.NewError(jsonrpc.CodeModuleUnavailable, "File shelf is disabled", fileshelf.Name)
			}
			if !registry.HasCapability(fileshelf.Name, fileshelf.Capability) {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Enable filesystem.access in your Omnibar config and restart the backend", fileshelf.Name)
			}
			canCopy := registry.HasCapability(fileshelf.Name, fileshelf.CopyCapability)
			if method == "shelf.copyPath" && !canCopy {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Permission denied: clipboard.write", fileshelf.Name)
			}
			var p struct {
				ID    string   `json:"id"`
				Paths []string `json:"paths"`
			}
			if method != "shelf.getSnapshot" && method != "shelf.clear" {
				if json.Unmarshal(params, &p) != nil || (method == "shelf.add" && (len(p.Paths) == 0 || len(p.Paths) > fileshelf.MaxEntries)) || (method != "shelf.add" && p.ID == "") {
					return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid file shelf parameters", fileshelf.Name)
				}
			}
			var snap fileshelf.Snapshot
			var err error
			switch method {
			case "shelf.getSnapshot":
				snap = shelf.Snapshot()
			case "shelf.add":
				snap, err = shelf.Add(p.Paths)
			case "shelf.remove":
				snap, err = shelf.Remove(p.ID)
			case "shelf.clear":
				snap = shelf.Clear()
			case "shelf.copyPath", "shelf.openFolder":
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				if method == "shelf.copyPath" {
					snap, err = shelf.CopyPath(ctx, p.ID)
				} else {
					snap, err = shelf.OpenFolder(ctx, p.ID)
				}
			}
			if err != nil {
				return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, err.Error(), fileshelf.Name)
			}
			snap.CanCopy = canCopy
			out, _ := json.Marshal(snap)
			return out, nil
		})
	}
}
