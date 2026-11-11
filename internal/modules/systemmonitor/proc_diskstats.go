package systemmonitor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DiskSample contains total read and write byte counts across primary block
// devices, and the timestamp the sample was collected.
type DiskSample struct {
	Timestamp  time.Time
	ReadBytes  uint64
	WriteBytes uint64
}

// hasDigit reports whether s contains any ASCII digit.
func hasDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return true
		}
	}
	return false
}

// isPrimaryDisk reports whether devName represents a whole physical or virtual
// block device rather than a partition or virtual loop device.
func isPrimaryDisk(name string) bool {
	if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "zram") {
		return false
	}
	if strings.HasPrefix(name, "nvme") {
		// nvme0n1 is primary, nvme0n1p1 is partition
		return strings.Contains(name, "n") && !strings.Contains(name, "p")
	}
	if strings.HasPrefix(name, "mmcblk") {
		// mmcblk0 is primary, mmcblk0p1 is partition
		return !strings.Contains(name, "p")
	}
	for _, prefix := range []string{"sd", "hd", "vd", "xvd"} {
		if strings.HasPrefix(name, prefix) {
			rest := strings.TrimPrefix(name, prefix)
			if len(rest) > 0 && !hasDigit(rest) {
				return true
			}
		}
	}
	return false
}

// ParseDiskstats reads /proc/diskstats from r and returns the aggregated read
// and write byte counts across all primary disk devices.
func ParseDiskstats(r io.Reader, now time.Time) (*DiskSample, error) {
	s := &DiskSample{
		Timestamp: now,
	}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}
		name := fields[2]
		if !isPrimaryDisk(name) {
			continue
		}
		sectorsRead, err := strconv.ParseUint(fields[5], 10, 64)
		if err != nil {
			continue
		}
		sectorsWritten, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}

		// Linux /proc/diskstats sector size is always 512 bytes.
		s.ReadBytes += sectorsRead * 512
		s.WriteBytes += sectorsWritten * 512
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read /proc/diskstats: %w", err)
	}
	return s, nil
}

// ParseDiskstatsFile reads /proc/diskstats from the given path.
func ParseDiskstatsFile(path string, now time.Time) (*DiskSample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseDiskstats(f, now)
}

// computeDiskDelta calculates read and write bytes/sec between two disk samples.
func computeDiskDelta(current, prev *DiskSample) (readBytesSec, writeBytesSec uint64) {
	if prev == nil || current == nil {
		return 0, 0
	}
	elapsed := current.Timestamp.Sub(prev.Timestamp).Seconds()
	if elapsed <= 0 {
		return 0, 0
	}
	if current.ReadBytes >= prev.ReadBytes {
		readBytesSec = uint64(float64(current.ReadBytes-prev.ReadBytes) / elapsed)
	}
	if current.WriteBytes >= prev.WriteBytes {
		writeBytesSec = uint64(float64(current.WriteBytes-prev.WriteBytes) / elapsed)
	}
	return readBytesSec, writeBytesSec
}

// FsStatFunc queries storage usage for a given filesystem mount point.
type FsStatFunc func(path string) (total, available, used uint64, err error)

// DefaultFsStat queries filesystem usage via syscall.Statfs.
func DefaultFsStat(path string) (total, available, used uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	bsize := uint64(stat.Bsize)
	total = stat.Blocks * bsize
	available = stat.Bavail * bsize
	used = (stat.Blocks - stat.Bfree) * bsize
	return total, available, used, nil
}
