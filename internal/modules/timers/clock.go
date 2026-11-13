package timers

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type Sample struct {
	WallMS int64
	BootNS int64
	BootID string
}
type Clock interface{ Sample() (Sample, error) }
type LinuxClock struct{ bootID string }

func (c *LinuxClock) Sample() (Sample, error) {
	if c.bootID == "" {
		b, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
		if err != nil {
			return Sample{}, fmt.Errorf("timer boot clock unavailable")
		}
		c.bootID = strings.TrimSpace(string(b))
	}
	var ts syscall.Timespec
	// CLOCK_BOOTTIME includes suspended time and is independent of wall time.
	_, _, errno := syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 7, uintptr(unsafe.Pointer(&ts)), 0)
	if errno != 0 {
		return Sample{}, fmt.Errorf("timer boot clock unavailable")
	}
	return Sample{WallMS: time.Now().UnixMilli(), BootNS: ts.Nano(), BootID: c.bootID}, nil
}
