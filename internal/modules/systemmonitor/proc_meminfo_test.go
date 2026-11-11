package systemmonitor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMeminfoNormal(t *testing.T) {
	m, err := ParseMeminfoFile(filepath.Join("fixtures", "meminfo_normal"))
	if err != nil {
		t.Fatalf("ParseMeminfoFile() returned error: %v", err)
	}

	if m.MemTotal != 16384000 {
		t.Errorf("MemTotal = %d, want 16384000", m.MemTotal)
	}
	if m.MemAvailable != 9000000 {
		t.Errorf("MemAvailable = %d, want 9000000", m.MemAvailable)
	}
	if m.SwapTotal != 2097148 {
		t.Errorf("SwapTotal = %d, want 2097148", m.SwapTotal)
	}
	if m.SwapFree != 1048576 {
		t.Errorf("SwapFree = %d, want 1048576", m.SwapFree)
	}
}

func TestParseMeminfoNoSwap(t *testing.T) {
	m, err := ParseMeminfoFile(filepath.Join("fixtures", "meminfo_no_swap"))
	if err != nil {
		t.Fatalf("ParseMeminfoFile() returned error: %v", err)
	}

	if m.SwapTotal != 0 {
		t.Errorf("SwapTotal = %d, want 0 (no swap)", m.SwapTotal)
	}
	if m.SwapFree != 0 {
		t.Errorf("SwapFree = %d, want 0 (no swap)", m.SwapFree)
	}
	if m.MemTotal != 16384000 {
		t.Errorf("MemTotal = %d, want 16384000", m.MemTotal)
	}
}

func TestParseMeminfoIgnoresUnknownKeys(t *testing.T) {
	content := `MemTotal:       1000 kB
FooBar:         123 kB
MemAvailable:   500 kB
`
	dir := t.TempDir()
	tmp := filepath.Join(dir, "meminfo")
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp fixture: %v", err)
	}

	m, err := ParseMeminfoFile(tmp)
	if err != nil {
		t.Fatalf("ParseMeminfoFile() returned error: %v", err)
	}
	if m.MemTotal != 1000 || m.MemAvailable != 500 {
		t.Errorf("unexpected values: %+v", m)
	}
}
