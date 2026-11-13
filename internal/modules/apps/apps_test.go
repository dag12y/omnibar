package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fixture(t *testing.T, root, id, body string) string {
	t.Helper()
	path := filepath.Join(root, "applications", id)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func desktopEnv(t *testing.T) (string, string) {
	t.Helper()
	user, system := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", user)
	t.Setenv("XDG_DATA_DIRS", system)
	t.Setenv("XDG_CURRENT_DESKTOP", "Hyprland:Test")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	return user, system
}

const validEntry = "[Desktop Entry]\nType=Application\nName=Fixture\nExec=/bin/true\n"

func TestDesktopPrecedenceVisibilityAndMetadata(t *testing.T) {
	u, s := desktopEnv(t)
	fixture(t, s, "override.desktop", validEntry)
	fixture(t, u, "override.desktop", "[Desktop Entry]\nHidden=true\n")
	for id, extra := range map[string]string{"hidden": "Hidden=true", "no-display": "NoDisplay=true", "wrong-desktop": "OnlyShowIn=GNOME;", "excluded": "NotShowIn=Hyprland;", "missing-exec": "TryExec=omnibar-nonexistent-fixture-command", "wrong-type": "Type=Link"} {
		fixture(t, u, id+".desktop", validEntry+extra+"\n")
	}
	fixture(t, u, "nested/app.desktop", validEntry+"Name[en_US]=Localized\nComment=Line\\nTwo\nKeywords=tools;test;\nOnlyShowIn=Test;\n[Desktop Action Hidden]\nName=Wrong\n")
	entries, truncated, err := (Desktop{}).Read(context.Background())
	if err != nil || truncated || len(entries) != 1 {
		t.Fatalf("unexpected entries: %+v, %v", entries, err)
	}
	a := entries[0]
	if a.ID != "nested-app.desktop" || a.Name != "Localized" || a.Description != "Line Two" || !strings.Contains(a.Keywords, "tools;") {
		t.Fatalf("wrong metadata: %+v", a)
	}
}
func TestDesktopBoundsMalformedAndCancellation(t *testing.T) {
	u, _ := desktopEnv(t)
	if err := os.MkdirAll(filepath.Join(u, "applications"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(u, "applications/fifo.desktop"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture(t, u, "oversized.desktop", validEntry+strings.Repeat("x", maxDesktopBytes))
	fixture(t, u, "invalid.desktop", validEntry+"\xff")
	fixture(t, u, "no-exec.desktop", "[Desktop Entry]\nType=Application\nName=No command\n")
	fixture(t, u, "dbus.desktop", "[Desktop Entry]\nType=Application\nName=Activated\nDBusActivatable=true\n")
	a, _, err := (Desktop{}).Read(context.Background())
	if err != nil || len(a) != 1 {
		t.Fatalf("unexpected: %v %v", a, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := (Desktop{}).Read(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
	for _, id := range []string{"../bad.desktop", "/tmp/app.desktop", "a\n.desktop", strings.Repeat("a", 257) + ".desktop", "anything"} {
		if ValidID(id) {
			t.Fatal("invalid ID accepted", id)
		}
	}
}

func TestLaunchDoesNotWaitForApplicationLifetime(t *testing.T) {
	u, _ := desktopEnv(t)
	fixture(t, u, "app.desktop", validEntry)
	bin := t.TempDir()
	// GIO-spawned apps can inherit its output descriptors after GIO exits.
	if err := os.WriteFile(filepath.Join(bin, "gio"), []byte("#!/bin/sh\n/bin/sleep 1 &\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := (Desktop{}).Launch(ctx, "app.desktop"); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("launcher waited for application output/lifetime")
	}
}

type fakeDriver struct {
	entries  []App
	launches int
	fail     bool
}

func (f *fakeDriver) Read(context.Context) ([]App, bool, error) {
	return append([]App{}, f.entries...), false, nil
}
func (f *fakeDriver) Launch(context.Context, string) error {
	f.launches++
	if f.fail {
		return fmt.Errorf("launch rejected")
	}
	return nil
}
func TestModuleLaunchSnapshotBoundsAndDisposal(t *testing.T) {
	f := &fakeDriver{}
	for i := 0; i < MaxApps+1; i++ {
		f.entries = append(f.entries, App{ID: strings.Repeat("<", 240) + fmt.Sprintf("%03d.desktop", i), Name: strings.Repeat("<&>", 300), Description: strings.Repeat("<&>", 300), Keywords: strings.Repeat("<&>", 300)})
	}
	m := New(f)
	snap, err := m.Snapshot(context.Background())
	if err != nil || !snap.Truncated || len(snap.Apps) != MaxApps {
		t.Fatal("bounds missing", err)
	}
	data, _ := json.Marshal(snap)
	if len(data) > 1000000 {
		t.Fatal("snapshot cannot fit IPC frame")
	}
	first := snap.Apps[0].ID
	snap.Apps[0].Name = "mutated"
	next, _ := m.Snapshot(context.Background())
	if next.Apps[0].Name == "mutated" || next.Revision != snap.Revision {
		t.Fatal("snapshot alias or unstable revision")
	}
	if _, err := m.Launch(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Launch(context.Background(), "missing.desktop"); err == nil || f.launches != 1 {
		t.Fatal("stale target launched")
	}
	f.fail = true
	if _, err := m.Launch(context.Background(), first); err == nil {
		t.Fatal("launch failure hidden")
	}
	f.entries = nil
	if _, err := m.Launch(context.Background(), first); err == nil || f.launches != 2 {
		t.Fatal("removed target launched")
	}
	m.Stop(context.Background())
	if len(m.last.Apps) != 0 {
		t.Fatal("cache retained")
	}
}
func TestDesktopExactLaunchAndStaleTarget(t *testing.T) {
	u, _ := desktopEnv(t)
	path := fixture(t, u, "app with spaces.desktop", validEntry)
	bin := t.TempDir()
	log := filepath.Join(bin, "args")
	t.Setenv("APP_TEST_LOG", log)
	if err := os.WriteFile(filepath.Join(bin, "gio"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$APP_TEST_LOG\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if err := (Desktop{}).Launch(context.Background(), "app with spaces.desktop"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(log)
	if string(data) != "launch\n"+path+"\n" {
		t.Fatalf("selector split or arbitrary command: %q", data)
	}
	os.Remove(path)
	if err := (Desktop{}).Launch(context.Background(), "app with spaces.desktop"); err == nil {
		t.Fatal("removed entry launched")
	}
}
