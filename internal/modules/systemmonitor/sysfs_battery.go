package systemmonitor

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// BatterySample holds battery state and charge metrics.
type BatterySample struct {
	Present    bool
	Percent    float64
	State      string
	PowerWatts float64
}

// readIntFromFile reads a single integer from path.
func readIntFromFile(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

// readStringFromFile reads a single trimmed string from path.
func readStringFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// ReadBattery reads battery status from the given sysfs power_supply directory.
// If no battery is found or the directory does not exist, it returns Present: false without error.
func ReadBattery(powerSupplyDir string) (*BatterySample, error) {
	entries, err := os.ReadDir(powerSupplyDir)
	if err != nil {
		// Non-existent or inaccessible power_supply directory means no battery present.
		return &BatterySample{Present: false}, nil
	}

	for _, entry := range entries {
		batDir := filepath.Join(powerSupplyDir, entry.Name())

		// Check if this power supply is a battery.
		isBat := strings.HasPrefix(entry.Name(), "BAT")
		if !isBat {
			if t, err := readStringFromFile(filepath.Join(batDir, "type")); err == nil && t == "Battery" {
				isBat = true
			}
		}
		if !isBat {
			continue
		}

		// Optional 'present' file: 0 means battery bay is empty.
		if presentStr, err := readStringFromFile(filepath.Join(batDir, "present")); err == nil {
			if presentStr == "0" {
				continue
			}
		}

		status, _ := readStringFromFile(filepath.Join(batDir, "status"))
		if status == "" {
			status = "Unknown"
		}

		capInt, _ := readIntFromFile(filepath.Join(batDir, "capacity"))
		capacity := clampPercent(float64(capInt))

		var watts float64
		if pNow, err := readIntFromFile(filepath.Join(batDir, "power_now")); err == nil && pNow > 0 {
			watts = float64(pNow) / 1e6
		} else {
			cNow, cErr := readIntFromFile(filepath.Join(batDir, "current_now"))
			vNow, vErr := readIntFromFile(filepath.Join(batDir, "voltage_now"))
			if cErr == nil && vErr == nil && cNow > 0 && vNow > 0 {
				watts = (float64(cNow) * float64(vNow)) / 1e12
			}
		}
		watts = math.Round(watts*10) / 10

		return &BatterySample{
			Present:    true,
			Percent:    capacity,
			State:      status,
			PowerWatts: watts,
		}, nil
	}

	return &BatterySample{Present: false}, nil
}
