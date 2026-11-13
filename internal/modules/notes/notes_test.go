package notes

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryStore struct {
	data          Data
	fail          bool
	loads, writes int
}

func (s *memoryStore) Load() (Data, error) {
	s.loads++
	d := s.data
	d.Notes = append([]Note{}, d.Notes...)
	return d, nil
}
func (s *memoryStore) Save(d Data) error {
	s.writes++
	if s.fail {
		return errors.New("disk full")
	}
	s.data = d
	return nil
}

type copier struct {
	text  string
	calls int
}

func (c *copier) Copy(_ context.Context, s string) error { c.text = s; c.calls++; return nil }
func fixture(t *testing.T) (*Module, *memoryStore, *copier) {
	t.Helper()
	s := &memoryStore{data: Data{Version: 1, Revision: 1, Notes: []Note{}}}
	c := &copier{}
	m := New(s, c)
	m.Initialize(context.Background())
	return m, s, c
}
func TestNotesPersistenceRevisionSearchAndCopy(t *testing.T) {
	m, s, c := fixture(t)
	first, err := m.Create("First", "Hello\nworld\tሰላም", "")
	if err != nil {
		t.Fatal(err)
	}
	n := *first.Note
	second, err := m.Create("Second", "other", "")
	if err != nil {
		t.Fatal(err)
	}
	n.Pinned = true
	updated, err := m.Update(n, "")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Notes[0].ID != n.ID || updated.Note.Revision <= n.Revision {
		t.Fatal("pin or revision lost")
	}
	if _, err = m.Update(n, ""); !errors.Is(err, ErrConflict) {
		t.Fatal("stale edit accepted", err)
	}
	if _, err = m.Delete(n.ID, n.Revision, ""); !errors.Is(err, ErrConflict) {
		t.Fatal("stale deletion accepted")
	}
	if _, err = m.Copy(context.Background(), n.ID, n.Revision, ""); !errors.Is(err, ErrConflict) || c.calls != 0 {
		t.Fatal("stale copy accepted")
	}
	n = *updated.Note
	if _, err = m.Copy(context.Background(), n.ID, n.Revision, ""); err != nil || c.text != n.Body {
		t.Fatal("exact body copy failed", err)
	}
	found := m.Snapshot("HELLO ሰላም")
	if len(found.Notes) != 1 || found.Notes[0].ID != n.ID {
		t.Fatal("body token search failed")
	}
	encoded, _ := json.Marshal(found)
	if strings.Contains(string(encoded), "Hello") {
		t.Fatal("snapshot leaks note body")
	}
	m2 := New(s, c)
	m2.Initialize(context.Background())
	got, err := m2.Get(n.ID, "")
	if err != nil || *got.Note != n {
		t.Fatal("restart lost note", err)
	}
	if _, err = m2.Delete(second.Note.ID, second.Note.Revision, ""); err != nil {
		t.Fatal(err)
	}
	if len(m2.Snapshot("").Notes) != 1 {
		t.Fatal("delete failed")
	}
}
func TestFailedWriteAndStaleConcurrentEditsPreserveNotes(t *testing.T) {
	m, s, _ := fixture(t)
	r, _ := m.Create("Draft", "original", "")
	n := *r.Note
	s.fail = true
	n.Body = "new"
	if _, err := m.Update(n, ""); err == nil {
		t.Fatal("write failure ignored")
	}
	got, _ := m.Get(n.ID, "")
	if got.Note.Body != "original" || got.Note.Revision != r.Note.Revision {
		t.Fatal("failed write changed memory")
	}
	s.fail = false
	a, b := n, n
	a.Body = "a"
	b.Body = "b"
	results := make(chan error, 2)
	go func() { _, e := m.Update(a, ""); results <- e }()
	go func() { _, e := m.Update(b, ""); results <- e }()
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent edit overwrote newer note")
	}
}
func TestLimitsAndInvalidText(t *testing.T) {
	m, _, _ := fixture(t)
	for _, tc := range [][2]string{{" ", "text"}, {"Bad\nTitle", "text"}, {"ok", "a\x00b"}, {"ok", string([]byte{0xff})}, {"ok", strings.Repeat("a", MaxBodyBytes+1)}} {
		if _, e := m.Create(tc[0], tc[1], ""); e == nil {
			t.Fatal("invalid text accepted")
		}
	}
	for range MaxNotes {
		if _, e := m.Create("note", "", ""); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := m.Create("one too many", "", ""); e == nil {
		t.Fatal("count limit ignored")
	}
}
func TestFileStorePrivateAtomicCorruptAndUnsafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private/notes.json")
	store := FileStore{Path: path}
	m := New(store, nil)
	m.Initialize(context.Background())
	r, err := m.Create("Persistent", "private text", "")
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("not private")
	}
	dir, _ := os.Stat(filepath.Dir(path))
	if dir.Mode().Perm() != 0700 {
		t.Fatal("directory not private")
	}
	original, _ := os.ReadFile(path)
	os.Chmod(path, 0644)
	n := *r.Note
	n.Body = "changed"
	if _, e := m.Update(n, ""); e == nil {
		t.Fatal("unsafe file overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("original changed")
	}
	os.Chmod(path, 0600)
	os.WriteFile(path, []byte("broken original"), 0600)
	broken := New(store, nil)
	broken.Initialize(context.Background())
	if broken.Snapshot("").StorageError == "" {
		t.Fatal("corruption hidden")
	}
	if _, e := broken.Create("Overwrite", "", ""); e == nil {
		t.Fatal("corrupt original overwritten")
	}
	after, _ = os.ReadFile(path)
	if string(after) != "broken original" {
		t.Fatal("corrupt data lost")
	}
	os.Remove(path)
	os.Symlink(filepath.Join(t.TempDir(), "missing"), path)
	if _, e := store.Load(); e == nil {
		t.Fatal("symlink followed")
	}
	if e := store.Save(Data{Version: 1, Revision: 1, Notes: []Note{}}); e == nil {
		t.Fatal("symlink replaced")
	}
}
func TestEncodedFileLimitAndStrictRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")
	s := FileStore{Path: path}
	d := Data{Version: 1, Revision: 1, Notes: []Note{}}
	if err := s.Save(d); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(path)
	for i := range 12 {
		d.Notes = append(d.Notes, Note{ID: strings.Repeat("0", 30) + string("0123456789ab"[i]) + "0", Title: "large", Body: strings.Repeat("<", MaxBodyBytes), Revision: 1})
	}
	if err := s.Save(d); err == nil {
		t.Fatal("escaped JSON exceeded file cap")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("bounded failure replaced data")
	}
	for _, data := range []string{`{"version":1,"revision":1,"notes":[],"unknown":1}`, `{"version":1,"revision":0,"notes":[]}`, `{"version":1,"revision":1,"notes":[]} {}`, strings.Repeat(" ", MaxFileBytes+1), "{\"version\":1,\"revision\":1,\"notes\":[{\"id\":\"00000000000000000000000000000000\",\"title\":\"bad\",\"body\":\"\xff\",\"pinned\":false,\"revision\":1}]}"} {
		os.WriteFile(path, []byte(data), 0600)
		if _, err := s.Load(); err == nil {
			t.Fatal("invalid file accepted")
		}
	}
	t.Setenv("XDG_DATA_HOME", "relative")
	if _, e := DefaultStore().Load(); e == nil {
		t.Fatal("relative data path accepted")
	}
}
