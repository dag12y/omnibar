package keepawake

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

type lease struct{ closed atomic.Int32 }

func (l *lease) Close() error { l.closed.Add(1); return nil }

func TestSessionOwnsLeaseUntilStoppedOrShutdown(t *testing.T) {
	resource := &lease{}
	calls := 0
	m := New(nil, func(context.Context) (io.Closer, error) { calls++; return resource, nil })
	if _, err := m.Begin(context.Background(), 60); err == nil {
		t.Fatal("started a stopped module")
	}
	m.Start(context.Background())
	for _, seconds := range []int{60, 120} {
		status, err := m.Begin(context.Background(), seconds)
		if err != nil || !status.Active || status.ExpiresAt == "" {
			t.Fatalf("begin: %+v %v", status, err)
		}
	}
	if calls != 1 {
		t.Fatalf("extension acquired %d leases", calls)
	}
	status, err := m.End()
	if err != nil || status.Active || resource.closed.Load() != 1 {
		t.Fatalf("stop did not release: %+v %v", status, err)
	}
	m.End()
	if resource.closed.Load() != 1 {
		t.Fatal("double close")
	}
	m.Begin(context.Background(), 60)
	m.Stop(context.Background())
	m.Dispose(context.Background())
	if m.Status().Active || resource.closed.Load() != 2 {
		t.Fatal("shutdown did not release the new session exactly once")
	}
}

func TestFailureAndInvalidDurationDoNotActivate(t *testing.T) {
	calls := 0
	m := New(nil, func(context.Context) (io.Closer, error) { calls++; return nil, errors.New("denied by logind") })
	m.Start(context.Background())
	for _, seconds := range []int{0, -1, MaxSeconds + 1} {
		if _, err := m.Begin(context.Background(), seconds); err == nil {
			t.Fatalf("accepted %d", seconds)
		}
	}
	if calls != 0 {
		t.Fatal("invalid request acquired an inhibitor")
	}
	if _, err := m.Begin(context.Background(), 60); err == nil {
		t.Fatal("lost acquisition error")
	}
	if m.Status().Active {
		t.Fatal("failed acquisition reported active")
	}
}

func TestExpiryAndExtension(t *testing.T) {
	resource := &lease{}
	m := New(nil, func(context.Context) (io.Closer, error) { return resource, nil })
	m.Start(context.Background())
	defer m.Stop(context.Background())
	m.Begin(context.Background(), 1)
	time.Sleep(600 * time.Millisecond)
	m.Begin(context.Background(), 1)
	time.Sleep(600 * time.Millisecond)
	if !m.Status().Active {
		t.Fatal("old timer expired the extended session")
	}
	deadline := time.Now().Add(2 * time.Second)
	for m.Status().Active && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if m.Status().Active || m.Status().Reason != "expired" || resource.closed.Load() != 1 {
		t.Fatal("expiry did not release the inhibitor")
	}
}
