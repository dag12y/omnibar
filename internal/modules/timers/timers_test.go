package timers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const bootOne = "11111111-1111-1111-1111-111111111111"
const bootTwo = "22222222-2222-2222-2222-222222222222"

type fakeClock struct {
	mu    sync.Mutex
	now   Sample
	fail  bool
	reads int
}

func (c *fakeClock) Sample() (Sample, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads++
	if c.fail {
		return Sample{}, errors.New("clock failed")
	}
	return c.now, nil
}
func (c *fakeClock) advance(wall, boot time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now.WallMS += wall.Milliseconds()
	c.now.BootNS += int64(boot)
}
func (c *fakeClock) reboot(wall time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now.WallMS += wall.Milliseconds()
	c.now.BootNS = int64(time.Minute)
	c.now.BootID = bootTwo
}

type memoryStore struct {
	mu            sync.Mutex
	data          Data
	fail          bool
	writes, loads int
}

func (s *memoryStore) Load() (Data, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loads++
	d := s.data
	d.Timers = append([]Record{}, d.Timers...)
	return d, nil
}
func (s *memoryStore) Save(d Data) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes++
	if s.fail {
		return errors.New("disk full")
	}
	d.Timers = append([]Record{}, d.Timers...)
	s.data = d
	return nil
}
func (s *memoryStore) setFail(v bool)  { s.mu.Lock(); s.fail = v; s.mu.Unlock() }
func (s *memoryStore) writeCount() int { s.mu.Lock(); defer s.mu.Unlock(); return s.writes }

type fakeNotifier struct {
	calls chan string
	fail  bool
	block bool
	store *memoryStore
}

