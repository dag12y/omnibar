package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"omnibar/internal/config"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/timers"
	"testing"
)

type timerStoreSpy struct {
	loads, writes int
	data          timers.Data
}

func (s *timerStoreSpy) Load() (timers.Data, error) {
	s.loads++
	return timers.Data{Version: 1, Revision: 1, Timers: []timers.Record{}}, nil
}
func (s *timerStoreSpy) Save(d timers.Data) error { s.writes++; s.data = d; return nil }
func TestTimerRPCPermissionsValidationAndExactActions(t *testing.T) {
	for _, mode := range []string{"denied", "disabled", "granted"} {
		t.Run(mode, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Permissions[timers.Capability] = mode != "denied"
			if mode == "disabled" {
				cfg.Modules[timers.Name] = config.ModuleConfig{Enabled: false}
			}
			spy := &timerStoreSpy{}
			var store timers.Store
			if cfg.Permissions[timers.Capability] {
				store = spy
			}
			m := timers.New(store, nil, nil, nil)
			r := modules.NewRegistry(cfg, nil)
			r.Register(m)
			r.Initialize(context.Background())
			r.Start(context.Background())
			defer r.Stop(context.Background())
			d := jsonrpc.NewDispatcher()
			RegisterTimerMethods(d, r, m)
			call := func(method string, p any) jsonrpc.Response {
				t.Helper()
				params, _ := json.Marshal(p)
				raw, e := d.Handle([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`, method, params)))
				if e != nil {
					t.Fatal(e)
				}
				var out jsonrpc.Response
				if json.Unmarshal(raw, &out) != nil {
					t.Fatal("bad response")
				}
				return out
			}
			if mode != "granted" {
				for _, method := range []string{"timers.getSnapshot", "timers.start", "timers.pause", "timers.resume", "timers.cancel", "timers.dismiss"} {
					out := call(method, map[string]any{})
					want := jsonrpc.CodePermissionDenied
					if mode == "disabled" {
						want = jsonrpc.CodeModuleUnavailable
					}
					if out.Error == nil || out.Error.Code != want {
						t.Fatal("gate bypass", out.Error)
					}
				}
				if spy.loads != 0 || spy.writes != 0 {
					t.Fatal("denied timer accessed storage")
				}
				return
			}
			for _, p := range []any{map[string]any{"name": "a", "seconds": 0}, map[string]any{"name": "a", "seconds": 86401}, map[string]any{"name": "\n", "seconds": 3}, map[string]any{"name": "a", "seconds": "5"}, map[string]any{"name": "a", "seconds": 3, "id": "bad"}} {
				if out := call("timers.start", p); out.Error == nil || out.Error.Code != jsonrpc.CodeInvalidParams {
					t.Fatal("invalid start accepted")
				}
			}
			if out := call("timers.pause", map[string]any{}); out.Error == nil || out.Error.Code != jsonrpc.CodeInvalidParams {
				t.Fatal("invalid target accepted")
			}
			if spy.writes != 0 {
				t.Fatal("invalid request wrote")
			}
			out := call("timers.start", map[string]any{"name": "RPC", "seconds": 60})
			if out.Error != nil {
				t.Fatal(out.Error)
			}
			var snap timers.Snapshot
			json.Unmarshal(out.Result, &snap)
			id, rev := snap.Timers[0].ID, snap.Timers[0].Revision
			out = call("timers.pause", map[string]any{"id": id, "revision": rev})
			if out.Error != nil {
				t.Fatal(out.Error)
			}
			json.Unmarshal(out.Result, &snap)
			if snap.Paused != 1 {
				t.Fatal("pause not confirmed")
			}
			if out := call("timers.cancel", map[string]any{"id": id, "revision": rev}); out.Error == nil || out.Error.Code != -32004 {
				t.Fatal("stale action accepted")
			}
			rev = snap.Timers[0].Revision
			out = call("timers.resume", map[string]any{"id": id, "revision": rev})
			if out.Error != nil {
				t.Fatal(out.Error)
			}
			json.Unmarshal(out.Result, &snap)
			out = call("timers.cancel", map[string]any{"id": id, "revision": snap.Timers[0].Revision})
			if out.Error != nil {
				t.Fatal(out.Error)
			}
			json.Unmarshal(out.Result, &snap)
			if len(snap.Timers) != 0 {
				t.Fatal("cancel failed")
			}
		})
	}
}
