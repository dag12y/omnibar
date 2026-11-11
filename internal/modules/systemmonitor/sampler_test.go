package systemmonitor

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"omnibar/internal/events"
)

// testSubscriber collects delivered events for inspection.
type testSubscriber struct {
	mu     sync.Mutex
	events []events.Event
}

func (s *testSubscriber) Deliver(ev events.Event) bool {
	s.mu.Lock()
	s.events = append(s.events, ev)
	s.mu.Unlock()
	return true
}

func (s *testSubscriber) names() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]bool, len(s.events))
	for _, ev := range s.events {
		out[ev.Name] = true
	}
	return out
}

func TestSamplerStartStopNoLeak(t *testing.T) {
	hub := newTestHub()
	s := newSampler("test", 50*time.Millisecond, hub, nil)
	s.statPath = filepath.Join("fixtures", "stat_normal")
	s.meminfoPath = filepath.Join("fixtures", "meminfo_normal")

	s.start()
	s.stop()
	// stop returns only after the goroutine has exited; no leak.
}

func TestSamplerPublishesOnlyWhenSubscribed(t *testing.T) {
	hub := newTestHub()
	subscriber := &testSubscriber{}
	hub.Subscribe(subscriber, "test", []string{"cpu", "memory", "disk", "network"})

	s := newSampler("test", 50*time.Millisecond, hub, nil)
	s.statPath = filepath.Join("fixtures", "stat_normal")
	s.meminfoPath = filepath.Join("fixtures", "meminfo_normal")
	s.diskPath = filepath.Join("fixtures", "diskstats_normal")
	s.netdevPath = filepath.Join("fixtures", "netdev_normal")

	s.start()
	defer s.stop()

	// Wait for at least one cpu, one memory, one disk, and one network event.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if names := subscriber.names(); names["cpu"] && names["memory"] && names["disk"] && names["network"] {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	names := subscriber.names()
	if !names["cpu"] {
		t.Error("expected cpu event to be published")
	}
	if !names["memory"] {
		t.Error("expected memory event to be published")
	}
	if !names["disk"] {
		t.Error("expected disk event to be published")
	}
	if !names["network"] {
		t.Error("expected network event to be published")
	}
}

func TestSamplerNoEventsWithoutSubscribers(t *testing.T) {
	hub := newTestHub()
	subscriber := &testSubscriber{}
	// Not subscribed: no events should flow.

	s := newSampler("test", 50*time.Millisecond, hub, nil)
	s.statPath = filepath.Join("fixtures", "stat_normal")
	s.meminfoPath = filepath.Join("fixtures", "meminfo_normal")

	s.start()
	defer s.stop()

	time.Sleep(150 * time.Millisecond)

	if n := len(subscriber.events); n != 0 {
		t.Errorf("expected no events without subscription, got %d", n)
	}
}

func TestSampleOnceOnDemand(t *testing.T) {
	s := newSampler("test", time.Hour, newTestHub(), nil)
	s.statPath = filepath.Join("fixtures", "stat_normal")
	s.meminfoPath = filepath.Join("fixtures", "meminfo_normal")

	snap, err := s.sampleOnce()
	if err != nil {
		t.Fatalf("sampleOnce() returned error: %v", err)
	}
	if snap == nil {
		t.Fatal("sampleOnce() returned nil snapshot")
	}
	if snap.CPU.Percent == 0 {
		t.Log("CPU percent is 0 (no baseline for first sample)")
	}
	if snap.Memory.Total == 0 {
		t.Error("Memory.Total should be non-zero")
	}
	if snap.SampledAt == "" {
		t.Error("SampledAt must not be empty")
	}

	// Second sample should have a baseline and produce a non-zero CPU delta.
	// Switch to stat_second so the delta differs from the baseline.
	s.statPath = filepath.Join("fixtures", "stat_second")
	snap2, err := s.sampleOnce()
	if err != nil {
		t.Fatalf("second sampleOnce() returned error: %v", err)
	}
	if snap2.CPU.Percent == 0 {
		t.Error("expected non-zero CPU percent with a baseline delta")
	}
}
