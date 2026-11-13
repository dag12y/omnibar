package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"omnibar/internal/config"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/notes"
	"path/filepath"
	"testing"
)

type noteStoreSpy struct {
	loads, writes int
	data          notes.Data
}

func (s *noteStoreSpy) Load() (notes.Data, error) {
	s.loads++
	return notes.Data{Version: 1, Revision: 1, Notes: []notes.Note{}}, nil
}
func (s *noteStoreSpy) Save(d notes.Data) error { s.writes++; s.data = d; return nil }
func TestNotesRPCGatesValidationAndConflicts(t *testing.T) {
	for _, mode := range []string{"denied", "disabled", "granted"} {
		t.Run(mode, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Permissions[notes.Capability] = mode != "denied"
			if mode == "disabled" {
				cfg.Modules[notes.Name] = config.ModuleConfig{Enabled: false}
			}
			spy := &noteStoreSpy{}
			var store notes.Store
			if cfg.Permissions[notes.Capability] {
				store = spy
			}
			m := notes.New(store, nil)
			r := modules.NewRegistry(cfg, nil)
			r.Register(m)
			r.Initialize(context.Background())
			r.Start(context.Background())
			defer r.Stop(context.Background())
			d := jsonrpc.NewDispatcher()
			RegisterNoteMethods(d, r, m)
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
				for _, method := range []string{"notes.getSnapshot", "notes.get", "notes.create", "notes.update", "notes.delete", "notes.copy"} {
					out := call(method, map[string]any{})
					want := jsonrpc.CodePermissionDenied
					if mode == "disabled" {
						want = jsonrpc.CodeModuleUnavailable
					}
					if out.Error == nil || out.Error.Code != want {
						t.Fatal("grant bypass", out.Error)
					}
				}
				if spy.loads != 0 || spy.writes != 0 {
					t.Fatal("disabled notes touched storage")
				}
				return
			}
			for _, tc := range []struct {
				method string
				p      any
			}{{"notes.get", map[string]any{"id": "bad"}}, {"notes.create", map[string]any{"title": "ok"}}, {"notes.update", map[string]any{"id": "00000000000000000000000000000000", "revision": 0}}, {"notes.getSnapshot", map[string]any{"query": 12}}} {
				if out := call(tc.method, tc.p); out.Error == nil || out.Error.Code != jsonrpc.CodeInvalidParams {
					t.Fatal("invalid params accepted", out.Error)
				}
			}
			if spy.writes != 0 {
				t.Fatal("invalid params wrote data")
			}
			resp := call("notes.create", map[string]any{"title": "desk", "body": "hello"})
			if resp.Error != nil {
				t.Fatal(resp.Error)
			}
			var result notes.Result
			json.Unmarshal(resp.Result, &result)
			n := *result.Note
			if result.CanCopy {
				t.Fatal("copy flag bypassed grant")
			}
			resp = call("notes.copy", map[string]any{"id": n.ID, "revision": n.Revision})
			if resp.Error == nil || resp.Error.Code != jsonrpc.CodePermissionDenied {
				t.Fatal("copy grant bypass")
			}
			resp = call("notes.update", map[string]any{"id": n.ID, "revision": n.Revision, "title": n.Title, "body": "new", "pinned": true})
			if resp.Error != nil {
				t.Fatal(resp.Error)
			}
			resp = call("notes.delete", map[string]any{"id": n.ID, "revision": n.Revision})
			if resp.Error == nil || resp.Error.Code != noteConflictCode {
				t.Fatal("stale deletion accepted")
			}
		})
	}
}
func TestNotesRPCRestartUsesPrivateFile(t *testing.T) {
	store := notes.FileStore{Path: filepath.Join(t.TempDir(), "notes.json")}
	m := notes.New(store, nil)
	m.Initialize(context.Background())
	r, e := m.Create("note", "persisted", "")
	if e != nil {
		t.Fatal(e)
	}
	m.Stop(context.Background())
	m = notes.New(store, nil)
	m.Initialize(context.Background())
	got, e := m.Get(r.Note.ID, "")
	if e != nil || got.Note.Body != "persisted" {
		t.Fatal("restart recovery failed")
	}
}
