package clipboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func historyWithStore(t *testing.T, store FavoritesStore) *Module {
	t.Helper()
	m := NewWithStore(&fakeDriver{}, store)
	if err := m.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().Collecting {
		t.Fatal("restore started collection")
	}
	// Feed synthetic selections directly; no desktop watcher is started.
	m.collecting = true
	t.Cleanup(func() { m.Stop(context.Background()) })
	return m
}
func addText(m *Module, text string) string {
	m.ingest(Capture{State: "data", Text: text})
	for _, entry := range m.Snapshot().Entries {
		if entry.Text == text {
			return entry.ID
		}
	}
	return ""
}
func TestFavoritesPersistOnlyExplicitlySavedText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "clipboard-favorites.json")
	store := FileStore{Path: path}
	m := historyWithStore(t, store)
	id := addText(m, "Favorite\nexact λ\ttext")
	addText(m, "ordinary recent text")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("collection wrote to disk")
	}
	snap, err := m.SetFavorite(id, true)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Entries[0].Favorite || snap.Entries[0].ID != id {
		t.Fatal("favorite not first or ID changed")
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "ordinary recent") {
		t.Fatal("ordinary history persisted")
	}
	for name, want := range map[string]os.FileMode{path: 0600, filepath.Dir(path): 0700} {
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("incorrect permissions: %s", name)
		}
	}
	for i := 0; i < 80; i++ {
		addText(m, fmt.Sprint("new copy ", i))
	}
	if !m.Snapshot().Entries[0].Favorite {
		t.Fatal("favorite evicted under pressure")
	}
	if len(m.Clear().Entries) != 1 {
		t.Fatal("Clear removed saved favorite")
	}
	m.Stop(context.Background())
	restored := historyWithStore(t, store)
	if len(restored.Snapshot().Entries) != 1 || restored.Snapshot().Entries[0].Text != "Favorite\nexact λ\ttext" {
		t.Fatal("favorite did not survive restart")
	}
	restored.SetCollecting(false)
	if restored.Snapshot().Collecting {
		t.Fatal("restored collection started itself")
	}
	rid := restored.Snapshot().Entries[0].ID
	if _, err := restored.SetFavorite(rid, false); err != nil {
		t.Fatal(err)
	}
	if len(restored.Snapshot().Entries) != 1 || restored.Snapshot().Entries[0].Favorite {
		t.Fatal("Unsave must leave recent copy")
	}
	third := historyWithStore(t, store)
	if len(third.Snapshot().Entries) != 0 {
		t.Fatal("unsaved text survived restart")
	}
}
func TestFavoriteRemovalAndDedup(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "favorites.json")}
	m := historyWithStore(t, store)
	id := addText(m, "saved")
	m.SetFavorite(id, true)
	if got := addText(m, "saved"); got != id {
		t.Fatal("duplicate changed identity")
	}
	if len(m.Snapshot().Entries) != 1 || !m.Snapshot().Entries[0].Favorite {
		t.Fatal("duplicate lost favorite")
	}
	if _, err := m.Delete(id); err != nil {
		t.Fatal(err)
	}
	if len(historyWithStore(t, store).Snapshot().Entries) != 0 {
		t.Fatal("deleted favorite still saved")
	}
	if _, err := m.SetFavorite(id, true); err == nil {
		t.Fatal("stale favorite ID accepted")
	}
}

type failedStore struct{}

func (failedStore) Load() ([]Entry, error) { return nil, nil }
func (failedStore) Save([]Entry) error     { return fmt.Errorf("disk unavailable") }
func TestFailedFavoriteSaveDoesNotChangeState(t *testing.T) {
	m := historyWithStore(t, failedStore{})
	id := addText(m, "fixture")
	before := m.Snapshot()
	if _, err := m.SetFavorite(id, true); err == nil {
		t.Fatal("save failure hidden")
	}
	if got := m.Snapshot(); got.Revision != before.Revision || got.Entries[0].Favorite {
		t.Fatal("failed save changed state")
	}
	// Simulate an already-saved item and test removal rollback too.
	m.entries[0].Favorite = true
	if _, err := m.SetFavorite(id, false); err == nil {
		t.Fatal("unsave failure hidden")
	}
	if _, err := m.Delete(id); err == nil {
		t.Fatal("delete failure hidden")
	}
	if !m.Snapshot().Entries[0].Favorite {
		t.Fatal("failed removal lost favorite")
	}
}
func TestFavoriteLimitsAndFrameBudget(t *testing.T) {
	m := historyWithStore(t, FileStore{Path: filepath.Join(t.TempDir(), "favorites.json")})
	for i := 0; i < MaxFavorites; i++ {
		id := addText(m, fmt.Sprint(i))
		if _, err := m.SetFavorite(id, true); err != nil {
			t.Fatal(err)
		}
	}
	id := addText(m, "one too many")
	if _, err := m.SetFavorite(id, true); err == nil {
		t.Fatal("favorite count exceeded")
	}
	big := historyWithStore(t, FileStore{Path: filepath.Join(t.TempDir(), "favorites.json")})
	for i := 0; i < 2; i++ {
		id := addText(big, fmt.Sprint(i)+strings.Repeat("<", MaxItemBytes-1))
		if _, err := big.SetFavorite(id, true); err != nil {
			t.Fatal(err)
		}
	}
	id = addText(big, "extra byte")
	if _, err := big.SetFavorite(id, true); err == nil {
		t.Fatal("favorite byte limit exceeded")
	}
	for i := 0; i < 20; i++ {
		addText(big, fmt.Sprint(i)+strings.Repeat("<", MaxItemBytes-2))
	}
	if big.totalBytes > MaxTotalBytes {
		t.Fatal("total history byte cap exceeded")
	}
	data, err := json.Marshal(big.Snapshot())
	if err != nil || len(data) >= 1024*1024 {
		t.Fatal("snapshot exceeds IPC budget")
	}
	favorites := 0
	for _, entry := range big.Snapshot().Entries {
		if entry.Favorite {
			favorites++
		}
	}
	if favorites != 2 {
		t.Fatal("byte eviction removed favorite")
	}
}
func TestInvalidFavoritesFileIsPreserved(t *testing.T) {
	for _, data := range []string{`not json`, `{"version":2,"entries":[]}`, strings.Repeat("x", maxFavoritesFileBytes+1), `{"version":1,"entries":[{"text":"hello","copied_at":"invalid"}]}`} {
		t.Run(fmt.Sprint(len(data)), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "favorites.json")
			os.WriteFile(path, []byte(data), 0600)
			m := historyWithStore(t, FileStore{Path: path})
			if m.Snapshot().CanFavorite || m.Snapshot().FavoritesError == "" {
				t.Fatal("bad file not reported")
			}
			id := addText(m, "recent remains usable")
			if _, err := m.SetFavorite(id, true); err == nil {
				t.Fatal("overwrote invalid file")
			}
			got, _ := os.ReadFile(path)
			if string(got) != data {
				t.Fatal("damaged file changed")
			}
		})
	}
}
func TestFavoritesFileRejectsSymlinkAndPublicPermissions(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	os.WriteFile(target, []byte(`{"version":1,"entries":[]}`), 0644)
	if _, err := (FileStore{Path: target}).Load(); err == nil {
		t.Fatal("accepted public file")
	}
	os.Chmod(target, 0600)
	link := filepath.Join(dir, "link")
	os.Symlink(target, link)
	if _, err := (FileStore{Path: link}).Load(); err == nil {
		t.Fatal("followed symlink")
	}
}
