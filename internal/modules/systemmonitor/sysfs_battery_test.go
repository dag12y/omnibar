package systemmonitor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadBatteryNonexistent(t *testing.T) {
	b, err := ReadBattery("/nonexistent/power_supply")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Present {
		t.Error("expected Present to be false for nonexistent directory")
	}
}

func TestReadBatteryEmpty(t *testing.T) {
	tmp := t.TempDir()
	b, err := ReadBattery(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Present {
		t.Error("expected Present to be false for empty directory")
	}
}

func TestReadBatteryWithPowerNow(t *testing.T) {
	tmp := t.TempDir()
	batDir := filepath.Join(tmp, "BAT0")
	if err := os.MkdirAll(batDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	os.WriteFile(filepath.Join(batDir, "capacity"), []byte("85\n"), 0644)
	os.WriteFile(filepath.Join(batDir, "status"), []byte("Charging\n"), 0644)
	os.WriteFile(filepath.Join(batDir, "power_now"), []byte("15200000\n"), 0644)

	b, err := ReadBattery(tmp)
	if err != nil {
		t.Fatalf("ReadBattery failed: %v", err)
	}
	if !b.Present {
		t.Fatal("expected Present to be true")
	}
	if b.Percent != 85 {
		t.Errorf("Percent = %f, want 85", b.Percent)
	}
	if b.State != "Charging" {
		t.Errorf("State = %q, want Charging", b.State)
	}
	if b.PowerWatts != 15.2 {
		t.Errorf("PowerWatts = %f, want 15.2", b.PowerWatts)
	}
}

func TestReadBatteryWithCurrentVoltage(t *testing.T) {
	tmp := t.TempDir()
	batDir := filepath.Join(tmp, "BAT1")
	if err := os.MkdirAll(batDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	os.WriteFile(filepath.Join(batDir, "capacity"), []byte("42\n"), 0644)
	os.WriteFile(filepath.Join(batDir, "status"), []byte("Discharging\n"), 0644)
	os.WriteFile(filepath.Join(batDir, "current_now"), []byte("1250000\n"), 0644)
	os.WriteFile(filepath.Join(batDir, "voltage_now"), []byte("12000000\n"), 0644)

	b, err := ReadBattery(tmp)
	if err != nil {
		t.Fatalf("ReadBattery failed: %v", err)
	}
	if !b.Present {
		t.Fatal("expected Present to be true")
	}
	if b.Percent != 42 {
		t.Errorf("Percent = %f, want 42", b.Percent)
	}
	if b.State != "Discharging" {
		t.Errorf("State = %q, want Discharging", b.State)
	}
	// 1250000 * 12000000 / 1e12 = 15.0 W
	if b.PowerWatts != 15.0 {
		t.Errorf("PowerWatts = %f, want 15.0", b.PowerWatts)
	}
}

func TestReadBatteryEmptyBay(t *testing.T) {
	tmp := t.TempDir()
	batDir := filepath.Join(tmp, "BAT0")
	if err := os.MkdirAll(batDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile(filepath.Join(batDir, "present"), []byte("0\n"), 0644)

	b, err := ReadBattery(tmp)
	if err != nil {
		t.Fatalf("ReadBattery failed: %v", err)
	}
	if b.Present {
		t.Error("expected Present to be false when present file is 0")
	}
}
