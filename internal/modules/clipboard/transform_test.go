package clipboard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTransformTextSemanticsAndLimits(t *testing.T) {
	for _, tc := range []struct{ operation, input, want string }{
		{"trim", "\u00a0 \n  One  two\n three\t \u2003", "One  two\n three"},
		{"lowercase", "HELLO Ελληνικά ÄÖÜ ПрИвЕт", "hello ελληνικά äöü привет"},
		{"uppercase", "Hello Ελληνικά äöü ПрИвЕт", "HELLO ΕΛΛΗΝΙΚΆ ÄÖÜ ПРИВЕТ"},
		{"remove_line_breaks", "one\r\ntwo\nthree\rfour\u2028five\u2029six", "one two three four five six"},
		{"remove_line_breaks", "a\n\n b\t c", "a   b\t c"},
		{"trim", "already trimmed", "already trimmed"},
	} {
		got, err := transformText(tc.input, tc.operation)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q (%v), want %q", tc.operation, got, err, tc.want)
		}
	}
	for _, tc := range []struct{ operation, text string }{
		{"unknown", "fixture"}, {"trim", " \n\t\u2003"},
		{"lowercase", strings.Repeat("Ⱥ", MaxItemBytes/2)},
	} {
		if _, err := transformText(tc.text, tc.operation); err == nil {
			t.Errorf("accepted invalid/empty/oversized %s", tc.operation)
		}
	}
}

type transformDriver struct {
	copied string
	calls  int
	fail   bool
}

func (f *transformDriver) Watch(ctx context.Context, _ func(Capture)) error {
	<-ctx.Done()
	return ctx.Err()
}
func (f *transformDriver) Copy(_ context.Context, text string) error {
	f.calls++
	if f.fail {
		return fmt.Errorf("clipboard unavailable")
	}
	f.copied = text
	return nil
}
func TestTransformCopyPreservesSourceAndFavoriteFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "favorites.json")
	driver := &transformDriver{}
	m := NewWithStore(driver, FileStore{Path: file})
	m.Initialize(context.Background())
	m.Start(context.Background())
	defer m.Stop(context.Background())
	source := "  Mixed Case\r\nSecond Line  "
	m.collecting = true
	m.ingest(Capture{State: "data", Text: source})
	m.SetCollecting(false)
	id := m.Snapshot().Entries[0].ID
	if _, err := m.SetFavorite(id, true); err != nil {
		t.Fatal(err)
	}
	before := m.Snapshot()
	saved, _ := os.ReadFile(file)
	for operation, want := range map[string]string{"trim": "Mixed Case\r\nSecond Line", "lowercase": "  mixed case\r\nsecond line  ", "uppercase": "  MIXED CASE\r\nSECOND LINE  ", "remove_line_breaks": "  Mixed Case Second Line  "} {
		after, err := m.TransformCopy(context.Background(), id, operation)
		if err != nil {
			t.Fatal(err)
		}
		if driver.copied != want {
			t.Fatalf("%s copy mismatch", operation)
		}
		if !reflect.DeepEqual(after, before) {
			t.Fatal("transform changed source, favorite, revision or collection state")
		}
		data, _ := os.ReadFile(file)
		if string(data) != string(saved) {
			t.Fatal("transform rewrote saved favorite")
		}
	}
	driver.fail = true
	if _, err := m.TransformCopy(context.Background(), id, "uppercase"); err == nil {
		t.Fatal("driver failure hidden")
	}
	if !reflect.DeepEqual(m.Snapshot(), before) {
		t.Fatal("failed copy changed history")
	}
	calls := driver.calls
	for _, tc := range []struct{ id, operation string }{{"stale", "trim"}, {id, "unknown"}} {
		if _, err := m.TransformCopy(context.Background(), tc.id, tc.operation); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	if driver.calls != calls {
		t.Fatal("invalid transform invoked driver")
	}
}
func TestEmptyTransformLeavesClipboardAlone(t *testing.T) {
	f := &transformDriver{copied: "existing clipboard"}
	m := New(f)
	m.collecting = true
	m.ingest(Capture{State: "data", Text: " \n\t"})
	id := m.Snapshot().Entries[0].ID
	if _, err := m.TransformCopy(context.Background(), id, "trim"); err == nil {
		t.Fatal("empty trim accepted")
	}
	if f.calls != 0 || f.copied != "existing clipboard" {
		t.Fatal("empty transform changed clipboard")
	}
}