func (n *fakeNotifier) Notify(ctx context.Context, name string) error {
	if n.store != nil {
		d, _ := n.store.Load()
		found := false
		for _, r := range d.Timers {
			if r.Name == name && r.State == "completed" {
				found = true
			}
		}
		if !found {
			panic("alert sent before durable completion")
		}
	}
	n.calls <- name
	if n.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if n.fail {
		return errors.New("service missing")
	}
	return nil
}
func fixture(t *testing.T) (*Module, *memoryStore, *fakeClock, *fakeNotifier) {
	t.Helper()
	s := &memoryStore{data: Data{Version: 1, Revision: 1, Timers: []Record{}}}
	c := &fakeClock{now: Sample{WallMS: 1800000000000, BootNS: int64(100 * time.Hour), BootID: bootOne}}
	n := &fakeNotifier{calls: make(chan string, 100), store: s}
	m := New(s, c, n, nil)
	m.Initialize(context.Background())
	m.Start(context.Background())
	t.Cleanup(func() { m.Stop(context.Background()) })
	return m, s, c, n
}
func row(t *testing.T, m *Module, id string) Row {
	t.Helper()
	for _, r := range m.Snapshot().Timers {
		if r.ID == id {
			return r
		}
	}
	t.Fatal("missing timer")
	return Row{}
}
func alert(t *testing.T, n *fakeNotifier) {
	t.Helper()
	select {
	case <-n.calls:
	case <-time.After(time.Second):
		t.Fatal("missing completion notification")
	}
}
func TestClockJumpsSuspendPauseResumeAndNoTickWrites(t *testing.T) {
	m, s, c, n := fixture(t)
	snap, e := m.Begin("", "Tea", 10)
	if e != nil {
		t.Fatal(e)
	}
	id := snap.Timers[0].ID
	writes := s.writeCount()
	m.tick()
	if s.writeCount() != writes {
		t.Fatal("ordinary tick wrote state")
	}
	c.advance(time.Hour, 0)
	if row(t, m, id).RemainingMS != 10000 {
		t.Fatal("forward wall jump changed countdown")
	}
	c.advance(-2*time.Hour, 2*time.Second)
	r := row(t, m, id)
	if r.RemainingMS != 8000 {
		t.Fatal("boot clock did not count")
	}
	snap, e = m.Change("pause", id, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	c.advance(time.Hour, time.Hour)
	if row(t, m, id).RemainingMS != 8000 {
		t.Fatal("paused timer advanced")
	}
	r = snap.Timers[0]
	if _, e = m.Change("resume", id, r.Revision); e != nil {
		t.Fatal(e)
	}
	c.advance(0, 9*time.Second)
	m.tick()
	alert(t, n)
	if row(t, m, id).State != "completed" {
		t.Fatal("suspend time did not complete")
	}
	m.tick()
	if len(n.calls) != 0 {
		t.Fatal("duplicate alert")
	}
}
func TestSameBootRestartAndPausedPersistence(t *testing.T) {
	m, s, c, n := fixture(t)
	a, _ := m.Begin("", "Running", 60)
	id := a.Timers[0].ID
	b, _ := m.Begin("", "Paused", 40)
	paused := b.Timers[0]
	m.Change("pause", paused.ID, paused.Revision)
	m.Stop(context.Background())
	c.advance(3*time.Hour, 20*time.Second)
	restored := New(s, c, n, nil)
	restored.Initialize(context.Background())
	restored.Start(context.Background())
	defer restored.Stop(context.Background())
	if row(t, restored, id).RemainingMS != 40000 {
		t.Fatal("same-boot restart used wall time")
	}
	if r := row(t, restored, paused.ID); r.State != "paused" || r.RemainingMS != 40000 {
		t.Fatal("paused recovery failed")
	}
	c.advance(0, 41*time.Second)
	restored.tick()
	alert(t, n)
	restored.Stop(context.Background())
	again := New(s, c, n, nil)
	again.Initialize(context.Background())
	again.Start(context.Background())
	defer again.Stop(context.Background())
	again.tick()
	if len(n.calls) != 0 {
		t.Fatal("completion replayed on restart")
	}
}
func TestNewBootUTCRecoveryExpiredAndBackwardClamp(t *testing.T) {
	for _, mode := range []string{"future", "expired", "backward"} {
		t.Run(mode, func(t *testing.T) {
			m, s, c, n := fixture(t)
			snap, _ := m.Begin("", "Restart", 30)
			id := snap.Timers[0].ID
			m.Stop(context.Background())
			delta := 10 * time.Second
			if mode == "expired" {
				delta = time.Hour
			}
			if mode == "backward" {
				delta = -time.Hour
			}
			c.reboot(delta)
			restored := New(s, c, n, nil)
			restored.Initialize(context.Background())
			restored.Start(context.Background())
			defer restored.Stop(context.Background())
			r := row(t, restored, id)
			if mode == "future" && r.RemainingMS != 20000 {
				t.Fatal("UTC recovery failed")
			}
			if mode == "backward" && r.RemainingMS != 30000 {
				t.Fatal("clock rollback extended original duration")
			}
			if mode == "expired" {
				restored.tick()
				alert(t, n)
				if row(t, restored, id).State != "completed" {
					t.Fatal("elapsed offline timer not completed")
				}
			}
		})
	}
}
func TestPersistenceFailureNeverAlertsOrAcknowledgesChange(t *testing.T) {
	m, s, c, n := fixture(t)
	snap, _ := m.Begin("", "Disk failure", 2)
	r := snap.Timers[0]
	s.setFail(true)
	if _, e := m.Change("pause", r.ID, r.Revision); e == nil {
		t.Fatal("failed pause acknowledged")
	}
	if row(t, m, r.ID).State != "running" {
		t.Fatal("failed write changed memory")
	}
	c.advance(3*time.Second, 3*time.Second)
	m.tick()
	if len(n.calls) != 0 || row(t, m, r.ID).State != "running" {
		t.Fatal("alert preceded durable completion")
	}
	if m.Snapshot().StorageError == "" {
		t.Fatal("failure hidden")
	}
	s.setFail(false)
	m.tick()
	alert(t, n)
	if m.Snapshot().StorageError != "" {
		t.Fatal("recovery retained error")
	}
}
func TestUnavailableNotifierAndSlowNotifierDoNotLoseCompletion(t *testing.T) {
	for _, block := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "slow"}[block], func(t *testing.T) {
			m, _, c, n := fixture(t)
			n.fail = true
			n.block = block
			snap, _ := m.Begin("", "Alert", 1)
			id := snap.Timers[0].ID
			c.advance(2*time.Second, 2*time.Second)
			m.tick()
			alert(t, n)
			if row(t, m, id).State != "completed" {
				t.Fatal("notification blocked snapshot")
			}
			if block {
				done := make(chan struct{})
				go func() { m.Stop(context.Background()); close(done) }()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("Stop did not cancel notification")
				}
			} else {
				until := time.Now().Add(time.Second)
				for row(t, m, id).Notification != "unavailable" && time.Now().Before(until) {
					time.Sleep(time.Millisecond)
				}
				if row(t, m, id).Notification != "unavailable" {
					t.Fatal("notification failure not shown")
				}
			}
		})
	}
}
func TestLimitsRevisionsIdempotencyAndDismiss(t *testing.T) {
	m, _, c, n := fixture(t)
	for _, name := range []string{"", " bad", "bad\nname", strings.Repeat("x", 65)} {
		if _, e := m.Begin("", name, 1); e == nil {
			t.Fatal("bad name accepted")
		}
	}
	if _, e := m.Begin("", "too long", MaxSeconds+1); e == nil {
		t.Fatal("duration cap ignored")
	}
	snap, e := m.Begin(strings.Repeat("a", 32), "Stable", 5)
	if e != nil {
		t.Fatal(e)
	}
	r := snap.Timers[0]
	again, e := m.Begin(r.ID, r.Name, 5)
	if e != nil || again.Revision != snap.Revision || len(again.Timers) != 1 {
		t.Fatal("start retry duplicated timer")
	}
	if _, e = m.Begin(r.ID, "other", 5); e == nil {
		t.Fatal("id reused")
	}
	paused, e := m.Change("pause", r.ID, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.Change("cancel", r.ID, r.Revision); !errors.Is(e, ErrConflict) {
		t.Fatal("stale action accepted")
	}
	p := paused.Timers[0]
	m.Change("resume", p.ID, p.Revision)
	for range MaxUnfinished - 1 {
		if _, e = m.Begin("", "more", 5); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = m.Begin("", "over", 5); e == nil {
		t.Fatal("unfinished cap ignored")
	}
	c.advance(6*time.Second, 6*time.Second)
	m.tick()
	for range MaxUnfinished {
		alert(t, n)
	}
	r = row(t, m, r.ID)
	if _, e = m.Change("cancel", r.ID, r.Revision); e == nil {
		t.Fatal("cancel accepted completed state")
	}
	if _, e = m.Change("dismiss", r.ID, r.Revision); e != nil {
		t.Fatal(e)
	}
	for range 25 {
		snap, e = m.Begin("", "history", 1)
		if e != nil {
			t.Fatal(e)
		}
		c.advance(2*time.Second, 2*time.Second)
		m.tick()
		alert(t, n)
	}
	m.mu.Lock()
	alertCount := len(m.alerts)
	m.mu.Unlock()
	if alertCount > MaxCompleted {
		t.Fatal("notification metadata leaked beyond history limit")
	}
	if m.Snapshot().Completed != MaxCompleted {
		t.Fatal("completed history not bounded")
	}
}
func TestPrivateFileCorruptionAndDeniedStorage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private/timers.json")
	store := FileStore{Path: path}
	m := New(store, nil, nil, nil)
	m.Initialize(context.Background())
	m.Start(context.Background())
	defer m.Stop(context.Background())
	if _, e := m.Begin("", "Private", 60); e != nil {
		t.Fatal(e)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("file not private")
	}
	dir, _ := os.Stat(filepath.Dir(path))
	if dir.Mode().Perm() != 0700 {
		t.Fatal("directory not private")
	}
	original, _ := os.ReadFile(path)
	os.Chmod(path, 0644)
	if _, e := m.Begin("", "bad write", 60); e == nil {
		t.Fatal("unsafe file replaced")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("failed write changed file")
	}
	m.Stop(context.Background())
	os.Chmod(path, 0600)
	for _, bad := range []string{"corrupt", strings.Repeat(" ", MaxFileBytes+1), `{"version":1,"revision":1,"bootId":"","timers":[],"extra":1}`} {
		os.WriteFile(path, []byte(bad), 0600)
		broken := New(store, nil, nil, nil)
		broken.Initialize(context.Background())
		broken.Start(context.Background())
		if broken.Snapshot().StorageError == "" {
			t.Fatal("corruption hidden")
		}
		if _, e := broken.Begin("", "overwrite", 1); e == nil {
			t.Fatal("corrupt data overwritten")
		}
		broken.Stop(context.Background())
		got, _ := os.ReadFile(path)
		if string(got) != bad {
			t.Fatal("corrupt original lost")
		}
	}
	os.Remove(path)
	os.Symlink(filepath.Join(t.TempDir(), "absent"), path)
	if _, e := store.Load(); e == nil {
		t.Fatal("symlink followed")
	}
	c := &fakeClock{}
	denied := New(nil, c, nil, nil)
	denied.Initialize(context.Background())
	denied.Start(context.Background())
	denied.Snapshot()
	denied.Stop(context.Background())
	if c.reads != 0 {
		t.Fatal("denied module sampled clocks")
	}
	t.Setenv("XDG_DATA_HOME", "relative")
	if _, e := DefaultStore().Load(); e == nil {
		t.Fatal("relative XDG accepted")
	}
}
