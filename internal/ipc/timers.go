package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/timers"
)

func RegisterTimerMethods(d *jsonrpc.Dispatcher, r *modules.Registry, m *timers.Module) {
	for _, method := range []string{"timers.getSnapshot", "timers.start", "timers.pause", "timers.resume", "timers.cancel", "timers.dismiss"} {
		d.RegisterContext(method, func(_ context.Context, raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
			if !r.IsEnabled(timers.Name) {
				return nil, jsonrpc.NewError(jsonrpc.CodeModuleUnavailable, "Timers is disabled", timers.Name)
			}
			if !r.HasCapability(timers.Name, timers.Capability) {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Enable timers.manage in your Omnibar config and restart the backend", timers.Name)
			}
			var p struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Seconds  int    `json:"seconds"`
				Revision uint64 `json:"revision"`
			}
			if len(raw) == 0 {
				raw = json.RawMessage(`{}`)
			}
			if json.Unmarshal(raw, &p) != nil {
				return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid timer parameters", timers.Name)
			}
			if method == "timers.start" && (!timers.ValidName(p.Name) || p.Seconds < 1 || p.Seconds > timers.MaxSeconds || (p.ID != "" && !timers.ValidID(p.ID))) {
				return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Use a name of 1–64 bytes and 1–86400 seconds", timers.Name)
			}
			if method != "timers.getSnapshot" && method != "timers.start" && (!timers.ValidID(p.ID) || !timers.ValidRevision(p.Revision)) {
				return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Timer ID and expected revision required", timers.Name)
			}
			var snap timers.Snapshot
			var err error
			switch method {
			case "timers.getSnapshot":
				snap = m.Snapshot()
			case "timers.start":
				snap, err = m.Begin(p.ID, p.Name, p.Seconds)
			case "timers.pause":
				snap, err = m.Change("pause", p.ID, p.Revision)
			case "timers.resume":
				snap, err = m.Change("resume", p.ID, p.Revision)
			case "timers.cancel":
				snap, err = m.Change("cancel", p.ID, p.Revision)
			case "timers.dismiss":
				snap, err = m.Change("dismiss", p.ID, p.Revision)
			}
			if err != nil {
				code := jsonrpc.CodeInternalError
				if errors.Is(err, timers.ErrConflict) {
					code = -32004
				}
				return nil, jsonrpc.NewError(code, err.Error(), timers.Name)
			}
			encoded, _ := json.Marshal(snap)
			return encoded, nil
		})
	}
}
