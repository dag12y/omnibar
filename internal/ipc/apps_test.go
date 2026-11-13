package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"omnibar/internal/config"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/apps"
	"testing"
)

type appDriver struct {
	reads, launches int

	deadline bool
}

func (f *appDriver) Read(ctx context.Context) ([]apps.App, bool, error) {
	f.reads++
	_, f.deadline = ctx.Deadline()
	return []apps.App{{ID: "fixture.desktop", Name: "Test app"}}, false, nil
}
func (f *appDriver) Launch(_ context.Context, id string) error {
	f.launches++

	return nil
}
func TestAppRPCGrantsValidationAndLaunch(t *testing.T) {
	for _, mode := range []string{"denied", "disabled", "granted"} {
		t.Run(mode, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Permissions[apps.Capability] = mode != "denied"
			if mode == "disabled" {
				cfg.Modules[apps.Name] = config.ModuleConfig{Enabled: false}
			}
			f := &appDriver{}
			m := apps.New(f)
			registry := modules.NewRegistry(cfg, nil)
			registry.Register(m)
			registry.Initialize(context.Background())
			registry.Start(context.Background())
			defer registry.Stop(context.Background())
			d := jsonrpc.NewDispatcher()
			RegisterAppMethods(d, registry, m)
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
			for _, method := range []string{"apps.getSnapshot", "apps.launch"} {
				response := call(method, `{"id":"fixture.desktop"}`)
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
				if f.reads != 0 || f.launches != 0 {
					t.Fatal("denied call reached desktop adapter")
				}
				return
			}
			if !f.deadline || f.launches != 1 {
				t.Fatal("launch/deadline missing")
			}
			reads := f.reads
			for _, params := range []string{`{}`, `null`, `{"id":2}`, `{"id":"../app.desktop"}`, `{"id":"/tmp/app.desktop"}`} {
				response := call("apps.launch", params)
				if response.Error == nil || response.Error.Code != jsonrpc.CodeInvalidParams {
					t.Fatal("invalid ID accepted")
				}
			}
			if f.reads != reads {
				t.Fatal("invalid params reached driver")
			}
			response := call("apps.launch", `{"id":"stale.desktop"}`)
			if response.Error == nil || f.launches != 1 {
				t.Fatal("stale target launched")
			}
		})
	}
}
