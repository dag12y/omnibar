package fileshelf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

type fakeDesktop struct {
	copied, opened string
	fail           bool
}

func (f *fakeDesktop) CopyPath(_ context.Context, path string) error {
	if f.fail {
		return fmt.Errorf("copy unavailable")
	}
	f.copied = path
	return nil
}
func (f *fakeDesktop) OpenFolder(_ context.Context, path string) error {
	if f.fail {
		return fmt.Errorf("opener unavailable")
	}
	f.opened = path
	return nil
}
func fixture(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("original test data"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestShelfBatchDedupAndOriginals(t *testing.T) {
	dir := t.TempDir()
	a := fixture(t, dir, "a with space # λ.txt")
	b := fixture(t, dir, "b.txt")
	m := New(&fakeDesktop{})
	uri := (&url.URL{Scheme: "file", Path: a}).String()
	snap, err := m.Add([]string{uri, b, dir, a})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Entries) != 3 || snap.Entries[0].Path != a || snap.Entries[2].Directory != true {
		t.Fatal("batch order, URI decoding or dedup failed")
	}
	id := snap.Entries[0].ID
	snap.Entries[0].Path = "changed"
	if m.Snapshot().Entries[0].Path != a {
		t.Fatal("snapshot aliases private state")
	}
	snap, err = m.Add([]string{a})
	if err != nil || snap.Entries[0].ID != id || len(snap.Entries) != 3 {
		t.Fatal("duplicate changed identity")
	}
	if _, err = m.Add([]string{b, filepath.Join(dir, "missing")}); err == nil {
		t.Fatal("accepted partly invalid batch")
	}
	if m.Snapshot().Entries[0].ID != id {
		t.Fatal("invalid batch changed shelf")
	}
	if _, err = m.Remove(id); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Remove(id); err == nil {
		t.Fatal("accepted stale ID")
	}
	m.Clear()
	m.Stop(context.Background())
	for _, path := range []string{a, b} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "original test data" {
			t.Fatal("original changed")
		}
	}
}
func TestLocalPathValidation(t *testing.T) {
	for _, value := range []string{"relative", "~/file", "https://example.com/file", "file://remote/tmp/a", "file:///tmp/a?query", "file:///tmp/a#fragment", "file:///tmp/%FF", "/tmp/a\nline", "/tmp/a\x00", string([]byte{'/', 0xff}), "file:opaque", strings.Repeat("x", MaxPathBytes*3+17)} {
		if _, err := localPath(value); err == nil {
			t.Errorf("accepted invalid path %q", value)
		}
	}
	for input, want := range map[string]string{"file://localhost/tmp/a%20b": "/tmp/a b", "/tmp/a/../b": "/tmp/b", "file:///tmp/%23x": "/tmp/#x"} {
		got, err := localPath(input)
		if err != nil || got != want {
			t.Fatalf("path normalization: %q %v", got, err)
		}
	}
}
func TestShelfUnavailableRecoveryAndActionValidation(t *testing.T) {
	dir := t.TempDir()
	path := fixture(t, dir, "fixture.txt")
	f := &fakeDesktop{}
	m := New(f)
	snap, _ := m.Add([]string{path, dir})
	id := snap.Entries[0].ID
	m.CopyPath(context.Background(), id)
	if f.copied != path {
		t.Fatal("copy altered path")
	}
	m.OpenFolder(context.Background(), id)
	if f.opened != dir {
		t.Fatal("file opened instead of containing folder")
	}
	m.OpenFolder(context.Background(), snap.Entries[1].ID)
	if f.opened != dir {
		t.Fatal("directory target changed")
	}
	os.Remove(path)
	snap = m.Snapshot()
	if snap.Entries[0].Available || snap.Entries[0].Size != 0 {
		t.Fatal("missing file still available")
	}
	f.copied = ""
	f.opened = ""
	if _, err := m.CopyPath(context.Background(), id); err == nil {
		t.Fatal("copied missing path")
	}
	if _, err := m.OpenFolder(context.Background(), id); err == nil {
		t.Fatal("opened missing item")
	}
	if f.copied != "" || f.opened != "" {
		t.Fatal("desktop helper invoked for missing item")
	}
	fixture(t, dir, "fixture.txt")
	if !m.Snapshot().Entries[0].Available {
		t.Fatal("restored file did not recover")
	}
	f.fail = true
	before := m.Snapshot().Revision
	if _, err := m.CopyPath(context.Background(), id); err == nil {
		t.Fatal("copy failure hidden")
	}
	if _, err := m.OpenFolder(context.Background(), id); err == nil {
		t.Fatal("open failure hidden")
	}
	if m.Snapshot().Revision != before {
		t.Fatal("failed action mutated shelf")
	}
	m.Stop(context.Background())
	if len(m.Snapshot().Entries) != 0 {
		t.Fatal("shutdown retained shelf")
	}
}
func TestShelfRejectsSpecialFilesAndBrokenSymlinks(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	syscall.Mkfifo(fifo, 0600)
	broken := filepath.Join(dir, "broken")
	os.Symlink(filepath.Join(dir, "absent"), broken)
	m := New(nil)
	for _, path := range []string{fifo, broken} {
		if _, err := m.Add([]string{path}); err == nil {
			t.Fatal("accepted unsupported file")
		}
	}
	file := fixture(t, dir, "real")
	link := filepath.Join(dir, "link")
	os.Symlink(file, link)
	snap, err := m.Add([]string{link})
	if err != nil || snap.Entries[0].Path != link {
		t.Fatal("symlink reference failed")
	}
}
func TestShelfCountAndByteBounds(t *testing.T) {
	dir := t.TempDir()
	m := New(nil)
	paths := []string{}
	for i := 0; i < MaxEntries; i++ {
		paths = append(paths, fixture(t, dir, fmt.Sprint(i)))
	}
	if _, err := m.Add(paths); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add([]string{fixture(t, dir, "extra")}); err == nil {
		t.Fatal("count limit exceeded")
	}
	if len(m.Snapshot().Entries) != MaxEntries {
		t.Fatal("limit failure evicted items")
	}
	// Long nested directory paths exercise the byte cap without large file data.
	long := dir
	for len(long) < 900 {
		long = filepath.Join(long, strings.Repeat("<", 100))
	}
	os.MkdirAll(long, 0700)
	m.Clear()
	for i := 0; i < MaxEntries; i++ {
		path := fixture(t, long, fmt.Sprint(i))
		_, err := m.Add([]string{path})
		if err != nil {
			if i == 0 {
				t.Fatal(err)
			}
			break
		}
	}
	total := 0
	for _, entry := range m.Snapshot().Entries {
		total += len(entry.Path)
	}
	if total > MaxTotalPathBytes || total < MaxTotalPathBytes-1100 {
		t.Fatal("path budget not enforced")
	}
	data, err := json.Marshal(m.Snapshot())
	if err != nil || len(data) >= 1024*1024 {
		t.Fatal("snapshot exceeds frame limit")
	}
}
