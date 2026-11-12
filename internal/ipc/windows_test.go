package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"omnibar/internal/config"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/windows"
	"testing"
)

type windowDriver struct {
	reads, focuses int
	active         string
	deadline       bool
}

func (f *windowDriver) Read(ctx context.Context) ([]windows.Window, string, error) {
	f.reads++
	_, f.deadline = ctx.Deadline()
	return []windows.Window{{Address: "0x1", Class: "Test app", Title: "Fixture", Workspace: "2"}}, f.active, nil
}
func (f *windowDriver) Focus(_ context.Context, address string) error {
	f.focuses++
	f.active = address
	return nil
}
func TestWindowRPCGrantsValidationAndFocus(t *testing.T) {
	for _, mode := range []string{"denied", "disabled", "granted"} {
		t.Run(mode, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Permissions[windows.Capability] = mode != "denied"
			if mode == "disabled" {
				cfg.Modules[windows.Name] = config.ModuleConfig{Enabled: false}
			}
			f := &windowDriver{}
			m := windows.New(f)
			registry := modules.NewRegistry(cfg, nil)
			registry.Register(m)
			registry.Initialize(context.Background())
			registry.Start(context.Background())
			defer registry.Stop(context.Background())
			d := jsonrpc.NewDispatcher()
			RegisterWindowMethods(d, registry, m)
			call := func(method, params string) jsonrpc.Response {
				t.Helper()
				out, err := d.Handle([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`, method, params)))
				if err != nil {
					t.Fatal(err)
				}
				var response jsonrpc.Response
				json.Unmarshal(out, &response)
				return response
			}
			for _, method := range []string{"windows.getSnapshot", "windows.focus"} {
				response := call(method, `{"address":"0x1"}`)
				if mode != "granted" {
					want := jsonrpc.CodePermissionDenied
					if mode == "disabled" {
						want = jsonrpc.CodeModuleUnavailable
					}
					if response.Error == nil || response.Error.Code != want {
						t.Fatal("grant bypassed")
					}
				} else if response.Error != nil {
					t.Fatal(response.Error)
				}
			}
			if mode != "granted" {
				if f.reads != 0 || f.focuses != 0 {
					t.Fatal("denied call contacted compositor")
				}
				return
			}
			if !f.deadline || f.focuses != 1 {
				t.Fatal("focus/deadline missing")
			}
			reads := f.reads
			for _, params := range []string{`{}`, `null`, `{"address":2}`, `{"address":"title:.*"}`, `{"address":"0x1;exec bad"}`} {
				response := call("windows.focus", params)
				if response.Error == nil || response.Error.Code != jsonrpc.CodeInvalidParams {
					t.Fatal("invalid selector accepted")
				}
			}
			if f.reads != reads {
				t.Fatal("invalid params reached driver")
			}
			response := call("windows.focus", `{"address":"0x2"}`)
			if response.Error == nil || f.focuses != 1 {
				t.Fatal("stale target dispatched")
			}
		})
	}
}
