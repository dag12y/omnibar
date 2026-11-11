package systemmonitor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseStatNormal(t *testing.T) {
	path := filepath.Join("fixtures", "stat_normal")
	s, err := ParseStatFile(path)
	if err != nil {
		t.Fatalf("ParseStatFile() returned error: %v", err)
	}

	// Aggregate line.
	if s.CPUTimes.User != 100 || s.CPUTimes.Nice != 200 || s.CPUTimes.System != 300 {
		t.Errorf("aggregate CPUTimes = %+v, want User=100 Nice=200 System=300", s.CPUTimes)
	}
	if s.CPUTimes.Total() != 4685 {
		t.Errorf("aggregate Total() = %d, want 4685", s.CPUTimes.Total())
	}
	if s.CPUTimes.IdleTotal() != 4050 {
		t.Errorf("aggregate IdleTotal() = %d, want 4050", s.CPUTimes.IdleTotal())
	}

	// Per-core lines.
	if len(s.Cores) != 2 {
		t.Fatalf("expected 2 cores, got %d", len(s.Cores))
	}
	if s.Cores[0].Total() != 2344 {
		t.Errorf("cpu0 Total() = %d, want 2344", s.Cores[0].Total())
	}
	if s.Cores[1].Total() != 2341 {
		t.Errorf("cpu1 Total() = %d, want 2341", s.Cores[1].Total())
	}
}

func TestParseStatIgnoresNonCPULines(t *testing.T) {
	path := filepath.Join("fixtures", "stat_normal")
	// Append non-cpu lines to a temp copy.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	content = append(content, []byte("intr 12345 67890\nctxt 987654\nbtime 1234567890\n")...)

	dir := t.TempDir()
	tmp := filepath.Join(dir, "stat_mixed")
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		t.Fatalf("write temp fixture: %v", err)
	}

	s, err := ParseStatFile(tmp)
	if err != nil {
		t.Fatalf("ParseStatFile() returned error: %v", err)
	}
	if len(s.Cores) != 2 {
		t.Errorf("expected 2 cores, got %d", len(s.Cores))
	}
	if s.CPUTimes.Total() != 4685 {
		t.Errorf("aggregate Total() = %d, want 4685", s.CPUTimes.Total())
	}
}

func TestParseStatMissingTrailingFields(t *testing.T) {
	// Line with fewer than 10 fields; missing ones default to zero.
	content := `cpu  100 200 300 4000
cpu0 50 100 150 2000 25
`
	dir := t.TempDir()
	tmp := filepath.Join(dir, "stat_short")
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp fixture: %v", err)
	}

	s, err := ParseStatFile(tmp)
	if err != nil {
		t.Fatalf("ParseStatFile() returned error: %v", err)
	}
	// cpu line has only 4 fields; IOWait onward should be zero.
	if s.CPUTimes.IOWait != 0 || s.CPUTimes.IRQ != 0 || s.CPUTimes.GuestNice != 0 {
		t.Errorf("aggregate missing fields should be zero: %+v", s.CPUTimes)
	}
	// cpu0 line has 5 fields; IRQ onward should be zero.
	if s.Cores[0].IRQ != 0 || s.Cores[0].GuestNice != 0 {
		t.Errorf("core missing fields should be zero: %+v", s.Cores[0])
	}
}

func TestComputeSnapshotDelta(t *testing.T) {
	first, err := ParseStatFile(filepath.Join("fixtures", "stat_normal"))
	if err != nil {
		t.Fatalf("parse first: %v", err)
	}
	second, err := ParseStatFile(filepath.Join("fixtures", "stat_second"))
	if err != nil {
		t.Fatalf("parse second: %v", err)
	}

	snap := computeSnapshot(second, &MemInfo{}, first, nil, nil, nil, "")
	// totalDelta = 9370-4685 = 4685, idleDelta = 8100-4050 = 4050
	// percent = (4685-4050)/4685 * 100 = 13.55... -> 13.6
	if snap.CPU.Percent < 13.5 || snap.CPU.Percent > 13.7 {
		t.Errorf("CPU.Percent = %f, want ~13.6", snap.CPU.Percent)
	}
	if len(snap.CPU.Cores) != 2 {
		t.Fatalf("expected 2 core percentages, got %d", len(snap.CPU.Cores))
	}
	// cpu0: (2344-2025)/2344 * 100 = 13.61 -> 13.6
	if snap.CPU.Cores[0] < 13.5 || snap.CPU.Cores[0] > 13.7 {
		t.Errorf("CPU.Cores[0] = %f, want ~13.6", snap.CPU.Cores[0])
	}
	// cpu1: (2341-2025)/2341 * 100 = 13.50 -> 13.5
	if snap.CPU.Cores[1] < 13.4 || snap.CPU.Cores[1] > 13.6 {
		t.Errorf("CPU.Cores[1] = %f, want ~13.5", snap.CPU.Cores[1])
	}
	if snap.SampledAt == "" {
		t.Error("SampledAt must not be empty")
	}
}

func TestComputeSnapshotNoBaseline(t *testing.T) {
	first, err := ParseStatFile(filepath.Join("fixtures", "stat_normal"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	snap := computeSnapshot(first, &MemInfo{}, nil, nil, nil, nil, "")
	if snap.CPU.Percent != 0 {
		t.Errorf("CPU.Percent = %f, want 0 with no baseline", snap.CPU.Percent)
	}
	if len(snap.CPU.Cores) != 0 {
		t.Errorf("expected no cores with no baseline, got %d", len(snap.CPU.Cores))
	}
}

func TestComputeSnapshotCounterWrap(t *testing.T) {
	first, err := ParseStatFile(filepath.Join("fixtures", "stat_wrap"))
	if err != nil {
		t.Fatalf("parse first: %v", err)
	}
	second, err := ParseStatFile(filepath.Join("fixtures", "stat_normal"))
	if err != nil {
		t.Fatalf("parse second: %v", err)
	}

	snap := computeSnapshot(second, &MemInfo{}, first, nil, nil, nil, "")
	// totalDelta is negative (counter reset), so percent clamps to 0.
	if snap.CPU.Percent != 0 {
		t.Errorf("CPU.Percent = %f, want 0 on counter wrap", snap.CPU.Percent)
	}
}
