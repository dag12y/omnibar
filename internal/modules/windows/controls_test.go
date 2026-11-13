package windows

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type controlDriver struct {
	fakeDriver
	changes []Change
	fail    bool
}

func (f *controlDriver) Apply(_ context.Context, address string, c Change) error {
	f.changes = append(f.changes, c)
	if f.fail {
		return fmt.Errorf("failed")
	}
	if f.noConfirm {
		return nil
	}
	for i := range f.entries {
		if f.entries[i].Address != address {
			continue
		}
		switch c.Action {
		case "workspace":
			f.entries[i].WorkspaceID = c.Workspace
		case "floating":
			f.entries[i].Floating = c.Enabled
		case "maximized":
			f.entries[i].Fullscreen = 0
			if c.Enabled {
				f.entries[i].Fullscreen = 1
			}
		case "geometry":
			f.entries[i].Geometry = c.Geometry
		}
	}
	return nil
}
func TestControlsConfirmExactTargetAndIdempotence(t *testing.T) {
	f := &controlDriver{fakeDriver: fakeDriver{entries: []Window{{Address: "0x1", WorkspaceID: 1}, {Address: "0x2", WorkspaceID: 1}}}}
	m := New(f)
	for _, c := range []Change{{Action: "workspace", Workspace: 2}, {Action: "floating", Enabled: true}, {Action: "maximized", Enabled: true}, {Action: "maximized", Enabled: false}, {Action: "geometry", Geometry: Geometry{X: 10, Y: 20, Width: 300, Height: 200}}} {
		s, err := m.Change(context.Background(), "0x2", c)
		if err != nil {
			t.Fatal(err)
		}
		w, _ := target(s, "0x2")
		if !matches(w, c) {
			t.Fatal("unconfirmed state")
		}
		count := len(f.changes)
		if _, err := m.Change(context.Background(), "0x2", c); err != nil || len(f.changes) != count {
			t.Fatal("idempotence failed", err)
		}
	}
	if f.entries[0].WorkspaceID != 1 || f.entries[0].Floating || f.entries[0].Fullscreen != 0 {
		t.Fatal("unrelated target changed")
	}
	count := len(f.changes)
	for _, c := range []Change{{Action: "workspace", Workspace: 0}, {Action: "workspace", Workspace: 10000}, {Action: "arbitrary"}, {Action: "geometry", Geometry: Geometry{Width: 1, Height: 1}}} {
		if _, err := m.Change(context.Background(), "0x2", c); err == nil {
			t.Fatal("invalid params accepted")
		}
	}
	if _, err := m.Change(context.Background(), "0x3", Change{Action: "floating", Enabled: true}); err == nil || len(f.changes) != count {
		t.Fatal("stale target changed")
	}
	f.entries[1].Restricted = true
	if _, err := m.Change(context.Background(), "0x2", Change{Action: "floating", Enabled: false}); err == nil {
		t.Fatal("restricted target changed")
	}
	f.entries[1].Restricted = false
	f.entries[1].Fullscreen = 2
	if _, err := m.Change(context.Background(), "0x2", Change{Action: "maximized", Enabled: false}); err == nil {
		t.Fatal("fullscreen silently exited")
	}
	f.entries[1].Fullscreen = 0
	f.fail = true
	if _, err := m.Change(context.Background(), "0x2", Change{Action: "floating", Enabled: false}); err == nil {
		t.Fatal("failure hidden")
	}
	f.fail = false
	f.noConfirm = true
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	if _, err := m.Change(ctx, "0x2", Change{Action: "floating", Enabled: false}); err == nil {
		t.Fatal("unconfirmed change accepted")
	}
}
func TestControlCommandsNeverUseFocusedWindowFallback(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("PATH", dir)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "fixture")
	t.Setenv("CONTROL_LOG", log)
	tool := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CONTROL_LOG\"\ncase \"$2\" in hl.dsp.*) echo ok;; *) exit 1;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "hyprctl"), []byte(tool), 0700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []Change{{Action: "workspace", Workspace: 2}, {Action: "floating", Enabled: true}, {Action: "maximized", Enabled: true}, {Action: "geometry", Geometry: Geometry{X: -50, Y: 20, Width: 300, Height: 200}}} {
		if err := (Hyprland{}).Apply(context.Background(), "0x123", c); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(log)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if !strings.Contains(line, "address:0x123") || strings.HasPrefix(line, "dispatch fullscreen ") || strings.HasPrefix(line, "dispatch fullscreenstate ") {
			t.Fatal("unsafe selector", line)
		}
	}
}
