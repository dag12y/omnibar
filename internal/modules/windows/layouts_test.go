package windows

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type layoutDriver struct {
	controlDriver
	topology string
	started  chan struct{}
	release  chan struct{}
}

func (f *layoutDriver) Topology(ctx context.Context) (string, error) {
	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
		select {
		case <-f.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return f.topology, nil
}
func layoutFixture(t *testing.T) (*Module, *layoutDriver, LayoutFile) {
	t.Helper()
	f := &layoutDriver{controlDriver: controlDriver{fakeDriver: fakeDriver{entries: []Window{
		{Address: "0x1", Identity: "a", Class: "Editor", Title: "Alpha", WorkspaceID: 1, Floating: true, Geometry: Geometry{10, 20, 300, 200}},
		{Address: "0x2", Identity: "b", Class: "Editor", Title: "Beta", WorkspaceID: 1, Floating: true, Geometry: Geometry{350, 20, 300, 200}},
	}}}, topology: strings.Repeat("a", 64)}
	store := LayoutFile{Path: filepath.Join(t.TempDir(), "data/layouts.json")}
	m := NewWithLayoutStore(f, store)
	m.Initialize(context.Background())
	return m, f, store
}
func TestLayoutSaveRestorePersistenceAndSummaryCloning(t *testing.T) {
	m, f, store := layoutFixture(t)
	ctx := context.Background()
	s, err := m.SaveLayout(ctx, "Desk", 1)
	if err != nil {
		t.Fatal(err)
	}
	id := s.Layouts[0].ID
	if !ValidLayoutID(id) || s.Layouts[0].Count != 2 {
		t.Fatal("invalid layout summary")
	}
	s.Layouts[0].Name = "tampered"
	if m.last.Layouts[0].Name != "Desk" {
		t.Fatal("summary alias")
	}
	if s, err = m.SaveLayout(ctx, "Desk", 1); err != nil || len(s.Layouts) != 1 || s.Layouts[0].ID != id {
		t.Fatal("same name did not update", err)
	}
	data, _ := os.ReadFile(store.Path)
	if strings.Contains(string(data), `"address"`) {
		t.Fatal("ephemeral addresses persisted")
	}
	f.entries[0].WorkspaceID = 3
	f.entries[1].Floating = false
	if _, err = m.RestoreLayout(ctx, id); err != nil {
		t.Fatal(err)
	}
	if f.entries[0].WorkspaceID != 1 || !f.entries[1].Floating || f.entries[1].Geometry != (Geometry{350, 20, 300, 200}) {
		t.Fatal("arrangement not restored")
	}
	reads, changes := f.reads, len(f.changes)
	other := NewWithLayoutStore(f, store)
	other.Initialize(ctx)
	if f.reads != reads || len(f.changes) != changes {
		t.Fatal("startup applied or read desktop state")
	}
	s, _ = other.Snapshot(ctx)
	if len(s.Layouts) != 1 || s.Layouts[0].ID != id {
		t.Fatal("layout did not reload")
	}
	if _, err = other.DeleteLayout(ctx, id); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || len(loaded) != 0 {
		t.Fatal("delete did not persist", err)
	}
}
func TestLayoutPreflightAndPartialFailures(t *testing.T) {
	for _, mode := range []string{"missing", "ambiguous", "monitor", "restricted", "fullscreen", "identity", "truncated", "dispatch"} {
		t.Run(mode, func(t *testing.T) {
			m, f, _ := layoutFixture(t)
			ctx := context.Background()
			s, err := m.SaveLayout(ctx, "Desk", 1)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "missing":
				f.entries = f.entries[:1]
			case "ambiguous":
				f.entries[1].Title = "Alpha"
			case "monitor":
				f.topology = strings.Repeat("b", 64)
			case "restricted":
				f.entries[0].Restricted = true
			case "fullscreen":
				f.entries[0].Fullscreen = 2
			case "identity":
				f.entries[0].Identity = ""
			case "truncated":
				for i := 0; i < MaxWindows; i++ {
					f.entries = append(f.entries, Window{Address: fmt.Sprintf("0x%x", i+100), Class: "Other", Title: fmt.Sprint(i)})
				}
			case "dispatch":
				f.entries[0].WorkspaceID = 2
				f.fail = true
			}
			_, err = m.RestoreLayout(ctx, s.Layouts[0].ID)
			if err == nil {
				t.Fatal("invalid restore accepted")
			}
			if mode != "dispatch" && len(f.changes) != 0 {
				t.Fatal("preflight failure changed desktop")
			}
			if mode == "dispatch" && !strings.Contains(err.Error(), "partially") {
				t.Fatal("partial failure not explained", err)
			}
		})
	}
}
func TestLayoutStorageBoundsCorruptionAndFailurePreserveState(t *testing.T) {
	m, f, store := layoutFixture(t)
	ctx := context.Background()
	s, err := m.SaveLayout(ctx, "Desk", 1)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(store.Path)
	info, _ := os.Stat(store.Path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("layout file not private")
	}
	os.Chmod(store.Path, 0644)
	if _, err := m.SaveLayout(ctx, "Other", 1); err == nil {
		t.Fatal("unsafe file overwritten")
	}
	if len(m.layouts) != 1 || m.layouts[0].ID != s.Layouts[0].ID {
		t.Fatal("failed write changed memory")
	}
	data, _ := os.ReadFile(store.Path)
	if string(data) != string(original) {
		t.Fatal("failed write changed file")
	}
	os.Chmod(store.Path, 0600)
	os.WriteFile(store.Path, []byte("{broken"), 0600)
	broken := NewWithLayoutStore(f, store)
	broken.Initialize(ctx)
	snap, _ := broken.Snapshot(ctx)
	if snap.LayoutError == "" {
		t.Fatal("corrupt state hidden")
	}
	if _, err := broken.SaveLayout(ctx, "Desk", 1); err == nil {
		t.Fatal("corrupt original overwritten")
	}
	data, _ = os.ReadFile(store.Path)
	if string(data) != "{broken" {
		t.Fatal("corrupt file changed")
	}
	os.Remove(store.Path)
	linkTarget := filepath.Join(t.TempDir(), "target")
	os.WriteFile(linkTarget, original, 0600)
	os.Symlink(linkTarget, store.Path)
	if _, err := store.Load(); err == nil {
		t.Fatal("symlink read")
	}
	if err := store.Save(m.layouts); err == nil {
		t.Fatal("symlink replaced")
	}
	for _, name := range []string{"", " leading", "bad\nname", strings.Repeat("a", 65)} {
		if _, err := m.SaveLayout(ctx, name, 1); err == nil {
			t.Fatal("invalid layout name accepted")
		}
	}
	t.Setenv("XDG_DATA_HOME", "relative")
	if DefaultLayoutStore().(LayoutFile).Path != "" {
		t.Fatal("relative data directory accepted")
	}
}
func TestLayoutCountAndSerializedByteBounds(t *testing.T) {
	m, _, store := layoutFixture(t)
	ctx := context.Background()
	for i := 0; i < MaxLayouts; i++ {
		if _, err := m.SaveLayout(ctx, fmt.Sprintf("Desk %d", i), 1); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := os.ReadFile(store.Path)
	if _, err := m.SaveLayout(ctx, "Extra", 1); err == nil {
		t.Fatal("layout count unbounded")
	}
	loaded, _ := store.Load()
	for i := range loaded {
		loaded[i].Windows = nil
		for j := 0; j < 16; j++ {
			loaded[i].Windows = append(loaded[i].Windows, Placement{Class: "Editor", Title: strings.Repeat("<", 500) + fmt.Sprint(j)})
		}
	}
	if err := store.Save(loaded); err == nil {
		t.Fatal("serialized file bytes unbounded")
	}
	after, _ := os.ReadFile(store.Path)
	if string(after) != string(before) {
		t.Fatal("bounded rejection changed file")
	}
	var saved map[string]any
	if json.Unmarshal(before, &saved) != nil {
		t.Fatal("invalid persistent JSON")
	}
}

func TestRestoreDoesNotHoldSnapshotMutexAndRejectsConcurrentRestore(t *testing.T) {
	m, f, _ := layoutFixture(t)
	ctx := context.Background()
	saved, err := m.SaveLayout(ctx, "Desk", 1)
	if err != nil {
		t.Fatal(err)
	}
	f.started = make(chan struct{}, 1)
	f.release = make(chan struct{})
	done := make(chan error, 1)
	go func() { _, e := m.RestoreLayout(ctx, saved.Layouts[0].ID); done <- e }()
	<-f.started
	if _, e := m.RestoreLayout(ctx, saved.Layouts[0].ID); e == nil {
		t.Fatal("concurrent restore accepted")
	}
	read := make(chan error, 1)
	go func() { _, e := m.Snapshot(ctx); read <- e }()
	select {
	case e := <-read:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		close(f.release)
		t.Fatal("restore held the snapshot mutex")
	}
	close(f.release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
