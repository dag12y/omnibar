#!/usr/bin/env python3
"""Record synthetic timer alerts on the e2e harness's private session bus."""
import json
import os
from pathlib import Path
import dbus
import dbus.service
from dbus.mainloop.glib import DBusGMainLoop
from gi.repository import GLib

DBusGMainLoop(set_as_default=True)
bus = dbus.SessionBus()
name = dbus.service.BusName("org.freedesktop.Notifications", bus)
log = Path(os.environ["OMNIBAR_TIMER_ALERT_LOG"])

class Notifications(dbus.service.Object):
    @dbus.service.method("org.freedesktop.Notifications", in_signature="susssasa{sv}i", out_signature="u")
    def Notify(self, app, replaces, icon, summary, body, actions, hints, timeout):
        with log.open("a") as output:
            output.write(json.dumps(dict(app=str(app), summary=str(summary), body=str(body))) + "\n")
        return dbus.UInt32(1)

service = Notifications(bus, "/org/freedesktop/Notifications")
log.with_suffix(".ready").touch()
GLib.MainLoop().run()
