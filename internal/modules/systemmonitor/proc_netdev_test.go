package systemmonitor

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseNetdev(t *testing.T) {
	now := time.Now()
	f, err := os.Open("fixtures/netdev_normal")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	sample, err := ParseNetdev(f, now)
	if err != nil {
		t.Fatalf("ParseNetdev returned error: %v", err)
	}

	// eth0 (2,000,000) + wlan0 (3,000,000) = 5,000,000 (lo excluded)
	wantRx := uint64(5000000)
	if sample.RxBytes != wantRx {
		t.Errorf("RxBytes = %d, want %d", sample.RxBytes, wantRx)
	}

	// eth0 (1,000,000) + wlan0 (2,000,000) = 3,000,000
	wantTx := uint64(3000000)
	if sample.TxBytes != wantTx {
		t.Errorf("TxBytes = %d, want %d", sample.TxBytes, wantTx)
	}
}

func TestComputeNetDelta(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(2 * time.Second)

	s0 := &NetSample{
		Timestamp: t0,
		RxBytes:   5000000,
		TxBytes:   3000000,
	}
	s1 := &NetSample{
		Timestamp: t1,
		RxBytes:   9000000, // +4,000,000 over 2s => 2,000,000 B/s
		TxBytes:   6000000, // +3,000,000 over 2s => 1,500,000 B/s
	}

	rxRate, txRate := computeNetDelta(s1, s0)
	if rxRate != 2000000 {
		t.Errorf("rxRate = %d, want 2000000", rxRate)
	}
	if txRate != 1500000 {
		t.Errorf("txRate = %d, want 1500000", txRate)
	}

	// Nil prev => 0 rates
	rx0, tx0 := computeNetDelta(s0, nil)
	if rx0 != 0 || tx0 != 0 {
		t.Errorf("computeNetDelta(s0, nil) = (%d, %d), want (0, 0)", rx0, tx0)
	}

	// Counter wrap or underflow => 0 rates
	sWrap := &NetSample{
		Timestamp: t1,
		RxBytes:   100,
		TxBytes:   100,
	}
	rxWrap, txWrap := computeNetDelta(sWrap, s0)
	if rxWrap != 0 || txWrap != 0 {
		t.Errorf("computeNetDelta wrap = (%d, %d), want (0, 0)", rxWrap, txWrap)
	}
}

func TestParseNetdevMissingFile(t *testing.T) {
	_, err := ParseNetdevFile("/nonexistent/netdev", time.Now())
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestParseNetdevMalformed(t *testing.T) {
	input := "short line\nanother line: 1 2 3\n"
	s, err := ParseNetdev(strings.NewReader(input), time.Now())
	if err != nil {
		t.Fatalf("unexpected error on malformed input: %v", err)
	}
	if s.RxBytes != 0 || s.TxBytes != 0 {
		t.Errorf("expected 0 bytes for malformed input, got rx=%d tx=%d", s.RxBytes, s.TxBytes)
	}
}
