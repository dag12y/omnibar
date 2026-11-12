package ipc

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"omnibar/internal/config"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/keepawake"
)

func TestKeepAwakeRPCPermissionsAndLifecycle(t *testing.T) {
	for _, grant := range []bool{false, true} {
		t.Run(map[bool]string{false: "denied", true: "granted"}[grant], func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Permissions[keepawake.Capability] = grant
			m := keepawake.New(nil, func(context.Context) (io.Closer, error) { return io.NopCloser(strings.NewReader("")), nil })
			registry := modules.NewRegistry(cfg, nil)
			if err := registry.Register(m); err != nil {
				t.Fatal(err)
			}
			registry.Initialize(context.Background())
			registry.Start(context.Background())
			defer registry.Stop(context.Background())
			d := jsonrpc.NewDispatcher()
			RegisterKeepAwakeMethods(d, registry, m)
			for _, method := range []string{"keepAwake.start", "keepAwake.getStatus", "keepAwake.stop"} {
				raw, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":{"seconds":60}}`))
				if err != nil {
					t.Fatal(err)
				}
				var response jsonrpc.Response
				if err := json.Unmarshal(raw, &response); err != nil {
					t.Fatal(err)
				}
				if !grant {
					if response.Error == nil || response.Error.Code != jsonrpc.CodePermissionDenied {
						t.Fatalf("%s bypassed permission: %s", method, raw)
					}
				} else {
					if response.Error != nil {
						t.Fatalf("%s: %s", method, raw)
					}
					var status keepawake.Status
					json.Unmarshal(response.Result, &status)
					if status.Active != (method != "keepAwake.stop") {
						t.Fatalf("%s: wrong status %+v", method, status)
					}
				}
			}
			if m.Status().Active {
				t.Fatal("inhibitor left active")
			}
		})
	}
}

func TestKeepAwakeRPCRejectsDisabledModuleAndInvalidDurations(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		cfg := config.DefaultConfig()
		cfg.Permissions[keepawake.Capability] = true
		if disabled {
			cfg.Modules[keepawake.Name] = config.ModuleConfig{Enabled: false}
		}
		calls := 0
		m := keepawake.New(nil, func(context.Context) (io.Closer, error) { calls++; return io.NopCloser(strings.NewReader("")), nil })
		registry := modules.NewRegistry(cfg, nil)
		registry.Register(m)
		registry.Initialize(context.Background())
		registry.Start(context.Background())
		d := jsonrpc.NewDispatcher()
		RegisterKeepAwakeMethods(d, registry, m)
		for _, params := range []string{`{}`, `null`, `{"seconds":0}`, `{"seconds":-1}`, `{"seconds":86401}`, `{"seconds":1.5}`, `{"seconds":"60"}`} {
			raw, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"keepAwake.start","params":` + params + `}`))
			if err != nil {
				t.Fatal(err)
			}
			var response jsonrpc.Response
			json.Unmarshal(raw, &response)
			want := jsonrpc.CodeInvalidParams
			if disabled {
				want = jsonrpc.CodeModuleUnavailable
			}
			if response.Error == nil || response.Error.Code != want {
				t.Fatalf("disabled=%v params=%s response=%s", disabled, params, raw)
			}
		}
		if calls != 0 {
			t.Fatal("invalid request acquired a lease")
		}
		registry.Stop(context.Background())
	}
}
