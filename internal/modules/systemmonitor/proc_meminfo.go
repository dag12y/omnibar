package systemmonitor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// MemInfo is a reading of /proc/meminfo. Values are in kB as reported by
// the kernel.
type MemInfo struct {
	MemTotal    uint64 // kB
	MemFree     uint64 // kB
	MemAvailable uint64 // kB
	SwapTotal   uint64 // kB
	SwapFree    uint64 // kB
}

// ParseMeminfo reads /proc/meminfo from r and returns the parsed values.
// Missing fields are left as zero.
func ParseMeminfo(r io.Reader) (*MemInfo, error) {
	m := &MemInfo{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// Strip the trailing " kB" if present.
		value = strings.TrimSuffix(value, " kB")
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			m.MemTotal = n
		case "MemFree":
			m.MemFree = n
		case "MemAvailable":
			m.MemAvailable = n
		case "SwapTotal":
			m.SwapTotal = n
		case "SwapFree":
			m.SwapFree = n
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read /proc/meminfo: %w", err)
	}
	return m, nil
}

// ParseMeminfoFile reads /proc/meminfo from the given path.
func ParseMeminfoFile(path string) (*MemInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseMeminfo(f)
}
