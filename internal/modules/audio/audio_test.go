package audio

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"omnibar/internal/events"
)

type fakeDriver struct {
	mu                             sync.Mutex
	fail                           bool
	reads, changes, watches, stops int
	snap                           Snapshot
	events                         chan struct{}
	disconnect                     chan struct{}
}

func testDriver() *fakeDriver {
	return &fakeDriver{snap: Snapshot{Available: true, Outputs: []Output{{ID: "1", Name: "speakers"}, {ID: "2", Name: "headphones"}}, DefaultOutput: "speakers", Streams: []Stream{{ID: "5", Key: "5:serial:pid:App", Name: "App", Volume: 50}}}, events: make(chan struct{}, 1), disconnect: make(chan struct{}, 1)}
}
func (f *fakeDriver) Read(context.Context) (Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if f.fail {
		return Snapshot{}, errors.New("service down")
	}
	snap := f.snap
	snap.Streams = append([]Stream{}, f.snap.Streams...)
	snap.Inputs = append([]Input{}, f.snap.Inputs...)
	return snap, nil
}
func (f *fakeDriver) SetVolume(_ context.Context, _ string, volume int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes++
	f.snap.Streams[0].Volume = volume
	return nil
}
func (f *fakeDriver) SetMute(_ context.Context, _ string, muted bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes++
	f.snap.Streams[0].Muted = muted
	return nil
}
func (f *fakeDriver) SetDefaultOutput(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes++
	f.snap.DefaultOutput = name
	return nil
}
func (f *fakeDriver) MoveStream(_ context.Context, id, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes++
	for _, output := range f.snap.Outputs {
		if output.Name == name {
			for i := range f.snap.Streams {
				if f.snap.Streams[i].ID == id {
					f.snap.Streams[i].OutputID = output.ID
					return nil
				}
			}
		}
	}
	return errors.New("target disappeared")
}
func (f *fakeDriver) SetInputVolume(_ context.Context, name string, volume int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes++
	for i := range f.snap.Inputs {
		if f.snap.Inputs[i].Name == name {
			f.snap.Inputs[i].Volume = volume
			return nil
		}
	}
	return errors.New("input disappeared")
}
func (f *fakeDriver) SetInputMute(_ context.Context, name string, muted bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes++
	for i := range f.snap.Inputs {
		if f.snap.Inputs[i].Name == name {
			f.snap.Inputs[i].Muted = muted
			return nil
		}
	}
	return errors.New("input disappeared")
}
func (f *fakeDriver) SetDefaultInput(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes++
	f.snap.DefaultInput = name
	return nil
}
func (f *fakeDriver) Watch(ctx context.Context, changed chan<- struct{}) error {
	f.mu.Lock()
	f.watches++
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.stops++; f.mu.Unlock() }()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.disconnect:
			return errors.New("disconnected")
		case <-f.events:
			select {
			case changed <- struct{}{}:
			default:
			}
		}
	}
}
func (f *fakeDriver) counts() (int, int, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads, f.changes, f.watches, f.stops
}

type collector struct {
	mu        sync.Mutex
	snapshots []Snapshot
}

