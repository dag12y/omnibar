package clipboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type forbiddenReader struct{}

func (forbiddenReader) Read([]byte) (int, error) {
	panic("sensitive or empty state must not read stdin")
}
func TestCaptureSkipsSensitiveBinaryInvalidAndOversizedData(t *testing.T) {
	for _, state := range []string{"sensitive", "nil", "clear", "unknown"} {
		var out bytes.Buffer
		if CaptureCommand(forbiddenReader{}, &out, state) != 0 {
			t.Fatal("helper failed")
		}
		var c Capture
		json.Unmarshal(out.Bytes(), &c)
		if c.Text != "" || c.State == "data" {
			t.Fatal("non-data state retained contents")
		}
	}
	for _, text := range []string{"", string([]byte{0xff}), "binary\x00data", "escape\x1b", strings.Repeat("x", MaxItemBytes+1)} {
		var out bytes.Buffer
		CaptureCommand(strings.NewReader(text), &out, "data")
		var c Capture
		json.Unmarshal(out.Bytes(), &c)
		if c.State != "skipped" || c.Text != "" {
			t.Fatal("invalid data retained")
		}
	}
	original := "line one\nline two\tλ\r\n"
	var out bytes.Buffer
	CaptureCommand(strings.NewReader(original), &out, "data")
	var c Capture
	json.Unmarshal(out.Bytes(), &c)
	if c.Text != original {
		t.Fatal("text/newlines changed")
	}
}
func TestHistoryDedupBoundsAndDeletion(t *testing.T) {
	m := New(nil)
	m.collecting = true
	m.ingest(Capture{State: "data", Text: "first"})
	id := m.Snapshot().Entries[0].ID
	m.ingest(Capture{State: "data", Text: "second"})
	m.ingest(Capture{State: "data", Text: "first"})
	snap := m.Snapshot()
	if len(snap.Entries) != 2 || snap.Entries[0].ID != id {
		t.Fatal("did not deduplicate/reorder")
	}
	snap.Entries[0].Text = "client mutation"
	if m.Snapshot().Entries[0].Text != "first" {
		t.Fatal("snapshot aliases private state")
	}
	if _, err := m.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Delete(id); err == nil {
		t.Fatal("accepted stale ID")
	}
	m.Clear()
	for i := 0; i < MaxEntries+10; i++ {
		m.ingest(Capture{State: "data", Text: fmt.Sprint(i)})
	}
	if len(m.Snapshot().Entries) != MaxEntries {
		t.Fatal("entry limit not applied")
	}
	m.Clear()
	for i := 0; i < 20; i++ {
		m.ingest(Capture{State: "data", Text: fmt.Sprint(i) + strings.Repeat("<", MaxItemBytes-2)})
	}
	if m.totalBytes > MaxTotalBytes {
		t.Fatal("byte budget exceeded")
	}
	encoded, err := json.Marshal(m.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) >= 1024*1024 {
		t.Fatal("bounded snapshot exceeds IPC frame budget")
	}
}

type fakeDriver struct {
	mu             sync.Mutex
	watches, stops int
	copied         string
	changes        chan Capture
}

func (f *fakeDriver) Watch(ctx context.Context, emit func(Capture)) error {
	f.mu.Lock()
	f.watches++
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.stops++; f.mu.Unlock() }()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case c := <-f.changes:
			emit(c)
		}
	}
}
func (f *fakeDriver) Copy(_ context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.copied = text
	return nil
}
func eventually(t *testing.T, check func() bool) {
	t.Helper()
	end := time.Now().Add(time.Second)
	for time.Now().Before(end) {
		if check() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition timed out")
}
func TestCollectionIsExplicitPausesAndShutdownClearsMemory(t *testing.T) {
	f := &fakeDriver{changes: make(chan Capture, 10)}
	m := New(f)
	m.Start(context.Background())
	defer m.Stop(context.Background())
	if m.Snapshot().Collecting || f.watches != 0 {
		t.Fatal("started watching without opt-in")
	}
	if _, err := m.SetCollecting(true); err != nil {
		t.Fatal(err)
	}
	f.changes <- Capture{State: "sensitive", Text: "fixture sensitive marker"}
	f.changes <- Capture{State: "data", Text: "exact\ntext"}
	eventually(t, func() bool { return len(m.Snapshot().Entries) == 1 })
	id := m.Snapshot().Entries[0].ID
	if _, err := m.Copy(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	copied := f.copied
	f.mu.Unlock()
	if copied != "exact\ntext" {
		t.Fatal("copy altered text")
	}
	m.SetCollecting(false)
	if m.Snapshot().Collecting {
		t.Fatal("pause did not finish")
	}
	f.mu.Lock()
	w, s := f.watches, f.stops
	f.mu.Unlock()
	if w != s {
		t.Fatal("watcher outlived Pause")
	}
	m.Clear()
	if len(m.Snapshot().Entries) != 0 {
		t.Fatal("clear retained entries")
	}
	f.mu.Lock()
	unchanged := f.copied == copied
	f.mu.Unlock()
	if !unchanged {
		t.Fatal("Clear altered system clipboard")
	}
	m.SetCollecting(true)
	f.changes <- Capture{State: "data", Text: "later"}
	eventually(t, func() bool { return len(m.Snapshot().Entries) == 1 })
	m.Stop(context.Background())
	if len(m.Snapshot().Entries) != 0 {
		t.Fatal("shutdown retained history")
	}
	if _, err := m.Copy(context.Background(), id); err == nil {
		t.Fatal("copied missing item")
	}
}

var _ io.Reader = forbiddenReader{}
