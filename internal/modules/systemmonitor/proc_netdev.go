package systemmonitor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// NetSample contains aggregated receive and transmit byte counts across active
// non-loopback network interfaces at a given timestamp.
type NetSample struct {
	Timestamp time.Time
	RxBytes   uint64
	TxBytes   uint64
}

// ParseNetdev reads /proc/net/dev from r and aggregates RX and TX bytes across
// all interfaces except the loopback ("lo") interface.
func ParseNetdev(r io.Reader, now time.Time) (*NetSample, error) {
	s := &NetSample{
		Timestamp: now,
	}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue // skip header lines
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue // ignore loopback
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		rxBytes, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		txBytes, err := strconv.ParseUint(fields[8], 10, 64)
		if err != nil {
			continue
		}

		s.RxBytes += rxBytes
		s.TxBytes += txBytes
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read /proc/net/dev: %w", err)
	}
	return s, nil
}

// ParseNetdevFile reads /proc/net/dev from the given path.
func ParseNetdevFile(path string, now time.Time) (*NetSample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseNetdev(f, now)
}

// computeNetDelta calculates RX and TX bytes/sec transfer rates between two samples.
func computeNetDelta(current, prev *NetSample) (rxBytesSec, txBytesSec uint64) {
	if prev == nil || current == nil {
		return 0, 0
	}
	elapsed := current.Timestamp.Sub(prev.Timestamp).Seconds()
	if elapsed <= 0 {
		return 0, 0
	}
	if current.RxBytes >= prev.RxBytes {
		rxBytesSec = uint64(float64(current.RxBytes-prev.RxBytes) / elapsed)
	}
	if current.TxBytes >= prev.TxBytes {
		txBytesSec = uint64(float64(current.TxBytes-prev.TxBytes) / elapsed)
	}
	return rxBytesSec, txBytesSec
}
