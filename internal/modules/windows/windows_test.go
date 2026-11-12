package windows

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

type fakeDriver struct {
	entries                          []Window
	active                           string
	reads, focuses                   int
	readError, focusError, noConfirm bool
}

func (f *fakeDriver) Read(context.Context) ([]Window, string, error) {
	f.reads++
	if f.readError {
		return nil, "", fmt.Errorf("compositor unavailable")
	}
	return f.entries, f.active, nil
}
func (f *fakeDriver) Focus(_ context.Context, address string) error {
	f.focuses++
	if f.focusError {
		return fmt.Errorf("focus failed")
	}
	if !f.noConfirm {
		f.active = address
	}
	return nil
}
func TestSnapshotStableSortedBoundedAndPrivate(t *testing.T) {
	f := &fakeDriver{entries: []Window{{Address: "0x2", Class: "Zed", Title: "second", Workspace: "2"}, {Address: "0x1", Class: "Alpha", Title: "first\nplain <b>text</b>", Workspace: "1"}, {Address: "not-an-address", Title: "invalid"}}, active: "0x2"}
	m := New(f)
	m.Initialize(context.Background())
	m.Start(context.Background())
	if f.reads != 0 {
		t.Fatal("startup queried compositor")
	}
	first, err := m.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Windows) != 2 || first.Windows[0].Address != "0x1" || !first.Windows[1].Active || strings.Contains(first.Windows[0].Title, "\n") {
		t.Fatal("incorrect filtering/sorting/labels")
	}
	again, _ := m.Snapshot(context.Background())
	if !reflect.DeepEqual(first, again) {
		t.Fatal("unchanged snapshot revised")
	}
	first.Windows[0].Title = "client mutation"
	third, _ := m.Snapshot(context.Background())
	if third.Windows[0].Title == "client mutation" {
		t.Fatal("snapshot aliases private state")
	}
	f.entries[0].Title = "new title"
	changed, _ := m.Snapshot(context.Background())
	if changed.Revision <= third.Revision {
		t.Fatal("metadata change did not revise")
	}
	m.Stop(context.Background())
	if len(m.last.Windows) != 0 {
		t.Fatal("shutdown retained titles")
	}
}
func TestFocusValidatesLiveTargetAndConfirmation(t *testing.T) {
	f := &fakeDriver{entries: []Window{{Address: "0x1", Title: "target"}}}
	m := New(f)
	snap, err := m.Focus(context.Background(), "0x1")
	if err != nil || !snap.Windows[0].Active || f.focuses != 1 {
		t.Fatal("valid focus failed")
	}
	f.entries = nil
	if _, err = m.Focus(context.Background(), "0x1"); err == nil || f.focuses != 1 {
		t.Fatal("stale window dispatched")
	}
	for _, address := range []string{"", "class:.*", "0x1;exec bad", "0x", "0x12345678901234567", "0xZZ"} {
		if _, err = m.Focus(context.Background(), address); err == nil {
			t.Fatal("invalid selector accepted")
		}
	}
	f.entries = []Window{{Address: "0x2"}}
	f.noConfirm = true
	if _, err = m.Focus(context.Background(), "0x2"); err == nil {
		t.Fatal("unconfirmed focus succeeded")
	}
	f.noConfirm = false
	f.focusError = true
	if _, err = m.Focus(context.Background(), "0x2"); err == nil {
		t.Fatal("dispatch error hidden")
	}
	f.readError = true
	calls := f.focuses
	if _, err = m.Focus(context.Background(), "0x2"); err == nil || f.focuses != calls {
		t.Fatal("unavailable compositor dispatched")
	}
}
func TestSnapshotLimitsAndFrameBudget(t *testing.T) {
	f := &fakeDriver{}
	for i := 0; i < MaxWindows+5; i++ {
		f.entries = append(f.entries, Window{Address: fmt.Sprintf("0x%x", i+1), Title: strings.Repeat("<", 1000), Class: strings.Repeat("λ", 100), Workspace: strings.Repeat("\"", 100)})
	}
	snap, err := New(f).Snapshot(context.Background())
	if err != nil || len(snap.Windows) != MaxWindows || !snap.Truncated {
		t.Fatal("window count not bounded")
	}
	for _, window := range snap.Windows {
		if len(window.Title) > 512 || len(window.Class) > 128 || len(window.Workspace) > 64 || !utf8.ValidString(window.Class) {
			t.Fatal("label bounds invalid")
		}
	}
	data, err := json.Marshal(snap)
	if err != nil || len(data) >= 1024*1024 {
		t.Fatal("snapshot exceeds frame cap")
	}
}
func TestHyprlandClientParsing(t *testing.T) {
	data := `[{"address":"0x1","mapped":true,"title":"normal","initialClass":"fallback","workspace":{"name":"1"}}, {"address":"0x2","mapped":false}, {"address":"0x3","mapped":true,"hidden":true}, {"address":"0x4","mapped":true,"acceptsInput":false}, {"address":"0x5","mapped":true,"workspace":{"name":"special:scratch"}}]`
	windows, err := parseClients([]byte(data))
	if err != nil || len(windows) != 1 || windows[0].Class != "fallback" {
		t.Fatal("parser filtering/fallback failed")
	}
	for _, data := range []string{`invalid`, `null`, `{"address":"0x1"}`} {
		if _, err := parseClients([]byte(data)); err == nil {
			t.Fatal("malformed clients accepted")
		}
	}
	if windows, err := parseClients([]byte(`[]`)); err != nil || len(windows) != 0 {
		t.Fatal("empty desktop failed")
	}
}
func TestBoundedCompositorOutput(t *testing.T) {
	b := &boundedOutput{}
	b.Write([]byte(strings.Repeat("x", maxReplyBytes+5)))
	b.Write([]byte("more"))
	if !b.overflow || b.Len() != maxReplyBytes {
		t.Fatal("output escaped bounds")
	}
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")
	if _, err := command(context.Background(), "-j", "clients"); err == nil {
		t.Fatal("missing session silently selected another compositor")
	}
}
