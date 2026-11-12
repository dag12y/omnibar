package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
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
			for _, tc := range []struct{ method, params string }{{"clipboard.getSnapshot", `{}`}, {"clipboard.setCollecting", `{"collecting":true}`}, {"clipboard.setCollecting", `{"collecting":false}`}, {"clipboard.copy", `{"id":"stale"}`}, {"clipboard.transformCopy", `{"id":"stale","operation":"trim"}`}, {"clipboard.clear", `{}`}, {"clipboard.setFavorite", `{"id":"stale","favorite":true}`}} {
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
				} else if tc.method == "clipboard.setFavorite" {
					want = jsonrpc.CodeInternalError
				} else if tc.method == "clipboard.copy" || tc.method == "clipboard.transformCopy" {
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
		{"clipboard.transformCopy", `{"id":"1"}`},
		{"clipboard.transformCopy", `{"id":"1","operation":"unknown"}`},
		{"clipboard.transformCopy", `{"id":"1","operation":3}`},
		{"clipboard.transformCopy", `{"operation":"trim"}`},
		{"clipboard.delete", `{"id":""}`},
		{"clipboard.setFavorite", `{"id":"1"}`},
		{"clipboard.setFavorite", `{"id":"1","favorite":"yes"}`},
		{"clipboard.setFavorite", `{"favorite":true}`},
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

func TestClipboardReadOnlyCanManageSavedFavorites(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Permissions[clipboard.ReadCapability] = true
	store := clipboard.FileStore{Path: filepath.Join(t.TempDir(), "favorites.json")}
	if err := store.Save([]clipboard.Entry{{Text: "saved fixture", CopiedAt: "2026-11-12T10:00:00Z"}}); err != nil {
		t.Fatal(err)
	}
	m := clipboard.NewWithStore(clipboardStub{}, store)
	registry := modules.NewRegistry(cfg, nil)
	registry.Register(m)
	registry.Initialize(context.Background())
	registry.Start(context.Background())
	defer registry.Stop(context.Background())
	d := jsonrpc.NewDispatcher()
	RegisterClipboardMethods(d, registry, m)
	id := m.Snapshot().Entries[0].ID
	for _, favorite := range []bool{false, true} {
		out, _ := d.Handle([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"clipboard.setFavorite","params":{"id":%q,"favorite":%t}}`, id, favorite)))
		var response jsonrpc.Response
		json.Unmarshal(out, &response)
		if response.Error != nil {
			t.Fatalf("read-only favorite action failed: %s", out)
		}
		var snap clipboard.Snapshot
		json.Unmarshal(response.Result, &snap)
		if snap.CanCopy || !snap.CanFavorite || snap.Entries[0].Favorite != favorite {
			t.Fatal("incorrect favorite or grant state")
		}
	}
}

type transformRPCDriver struct {
	copied   string
	deadline bool
}

func (f *transformRPCDriver) Watch(ctx context.Context, _ func(clipboard.Capture)) error {
	<-ctx.Done()
	return ctx.Err()
}
func (f *transformRPCDriver) Copy(ctx context.Context, text string) error {
	f.copied = text
	_, f.deadline = ctx.Deadline()
	return nil
}
func TestClipboardTransformRPCUsesConfirmedSourceAndDeadline(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Permissions[clipboard.ReadCapability] = true
	cfg.Permissions[clipboard.WriteCapability] = true
	store := clipboard.FileStore{Path: filepath.Join(t.TempDir(), "favorites.json")}
	store.Save([]clipboard.Entry{{Text: "  Exact Source  ", CopiedAt: "2026-11-12T10:00:00Z"}})
	driver := &transformRPCDriver{}
	m := clipboard.NewWithStore(driver, store)
	registry := modules.NewRegistry(cfg, nil)
	registry.Register(m)
	registry.Initialize(context.Background())
	registry.Start(context.Background())
	defer registry.Stop(context.Background())
	d := jsonrpc.NewDispatcher()
	RegisterClipboardMethods(d, registry, m)
	id := m.Snapshot().Entries[0].ID
	out, err := d.Handle([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"clipboard.transformCopy","params":{"id":%q,"operation":"trim","text":"untrusted replacement"}}`, id)))
	if err != nil {
		t.Fatal(err)
	}
	var response jsonrpc.Response
	json.Unmarshal(out, &response)
	if response.Error != nil || driver.copied != "Exact Source" || !driver.deadline {
		t.Fatalf("transform RPC failed: %s", out)
	}
	var snap clipboard.Snapshot
	json.Unmarshal(response.Result, &snap)
	if !snap.CanCopy || snap.Entries[0].Text != "  Exact Source  " || !snap.Entries[0].Favorite {
		t.Fatal("RPC changed original or grants")
	}
}
