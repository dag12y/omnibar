package timers

import (
	"context"
	"fmt"
	"github.com/godbus/dbus/v5"
	"html"
)

type Notifier interface {
	Notify(context.Context, string) error
}
type DesktopNotifier struct{}

func (DesktopNotifier) Notify(ctx context.Context, name string) error {
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("desktop notification service unavailable")
	}
	defer conn.Close()
	var id uint32
	err = conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications").CallWithContext(ctx, "org.freedesktop.Notifications.Notify", dbus.FlagNoAutoStart,
		"Omnibar", uint32(0), "alarm-symbolic", "Timer finished", html.EscapeString(name), []string{}, map[string]dbus.Variant{}, int32(-1)).Store(&id)
	if err != nil || id == 0 {
		return fmt.Errorf("desktop notification unavailable; timer completed")
	}
	return nil
}
