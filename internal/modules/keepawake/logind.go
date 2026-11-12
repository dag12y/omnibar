package keepawake

import (
	"context"
	"io"
	"os"

	"github.com/godbus/dbus/v5"
)

// logind transfers a file descriptor: keeping it open holds the inhibition,
// and closing it (including on process death) releases it automatically.
func acquireLogind(ctx context.Context) (io.Closer, error) {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var fd dbus.UnixFD
	err = conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").CallWithContext(ctx,
		"org.freedesktop.login1.Manager.Inhibit", 0,
		"idle:sleep", "Omnibar", "Timed keep awake", "block").Store(&fd)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "omnibar-keep-awake"), nil
}
