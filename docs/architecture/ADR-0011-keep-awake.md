# ADR-0011: Timed Keep Awake

Status: Accepted

## Decision

Add the static `keep_awake` module with the `power.inhibit` capability. The
module runs without acquiring resources until an explicit start request. Like
other capabilities, a missing grant is denied; the example config grants it.

Use logind's `org.freedesktop.login1.Manager.Inhibit` method on the system bus
with `idle:sleep` and `block`. The returned file descriptor is the session's
lease: Stop, expiry, module shutdown, or process death releases it. Use godbus
for descriptor passing rather than managing an external inhibitor subprocess.
Acquisition has a three-second context deadline; failure is returned to the UI
and never reported as an active session. Restarting a session extends its timer
without creating a second lease. Timers use a generation guard so an earlier
expiry cannot stop an extended session.

The Quickshell client subscribes to `keep_awake` events independently of panel
visibility. While active and connected, a one-pixel, input-transparent layer
surface owns a Wayland `IdleInhibitor`. This inhibits compositor idle even when
the dashboard is hidden. Omarchy's shell uses an `IdleMonitor` that respects
these inhibitors. Quitting the UI releases the display inhibitor; the backend's
logind session continues until its timer ends or the backend exits.

No persistent Omarchy idle setting is modified. Existing user inhibitors remain
owned by their original applications. This does not override explicit manual
locking, lid-switch policy, or administrator policy. Automatic low-battery
cancellation is future work.

## Protocol

- `keepAwake.getStatus`: current state.
- `keepAwake.start`, params `{"seconds": 900}`: integer duration from 1 to 86400.
- `keepAwake.stop`: idempotent release.
- `module.event`, module `keep_awake`, event `state`: same status schema.

Status includes `active`, `expires_at` while active, and an optional stop
`reason`. The UI presets are 15, 30, and 60 minutes. Sessions are local,
nonpersistent, and owned by the backend rather than a particular IPC client.
All three RPC methods and subscriptions require the capability grant. Disabled
modules cannot start sessions. No administrator prompt or elevation is added.

## Validation

Unit tests use fake leases to verify denial, validation, extension, expiry, and
exactly-once release. The normal test suite never inhibits the host. An opt-in
smoke test verifies the real logind lease, Wayland surface, hide/show behavior,
manual stop, timed expiry, and backend-shutdown cleanup on an active Hyprland
session.
