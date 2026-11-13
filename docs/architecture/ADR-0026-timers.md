# ADR-0026: Durable countdown timers

Status: Accepted for M31, v0.3.0 development.

## Decision

Add a timers module under timers.manage. The grant covers only Omnibar-owned
timer state and explicitly requested timer completion notifications, not general
filesystem access or arbitrary notification messages. Disabled/denied modules
must not load timer files, sample clocks, schedule work, or send notifications.

Persist private atomic versioned JSON at `$XDG_DATA_HOME/omnibar/timers.json`
(default `~/.local/share/omnibar/timers.json`). Limit unfinished timers to 10,
completed history to the latest 20, file size to 64 KiB, names to 64 UTF-8 bytes,
and duration to 1–86400 seconds. Corrupt/insecure files are preserved. Failed
writes leave acknowledged state unchanged. Names are plain text and never logs.

## Timing and restart

Use Linux CLOCK_BOOTTIME for countdown deadlines: it is monotonic, includes
suspend, and ignores discontinuous wall-clock changes. Persist boot ID and boot
clock deadlines so backend restarts in the same boot retain those semantics.
Persist a UTC deadline as a fallback after reboot. On a new boot, recover from
UTC and clamp remaining time to the original duration (backward clock changes
cannot extend beyond a full timer). Already elapsed deadlines complete on start.
Paused timers preserve remaining milliseconds and do not count down while off.
After reboot, wall-clock accuracy necessarily determines offline elapsed time.

An active backend checks deadlines once per second and after commands, and sleeps entirely
when no timer is running. No inhibitor, wake alarm, per-second disk write, or
hidden frontend polling is introduced. Suspend can delay an alert until wake;
the backend then detects expiry within its next scheduled check. Public snapshots
contain remaining milliseconds, not internal nanosecond timestamps.

## State and alerts

Transitions are running → paused → running, running → completed, and explicit
Cancel (running/paused removal) or Dismiss (completed removal). Per-timer expected
revisions protect commands from stale clients. Start accepts an optional opaque
ID as an idempotency key; a retry with identical name/duration returns that timer.
Reusing an ID with different parameters fails. Data revisions stay within exact
JavaScript integer bounds. Each mutation persists before acknowledgment/event.

Completion and its alert claim are written atomically before any D-Bus call.
Send at most one notification attempt per timer through org.freedesktop.Notifications
on the session bus, with bounded time and escaped body text. Failure is visible
in the timer row and never removes completion. Claim-before-send prevents replay
across crashes/restarts; a crash between claim and delivery may lose an alert.
Exactly-once delivery cannot be guaranteed across a file and external service.
Completion remains visible in Omnibar. No automatic notification retries occur.

Notification results are ephemeral row metadata; the durable completed state
means an attempt was claimed. Notifications may be delayed by a slow service,
but timer mutations and reads remain responsive. Stop cancels notification work
and waits for workers so nothing is sent after module shutdown completes.

## RPC/UI

RPC methods: timers.getSnapshot, timers.start ({name,seconds,id?}), timers.pause,
timers.resume, timers.cancel, timers.dismiss ({id,revision}). Every method checks
module availability and timers.manage before work. Return revision, timer rows,
running/paused/completed counts, and optional storageError.

The Timers page provides 5/15/25/60-minute presets, custom minutes/seconds, named
rows, remaining time, and state-appropriate actions. A compact header indicator
shows running/paused timers and unacknowledged completions. The visible page
polls; hidden pages rely on lightweight state events, with snapshot refresh on
reopen/reconnect. The frontend countdown is only presentation: completion is
always backend-confirmed. Refresh/failed/denied states stay explicit.

## Verification

Fake-clock tests cover forward/backward wall-clock jumps, suspend, same/new boot
restart, paused recovery, expiry, counts/bounds, stale revisions, persistence
failure, notification failure, no replay, and shutdown. RPC and QML checks cover
gates, input validation, action targets, formatting, and hidden polling behavior.
Real tests use synthetic timers and a private session bus notification stub,
private XDG storage, and a nested compositor; never send test alerts to the host.
Package lifecycle checks preserve timers alongside notes/favorites/layouts/config.

References: [Linux clocks](https://man7.org/linux/man-pages/man2/clock_gettime.2.html),
[desktop notification protocol](https://specifications.freedesktop.org/notification/latest/protocol.html).
