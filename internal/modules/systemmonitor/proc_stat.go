// Package systemmonitor provides Omnibar's built-in system monitor module.
//
// It reads /proc/stat and /proc/meminfo to report CPU, memory, and swap
// usage, publishing events and answering system.getSnapshot requests.
// All parsing is pure Go over plain files so tests can use fixtures and
// never depend on the real /proc.
package systemmonitor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// CPUTimes holds the jiffy counters for one CPU line from /proc/stat.
//
// Fields match /proc/stat in order: user, nice, system, idle, iowait,
// irq, softirq, steal, guest, guest_nice. Missing trailing fields are
// treated as zero (older kernels may not report all of them).
type CPUTimes struct {
	User      uint64
	Nice      uint64
	System    uint64
	Idle      uint64
	IOWait    uint64
	IRQ       uint64
	SoftIRQ   uint64
	Steal     uint64
	Guest     uint64
	GuestNice uint64
}

// Total returns the sum of all counters.
func (c CPUTimes) Total() uint64 {
	return c.User + c.Nice + c.System + c.Idle + c.IOWait +
		c.IRQ + c.SoftIRQ + c.Steal + c.Guest + c.GuestNice
}

// IdleTotal returns the idle plus iowait counters.
func (c CPUTimes) IdleTotal() uint64 {
	return c.Idle + c.IOWait
}

// StatSample is a single reading of /proc/stat: the aggregate "cpu" line
// and the per-core "cpuN" lines.
type StatSample struct {
	CPUTimes CPUTimes
	Cores    []CPUTimes
}

// ParseStat reads /proc/stat from r and returns the aggregate and
// per-core samples. It tolerates missing trailing fields and ignores
// lines that are not cpu or cpuN.
func ParseStat(r io.Reader) (*StatSample, error) {
	s := &StatSample{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "cpu") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		values, err := parseUint64s(fields[1:])
		if err != nil {
			return nil, fmt.Errorf("parse %s line: %w", name, err)
		}
		ct := timesFromValues(values)
		if name == "cpu" && len(name) == 3 {
			s.CPUTimes = ct
		} else if strings.HasPrefix(name, "cpu") && len(name) > 3 {
			s.Cores = append(s.Cores, ct)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read /proc/stat: %w", err)
	}
	return s, nil
}

// ParseStatFile reads /proc/stat from the given path.
func ParseStatFile(path string) (*StatSample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseStat(f)
}

// timesFromValues converts the numeric fields of a /proc/stat line into
// CPUTimes, zero-filling any missing trailing fields.
func timesFromValues(values []uint64) CPUTimes {
	c := CPUTimes{}
	if len(values) > 0 {
		c.User = values[0]
	}
	if len(values) > 1 {
		c.Nice = values[1]
	}
	if len(values) > 2 {
		c.System = values[2]
	}
	if len(values) > 3 {
		c.Idle = values[3]
	}
	if len(values) > 4 {
		c.IOWait = values[4]
	}
	if len(values) > 5 {
		c.IRQ = values[5]
	}
	if len(values) > 6 {
		c.SoftIRQ = values[6]
	}
	if len(values) > 7 {
		c.Steal = values[7]
	}
	if len(values) > 8 {
		c.Guest = values[8]
	}
	if len(values) > 9 {
		c.GuestNice = values[9]
	}
	return c
}

func parseUint64s(fields []string) ([]uint64, error) {
	out := make([]uint64, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", f, err)
		}
		out = append(out, v)
	}
	return out, nil
}
