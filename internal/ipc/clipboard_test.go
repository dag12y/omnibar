package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"omnibar/internal/config"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/clipboard"
)

type clipboardStub struct{}

func (clipboardStub) Watch(ctx context.Context, _ func(clipboard.Capture)) error {
	<-ctx.Done()
	return ctx.Err()
}
func (clipboardStub) Copy(context.Context, string) error { return nil }
func TestClipboardReadAndWritePermissionsAreIndependent(t *testing.T) {
	for _, mode := range []string{"denied", "disabled", "read-only", "read-write"} {
		t.Run(mode, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Permissions[clipboard.ReadCapability] = mode != "denied"
			cfg.Permissions[clipboard.WriteCapability] = mode == "read-write"
			if mode == "disabled" {
				cfg.Modules[clipboard.Name] = config.ModuleConfig{Enabled: false}
			}
			m := clipboard.New(clipboardStub{})
			registry := modules.NewRegistry(cfg, nil)
			registry.Register(m)
			registry.Initialize(context.Background())
			registry.Start(context.Background())
			defer registry.Stop(context.Background())
			d := jsonrpc.NewDispatcher()
			RegisterClipboardMethods(d, registry, m)
			for _, tc := range []struct{ method, params string }{{"clipboard.getSnapshot", `{}`}, {"clipboard.setCollecting", `{"collecting":true}`}, {"clipboard.setCollecting", `{"collecting":false}`}, {"clipboard.copy", `{"id":"stale"}`}, {"clipboard.clear", `{}`}} {
				out, err := d.Handle([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`, tc.method, tc.params)))
				if err != nil {
					t.Fatal(err)
				}
				var response jsonrpc.Response
				json.Unmarshal(out, &response)
				want := 0
				if mode == "denied" {
					want = jsonrpc.CodePermissionDenied
				} else if mode == "disabled" {
					want = jsonrpc.CodeModuleUnavailable
				} else if tc.method == "clipboard.copy" {
					want = jsonrpc.CodeInternalError
					if mode == "read-only" {
						want = jsonrpc.CodePermissionDenied
					}
				}
				if want == 0 {
					if response.Error != nil {
						t.Fatalf("%s failed: %s", tc.method, out)
					}
					var snap clipboard.Snapshot
					json.Unmarshal(response.Result, &snap)
					if snap.CanCopy != (mode == "read-write") {
						t.Fatal("wrong write capability")
					}
				} else if response.Error == nil || response.Error.Code != want {
					t.Fatalf("%s: %s", tc.method, out)
				}
			}
		})
	}
}

func TestClipboardInvalidActionParameters(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Permissions[clipboard.ReadCapability] = true
	cfg.Permissions[clipboard.WriteCapability] = true
	m := clipboard.New(clipboardStub{})
	registry := modules.NewRegistry(cfg, nil)
	registry.Register(m)
	registry.Initialize(context.Background())
	registry.Start(context.Background())
	defer registry.Stop(context.Background())
	d := jsonrpc.NewDispatcher()
	RegisterClipboardMethods(d, registry, m)
	for _, tc := range []struct{ method, params string }{
		{"clipboard.setCollecting", `{}`},
		{"clipboard.setCollecting", `{"collecting":"yes"}`},
		{"clipboard.setCollecting", `null`},
		{"clipboard.copy", `{"id":7}`},
		{"clipboard.copy", `{}`},
		{"clipboard.delete", `{"id":""}`},
	} {
		out, err := d.Handle([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`, tc.method, tc.params)))
		if err != nil {
			t.Fatal(err)
		}
		var response jsonrpc.Response
		if err := json.Unmarshal(out, &response); err != nil {
			t.Fatal(err)
		}
		if response.Error == nil || response.Error.Code != jsonrpc.CodeInvalidParams {
			t.Fatalf("%s %s: %s", tc.method, tc.params, out)
		}
	}
	if m.Snapshot().Collecting {
		t.Fatal("invalid request started collection")
	}
}
