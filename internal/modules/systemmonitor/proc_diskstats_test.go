package systemmonitor

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseDiskstats(t *testing.T) {
	now := time.Now()
	f, err := os.Open("fixtures/diskstats_normal")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	sample, err := ParseDiskstats(f, now)
	if err != nil {
		t.Fatalf("ParseDiskstats returned error: %v", err)
	}

	// nvme0n1: 2000 * 512 = 1,024,000; sda: 6000 * 512 = 3,072,000 => 4,096,000
	wantRead := uint64(4096000)
	if sample.ReadBytes != wantRead {
		t.Errorf("ReadBytes = %d, want %d", sample.ReadBytes, wantRead)
	}

	// nvme0n1: 4000 * 512 = 2,048,000; sda: 8000 * 512 = 4,096,000 => 6,144,000
	wantWrite := uint64(6144000)
	if sample.WriteBytes != wantWrite {
		t.Errorf("WriteBytes = %d, want %d", sample.WriteBytes, wantWrite)
	}
}

func TestComputeDiskDelta(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(2 * time.Second)

	s0 := &DiskSample{
		Timestamp:  t0,
		ReadBytes:  4096000,
		WriteBytes: 6144000,
	}
	s1 := &DiskSample{
		Timestamp:  t1,
		ReadBytes:  6144000,  // +2,048,000 bytes over 2s => 1,024,000 B/s
		WriteBytes: 10240000, // +4,096,000 bytes over 2s => 2,048,000 B/s
	}

	rRate, wRate := computeDiskDelta(s1, s0)
	if rRate != 1024000 {
		t.Errorf("rRate = %d, want 1024000", rRate)
	}
	if wRate != 2048000 {
		t.Errorf("wRate = %d, want 2048000", wRate)
	}

	// First reading (no previous sample): zero rates
	rRate0, wRate0 := computeDiskDelta(s0, nil)
	if rRate0 != 0 || wRate0 != 0 {
		t.Errorf("computeDiskDelta(s0, nil) = (%d, %d), want (0, 0)", rRate0, wRate0)
	}

	// Counter wrap or underflow: clamps to 0
	sWrapped := &DiskSample{
		Timestamp:  t1,
		ReadBytes:  1000,
		WriteBytes: 1000,
	}
	rRateWrap, wRateWrap := computeDiskDelta(sWrapped, s0)
	if rRateWrap != 0 || wRateWrap != 0 {
		t.Errorf("computeDiskDelta wrap = (%d, %d), want (0, 0)", rRateWrap, wRateWrap)
	}
}

func TestDefaultFsStat(t *testing.T) {
	total, avail, used, err := DefaultFsStat("/")
	if err != nil {
		t.Fatalf("DefaultFsStat(/) returned error: %v", err)
	}
	if total == 0 {
		t.Errorf("expected total > 0, got %d", total)
	}
	if avail == 0 && used == 0 {
		t.Errorf("expected avail or used > 0")
	}
}

func TestParseDiskstatsMissingFile(t *testing.T) {
	_, err := ParseDiskstatsFile("/nonexistent/diskstats", time.Now())
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestParseDiskstatsMalformed(t *testing.T) {
	input := "short line\nanother bad line with fewer fields\n"
	s, err := ParseDiskstats(strings.NewReader(input), time.Now())
	if err != nil {
		t.Fatalf("unexpected error on malformed input: %v", err)
	}
	if s.ReadBytes != 0 || s.WriteBytes != 0 {
		t.Errorf("expected 0 bytes for malformed input, got read=%d write=%d", s.ReadBytes, s.WriteBytes)
	}
}