func (c *collector) Deliver(event events.Event) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	var snap Snapshot
	if err := json.Unmarshal(event.Data, &snap); err != nil {
		return false
	}
	c.snapshots = append(c.snapshots, snap)
	return true
}
func (c *collector) last() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.snapshots) == 0 {
		return Snapshot{}
	}
	return c.snapshots[len(c.snapshots)-1]
}
func await(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}
func TestChangesValidateLiveTargets(t *testing.T) {
	d := testDriver()
	m := New(nil, d)
	ctx := context.Background()
	for _, tc := range []struct {
		action, key string
		volume      int
	}{{"volume", "gone", 40}, {"volume", "5:serial:pid:App", 101}, {"output", "disconnected", 0}} {
		if _, err := m.Change(ctx, tc.action, tc.key, tc.volume, false); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
	_, changes, _, _ := d.counts()
	if changes != 0 {
		t.Fatal("invalid target mutated audio")
	}
	snap, err := m.Change(ctx, "volume", "5:serial:pid:App", 33, false)
	if err != nil || snap.Streams[0].Volume != 33 {
		t.Fatalf("volume: %+v %v", snap, err)
	}
	snap, err = m.Change(ctx, "mute", "5:serial:pid:App", 0, true)
	if err != nil || !snap.Streams[0].Muted {
		t.Fatalf("mute: %+v %v", snap, err)
	}
	snap, err = m.Change(ctx, "output", "headphones", 0, false)
	if err != nil || snap.DefaultOutput != "headphones" {
		t.Fatalf("output: %+v %v", snap, err)
	}
}
func TestDemandAndRecovery(t *testing.T) {
	d := testDriver()
	hub := events.New(nil)
	m := New(hub, d)
	sub := &collector{}
	m.Start(context.Background())
	defer m.Stop(context.Background())
	time.Sleep(30 * time.Millisecond)
	reads, _, watches, _ := d.counts()
	if reads != 0 || watches != 0 {
		t.Fatal("audio connected without demand")
	}
	hub.Subscribe(sub, Name, nil)
	await(t, func() bool { return sub.last().Available })
	d.mu.Lock()
	d.fail = true
	d.mu.Unlock()
	d.disconnect <- struct{}{}
	await(t, func() bool { return sub.last().Error != "" })
	d.mu.Lock()
	d.fail = false
	d.snap.Outputs = []Output{{ID: "3", Name: "new-bluetooth"}}
	d.mu.Unlock()
	await(t, func() bool {
		return sub.last().Available && len(sub.last().Outputs) == 1 && sub.last().Outputs[0].Name == "new-bluetooth"
	})
	hub.Remove(sub)
	await(t, func() bool { _, _, w, s := d.counts(); return w == s })
	reads, _, _, _ = d.counts()
	time.Sleep(50 * time.Millisecond)
	after, _, _, _ := d.counts()
	if after != reads {
		t.Fatal("audio read after unsubscribe")
	}
}

func TestMoveStreamPreservesOtherAudioState(t *testing.T) {
	d := testDriver()
	d.snap.Streams[0].OutputID = "1"
	d.snap.Streams[0].Muted = true
	d.snap.Streams = append(d.snap.Streams, Stream{ID: "6", Key: "other", Name: "Other", OutputID: "1", Volume: 75})
	m := New(nil, d)
	snap, err := m.MoveStream(context.Background(), "5:serial:pid:App", "headphones")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Streams[0].OutputID != "2" || snap.Streams[0].Volume != 50 || !snap.Streams[0].Muted {
		t.Fatalf("bad routed stream: %+v", snap.Streams[0])
	}
	if snap.DefaultOutput != "speakers" || snap.Streams[1].OutputID != "1" || snap.Streams[1].Volume != 75 {
		t.Fatalf("unrelated audio changed: %+v", snap)
	}
	for _, tc := range []struct{ key, output string }{{"gone", "headphones"}, {"5:serial:pid:App", "unplugged"}} {
		if _, err := m.MoveStream(context.Background(), tc.key, tc.output); err == nil {
			t.Fatalf("accepted stale route: %+v", tc)
		}
	}
	_, changes, _, _ := d.counts()
	if changes != 1 {
		t.Fatalf("stale requests mutated audio: %d calls", changes)
	}
	d.fail = true
	if _, err := m.MoveStream(context.Background(), "5:serial:pid:App", "speakers"); err == nil {
		t.Fatal("routed while service down")
	}
}

func TestMicrophoneControlsValidateTargetsAndPreservePlayback(t *testing.T) {
	d := testDriver()
	d.snap.DefaultInput = "mic_a"
	d.snap.Inputs = []Input{{ID: "11", Name: "mic_a", Volume: 50}, {ID: "12", Name: "mic_b", Volume: 80, Muted: true}, {ID: "1", Name: "speakers.monitor", Monitor: true}}
	d.snap.Recordings = []Recording{{ID: "21", Name: "Call", InputID: "11"}}
	m := New(nil, d)
	ctx := context.Background()
	snap, err := m.ChangeInput(ctx, "input-volume", "mic_a", 35, false)
	if err != nil || snap.Inputs[0].Volume != 35 {
		t.Fatalf("gain: %+v %v", snap, err)
	}
	snap, err = m.ChangeInput(ctx, "input-mute", "mic_a", 0, true)
	if err != nil || !snap.Inputs[0].Muted {
		t.Fatalf("mute: %+v %v", snap, err)
	}
	snap, err = m.ChangeInput(ctx, "input-default", "mic_b", 0, false)
	if err != nil || snap.DefaultInput != "mic_b" {
		t.Fatalf("default: %+v %v", snap, err)
	}
	if snap.Inputs[1].Volume != 80 || !snap.Inputs[1].Muted || snap.DefaultOutput != "speakers" || snap.Streams[0].Volume != 50 || snap.Recordings[0].InputID != "11" {
		t.Fatalf("unrelated state changed: %+v", snap)
	}
	for _, tc := range []struct {
		action, name string
		level        int
	}{{"input-volume", "mic_a", 101}, {"input-volume", "mic_a", -1}, {"input-mute", "gone", 0}, {"input-default", "speakers.monitor", 0}, {"input-volume", "speakers.monitor", 40}, {"input-mute", "speakers.monitor", 0}} {
		if _, err := m.ChangeInput(ctx, tc.action, tc.name, tc.level, false); err == nil {
			t.Fatalf("accepted invalid input action %+v", tc)
		}
	}
	_, changes, _, _ := d.counts()
	if changes != 3 {
		t.Fatalf("invalid request touched audio: %d", changes)
	}
}
