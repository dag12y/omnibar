package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"omnibar/internal/config"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/fileshelf"
)

type shelfDesktop struct{ copies, opens int }

func (f *shelfDesktop) CopyPath(context.Context, string) error   { f.copies++; return nil }
func (f *shelfDesktop) OpenFolder(context.Context, string) error { f.opens++; return nil }
func TestShelfRPCPermissionsAndActions(t *testing.T) {
	for _, mode := range []string{"denied", "disabled", "filesystem-only", "full"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.txt")
			os.WriteFile(path, []byte("original"), 0600)
			cfg := config.DefaultConfig()
			cfg.Permissions[fileshelf.Capability] = mode != "denied"
			cfg.Permissions[fileshelf.CopyCapability] = mode == "full"
			if mode == "disabled" {
				cfg.Modules[fileshelf.Name] = config.ModuleConfig{Enabled: false}
			}
			f := &shelfDesktop{}
			m := fileshelf.New(f)
			r := modules.NewRegistry(cfg, nil)
			r.Register(m)
			r.Initialize(context.Background())
			r.Start(context.Background())
			defer r.Stop(context.Background())
			d := jsonrpc.NewDispatcher()
			RegisterFileShelfMethods(d, r, m)
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
			response := call("shelf.add", fmt.Sprintf(`{"paths":[%q]}`, path))
			if mode == "denied" || mode == "disabled" {
				want := jsonrpc.CodePermissionDenied
				if mode == "disabled" {
					want = jsonrpc.CodeModuleUnavailable
				}
				if response.Error == nil || response.Error.Code != want {
					t.Fatal("add did not fail closed")
				}
				for _, method := range []string{"shelf.getSnapshot", "shelf.clear", "shelf.remove", "shelf.copyPath", "shelf.openFolder"} {
					response = call(method, `{"id":"1"}`)
					if response.Error == nil || response.Error.Code != want {
						t.Fatalf("%s bypassed grant", method)
					}
				}
				if len(m.Snapshot().Entries) != 0 || f.copies != 0 || f.opens != 0 {
					t.Fatal("denied method contacted filesystem/desktop")
				}
				return
			}
			if response.Error != nil {
				t.Fatal(response.Error)
			}
			var snap fileshelf.Snapshot
			json.Unmarshal(response.Result, &snap)
			id := snap.Entries[0].ID
			if snap.CanCopy != (mode == "full") {
				t.Fatal("wrong copy grant")
			}
			response = call("shelf.copyPath", fmt.Sprintf(`{"id":%q}`, id))
			if mode == "full" {
				if response.Error != nil || f.copies != 1 {
					t.Fatal("copy failed")
				}
			} else if response.Error == nil || response.Error.Code != jsonrpc.CodePermissionDenied || f.copies != 0 {
				t.Fatal("copy bypassed write grant")
			}
			response = call("shelf.openFolder", fmt.Sprintf(`{"id":%q}`, id))
			if response.Error != nil || f.opens != 1 {
				t.Fatal("filesystem grant cannot open folder")
			}
			for _, tc := range []struct{ method, params string }{{"shelf.add", `{"paths":[]}`}, {"shelf.add", `{"paths":"bad"}`}, {"shelf.remove", `{}`}, {"shelf.copyPath", `null`}, {"shelf.openFolder", `{"id":3}`}} {
				response = call(tc.method, tc.params)
				want := jsonrpc.CodeInvalidParams
				if tc.method == "shelf.copyPath" && mode != "full" {
					want = jsonrpc.CodePermissionDenied
				}
				if response.Error == nil || response.Error.Code != want {
					t.Fatalf("invalid %s accepted", tc.method)
				}
			}
			response = call("shelf.remove", fmt.Sprintf(`{"id":%q}`, id))
			if response.Error != nil || len(m.Snapshot().Entries) != 0 {
				t.Fatal("remove failed")
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("remove deleted original")
			}
		})
	}
}
