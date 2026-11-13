package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"omnibar/internal/config"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/windows"
	"testing"
)

type windowDriver struct {
	reads, focuses                 int
	workspace, fullscreen, changes int
	floating                       bool
	captures                       int
	active                         string
	deadline                       bool
}

func (f *windowDriver) Capture(ctx context.Context, _ string) ([]byte, error) {
	f.captures++
	_, f.deadline = ctx.Deadline()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (f *windowDriver) Read(ctx context.Context) ([]windows.Window, string, error) {
	f.reads++
	_, f.deadline = ctx.Deadline()
	return []windows.Window{{Address: "0x1", Class: "Test app", Title: "Fixture", Workspace: fmt.Sprint(f.workspace), WorkspaceID: f.workspace, Floating: f.floating, Fullscreen: f.fullscreen}}, f.active, nil
}

func TestWindowPreviewPermissionsAndMetadataRemainIndependent(t *testing.T) {
	for _, mode := range []string{"none", "window-only", "capture-only", "both", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Permissions[windows.Capability] = mode == "window-only" || mode == "both" || mode == "disabled"
			cfg.Permissions[windows.PreviewCapability] = mode == "capture-only" || mode == "both" || mode == "disabled"
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
				out, err := d.Handle([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`, method, params)))
				if err != nil {
					t.Fatal(err)
				}
				var r jsonrpc.Response
				if json.Unmarshal(out, &r) != nil {
					t.Fatal("invalid response")
				}
				return r
			}
			r := call("windows.getPreview", `{"address":"0x1"}`)
			if mode != "both" {
				want := jsonrpc.CodePermissionDenied
				if mode == "disabled" {
					want = jsonrpc.CodeModuleUnavailable
				}
				if r.Error == nil || r.Error.Code != want || f.reads != 0 || f.captures != 0 {
					t.Fatal("preview grant bypassed", r.Error)
				}
			} else {
				if r.Error != nil || f.captures != 1 || !f.deadline {
					t.Fatal("preview failed or deadline missing", r.Error)
				}
				reads := f.reads
				for _, params := range []string{`{}`, `null`, `{"address":3}`, `{"address":"0x1;bad"}`} {
					if r := call("windows.getPreview", params); r.Error == nil || r.Error.Code != jsonrpc.CodeInvalidParams {
						t.Fatal("invalid capture selector accepted")
					}
				}
				if f.reads != reads {
					t.Fatal("invalid params reached capture driver")
				}
			}
			if mode == "window-only" || mode == "both" {
				r = call("windows.getSnapshot", `{}`)
				if r.Error != nil {
					t.Fatal("metadata requires capture grant", r.Error)
				}
				var flags struct {
					PreviewsAllowed bool `json:"previewsAllowed"`
				}
				json.Unmarshal(r.Result, &flags)
				if flags.PreviewsAllowed != (mode == "both") {
					t.Fatal("incorrect preview flag")
				}
				if r := call("windows.focus", `{"address":"0x1"}`); r.Error != nil {
					t.Fatal("focus requires capture grant", r.Error)
				}
			}
		})
	}
}
func (f *windowDriver) Focus(_ context.Context, address string) error {
	f.focuses++
	f.active = address
	return nil
}
func (f *windowDriver) Apply(_ context.Context, _ string, c windows.Change) error {
	f.changes++
	switch c.Action {
	case "workspace":
		f.workspace = c.Workspace
	case "floating":
		f.floating = c.Enabled
	case "maximized":
		f.fullscreen = 0
		if c.Enabled {
			f.fullscreen = 1
		}
	}
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

func TestWindowControlRPCValidationAndGrants(t *testing.T) {
	for _, grant := range []bool{false, true} {
		cfg := config.DefaultConfig()
		cfg.Permissions[windows.Capability] = grant
		f := &windowDriver{workspace: 1}
		m := windows.New(f)
		r := modules.NewRegistry(cfg, nil)
		r.Register(m)
		r.Initialize(context.Background())
		r.Start(context.Background())
		d := jsonrpc.NewDispatcher()
		RegisterWindowMethods(d, r, m)
		call := func(method, params string) jsonrpc.Response {
			out, _ := d.Handle([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`, method, params)))
			var response jsonrpc.Response
			json.Unmarshal(out, &response)
			return response
		}
		for _, tc := range [][2]string{{"windows.moveWorkspace", `{"address":"0x1","workspace":2}`}, {"windows.setFloating", `{"address":"0x1","enabled":true}`}, {"windows.setMaximized", `{"address":"0x1","enabled":true}`}} {
			method, params := tc[0], tc[1]
			response := call(method, params)
			if grant && response.Error != nil {
				t.Fatal(response.Error)
			}
			if !grant && (response.Error == nil || response.Error.Code != jsonrpc.CodePermissionDenied) {
				t.Fatal("grant bypass")
			}
		}
		if !grant {
			if f.reads != 0 || f.changes != 0 {
				t.Fatal("denial reached driver")
			}
			continue
		}
		reads := f.reads
		for method, params := range map[string]string{"windows.moveWorkspace": `{"address":"0x1","workspace":"2;bad"}`, "windows.setFloating": `{"address":"0x1"}`, "windows.setMaximized": `{"address":"0x1","enabled":null}`} {
			if response := call(method, params); response.Error == nil || response.Error.Code != jsonrpc.CodeInvalidParams {
				t.Fatal("invalid control params accepted")
			}
		}
		if f.reads != reads {
			t.Fatal("invalid request reached driver")
		}
		r.Stop(context.Background())
	}
}
