# Local development and end-to-end verification

This guide covers running the Omnibar backend and Quickshell UI as separate
processes, exercising the connection lifecycle manually, and running the
automated integration checks.

## Prerequisites

- Go 1.27 or newer.
- Quickshell 0.3.2 (the pinned UI version).
- Qt 6, including `/usr/lib/qt6/bin/qmltestrunner`.
- A Wayland session for the graphical UI (Hyprland on Omarchy).

Everything runs as the invoking user. Omnibar never uses `sudo`.

## Build

```sh
make build
# equivalent to
go build -o build/omnibar ./cmd/omnibar
```

## Run the backend

```sh
./build/omnibar
# or
make run
```

The backend:

- reads `$XDG_CONFIG_HOME/omnibar/config.toml` (or
  `~/.config/omnibar/config.toml`), and uses defaults if the file is missing;
- creates `$XDG_RUNTIME_DIR/omnibar/` with mode `0700`;
- listens on `$XDG_RUNTIME_DIR/omnibar/omnibar.sock` with mode `0600`;
- logs structured `slog` records to stderr.

`XDG_RUNTIME_DIR` is mandatory. There is no fallback to `/tmp`.

A minimal configuration is enough to get started:

```sh
mkdir -p "${XDG_CONFIG_HOME:-$HOME/.config}/omnibar"
cp config/config.toml.example "${XDG_CONFIG_HOME:-$HOME/.config}/omnibar/config.toml"
```

Edit the permissions table to grant or deny capabilities. Missing entries are
denied (fail-closed). Permission changes require a backend restart in Phase 1.

## Run the UI

In a second terminal:

```sh
quickshell -p ui/qml/shell.qml
```

The UI connects to the backend socket, sends `client.handshake`, and should
show **Connected** with the `core` module. It does not build or bundle QML;
Quickshell loads the files directly.

If the UI starts before the backend, it shows **Backend unavailable** (or
**Connecting…**) and retries with exponential backoff: 1s, 2s, 4s, 8s, 16s,
then every 30s (the delay doubles and is capped at 30s).

## Manual end-to-end checklist

`make e2e-ui` automates steps 1–6 below (see
[Automated UI end-to-end check](#automated-ui-end-to-end-check)). Use this
sequence to verify the same behavior by hand and look at the real window:

1. Start the backend with `./build/omnibar`.
2. Start the UI with `quickshell -p ui/qml/shell.qml`.
3. Confirm the UI shows **Connected** and lists `core` with a green dot, which
   means the UI subscribed to its events.
4. Stop the backend with `Ctrl+C` (`SIGINT`) or `kill -TERM <pid>`.
   - The process exits with status 0.
   - The socket file is removed.
   - The UI switches to offline mode and keeps cached module data.
5. Start the backend again.
6. Confirm the UI reconnects on its own, re-sends the handshake, subscribes
   again, and returns to **Connected** with a green dot. The reconnect may take up to 30 seconds because the backoff
   is intentionally capped.
7. Run the automated checks below.

You can also inspect the protocol without the UI:

```sh
SOCKET="${XDG_RUNTIME_DIR}/omnibar/omnibar.sock"
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"client.handshake","params":{"protocol":"omnibar.ipc/v1","client_id":"manual"}}' \
  | socat - UNIX-CONNECT:"$SOCKET"
```

Expected response:

```json
{"jsonrpc":"2.0","id":1,"result":{"protocol":"omnibar.ipc/v1","version":"0.1.0","modules":["core"]}}
```

Subscribe to a module's events on the same connection (the connection must stay
open to receive pushed `module.event` notifications, hence the `sleep`):

```sh
(printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"module.subscribe","params":{"module":"core"}}'
 sleep 5) | socat - UNIX-CONNECT:"$SOCKET"
```

Expected response: `{"jsonrpc":"2.0","id":1,"result":{"module":"core","events":["*"]}}`.
The shipped `core` module publishes no events yet, so nothing else arrives until
a module that produces events lands (the system monitor, M10).

Check socket security:

```sh
ls -ld "${XDG_RUNTIME_DIR}/omnibar"
ls -l "${XDG_RUNTIME_DIR}/omnibar/omnibar.sock"
# directory: drwx------ (0700)
# socket:    srw------- (0600)
```

## Automated tests

Run everything:

```sh
make test
```

Run only the end-to-end integration tests with verbose output:

```sh
go test ./tests/integration -v
```

The integration harness builds the real `cmd/omnibar` binary in `TestMain` and
spawns it with isolated `XDG_RUNTIME_DIR` and `XDG_CONFIG_HOME` values. Tests
cover:

- handshake request/response with matching IDs, protocol version, and module
  list;
- request/response round trips and JSON-RPC error codes;
- notifications (no `id`) producing no response;
- socket and runtime-directory permissions;
- graceful shutdown (`SIGTERM`, exit 0, socket removed);
- backend restart and re-handshake over the same socket path;
- crash restart with stale-socket recovery;
- single-instance locking;
- module disablement;
- fail-closed capability checks.

### Automated UI end-to-end check

```sh
make e2e-ui
```

`scripts/e2e-ui.sh` runs the real `build/omnibar` binary and the real
`quickshell -p ui/qml/shell.qml` UI together, in an isolated
`XDG_RUNTIME_DIR` and `XDG_CONFIG_HOME`, and checks the UI's own log output:

1. the UI connects, completes the handshake, and subscribes to module events;
2. `SIGTERM` stops the backend with exit 0 and removes the socket;
3. the UI reports the backend unavailable and retries after 1000 ms, then
   2000 ms;
4. after the backend restarts, the UI reconnects, handshakes again, and
   re-subscribes (subscriptions live in backend memory);
5. the UI process survives the whole sequence.

It needs a running Wayland session (a panel flashes on screen for a few
seconds) and `quickshell` on `PATH`. It never touches your real Omnibar socket
or configuration. It is not part of `make test` because it needs a display.

Run the QML logic tests separately:

```sh
make ui-test
```

This runs the Qt Test suite under `ui/qml/tests/` with Qt 6 in offscreen mode.

### How fail-closed capability testing works

The shipped `core` module declares no capabilities, so no production RPC method
currently consumes one. The integration test in
`tests/integration/permissions_test.go` therefore registers a test-only module
that declares `system.info` and a test-only `test.useCapability` method. The
method calls the production `Registry.HasCapability` path over a real Unix
socket, with real TOML configuration loading and the production registry,
dispatcher, and framing layers.

The test verifies that access is denied when:

- the `[permissions]` table is missing;
- the permission is explicitly `false`;
- the permission is granted but the module does not declare it;
- the module is disabled;
- the module's declared capability has no matching grant.

Access is granted only when the module declares the capability, the user
grants the matching permission, and the module is enabled and healthy.

A production capability-consuming method will replace the test-only method when
the first such module lands. This is a known limitation of Phase 1: the denial
logic is covered end to end, but no shipped RPC method depends on it yet.

### Event subscription

`module.subscribe`, `module.unsubscribe`, and `module.event` notifications
follow [ADR-0009](../architecture/ADR-0009-event-subscription.md). The shipped
`core` module publishes no events, so the integration tests cover delivery two
ways:

- Against the real binary (`tests/integration/events_test.go`): subscribe and
  unsubscribe, error codes, a disabled module, graceful shutdown with a live
  subscription, and re-subscribing after a restart.
- Against an in-process backend with a real event publisher module: events
  reach only subscribed clients, filtered and in publish order; denial without
  a grant; no events from a stopped module; cleanup on disconnect.

Slow-client disconnection, reply and event ordering on one connection, oversize
events, and teardown races are covered by the unit tests in `internal/ipc` and
`internal/events`, and are worth running under `go test -race`.

## Troubleshooting

### `XDG_RUNTIME_DIR is not set`

Run from a graphical login session, or export a private runtime directory:

```sh
export XDG_RUNTIME_DIR="/run/user/$(id -u)"
```

Do not point `XDG_RUNTIME_DIR` at a shared or world-writable directory.

### `another instance is already running`

A backend already holds the single-instance lock. Stop the old process:

```sh
pkill -TERM -x omnibar
```

The lock file (`omnibar.lock`) is intentionally not deleted on shutdown to
avoid inode races. Its presence alone does not mean an instance is running.

### The UI stays on "Backend unavailable"

- Check that the socket exists and is a socket:
  `ls -l "${XDG_RUNTIME_DIR}/omnibar/omnibar.sock"`.
- Check the backend log for startup or listen errors.
- Check that the runtime directory is mode `0700`; the backend refuses unsafe
  directory permissions.
- Remember that reconnect backoff can take up to 30 seconds.

### Wrong Qt test runner

The `qmltestrunner` on `PATH` may be Qt 5. Use `make ui-test`, which defaults to
`/usr/lib/qt6/bin/qmltestrunner`, or set `QMLTESTRUNNER` explicitly.

### Stale socket

A crashed backend can leave a socket file behind. The next startup detects a
refused connection, removes only confirmed stale Unix sockets, and binds
normally. Regular files, directories, and symlinks are never removed.

## Related documents

- [Development guidelines](../../DEVELOPMENT.md)
- [Architecture](../../ARCHITECTURE.md)
- [IPC protocol ADR](../architecture/ADR-0003-ipc-protocol.md)
- [Permissions ADR](../architecture/ADR-0007-permissions-security.md)

## Panel controls

The panel fits the current screen and scrolls when content is taller. Use Tab
and Shift+Tab to move through actions, Enter or Space to activate, and Escape
to hide. Click Processor or Memory to expand details. Hover an action for its
label. The installed `omnibar-toggle` command toggles a running panel or launches
one; it can be assigned to an Omarchy keybinding or bar button. No desktop
configuration is changed by building the project.

Closing the panel pauses its system-monitor subscription. With no other clients,
the backend stops the sampling timer. Reopening requests fresh metrics and starts
a new graph segment. Long sampling gaps reset activity-rate baselines.

For IPC functions whose names overlap Quickshell commands, use the argument
separator, for example `quickshell -p ui/qml/shell.qml ipc call -- panel show`.


## Keep Awake

Grant `"power.inhibit" = true` in `[permissions]` and restart the backend. Use
the panel's 15-, 30-, or 60-minute preset. Stop releases the lease immediately;
hiding the panel leaves the session running. Backend shutdown or a crash closes
the logind lease. Quitting only the UI removes Wayland display-idle inhibition,
while the backend session continues until Stop, expiry, or backend shutdown.
Low-battery automatic cancellation is not implemented. Existing manual idle
settings and other applications' inhibitors are left alone.

`make e2e-awake` is an opt-in test on an active Hyprland desktop. It briefly
acquires real idle/sleep inhibitors and verifies Stop, expiry, hidden-panel
behavior, and shutdown cleanup. It needs Python 3, logind's system bus, and
permission to inhibit sleep. It uses isolated Omnibar config/runtime paths and
prints the temporary log directory. The regular Go/QML tests use fake leases.

For live IPC, use `keepAwake.getStatus`, `keepAwake.start` with
`{"seconds":900}`, and `keepAwake.stop`. API durations range from 1 to 86400
seconds. See [ADR-0011](../architecture/ADR-0011-keep-awake.md).


## Playback audio mixer

Select **Audio** in the panel, or call
`quickshell -p ui/qml/shell.qml ipc call -- panel selectPage audio`.
Grant `"audio.control" = true` in `[permissions]` and restart the backend.
`[modules.audio] enabled = false` disables this module. Reading, subscribing,
and actions all require the grant; without it, no audio commands are started.

The module uses `pactl` (`libpulse` on Arch) and `pipewire-pulse`. Verify the
existing service with `pactl info`. Rows represent playback streams, so an app
can have multiple rows or disappear when it closes its stream. Paused streams
can remain listed. Set a row's volume/mute, or select the default output. The
DEFAULT OUTPUT selector changes the system default: movement of existing streams
then follows WirePlumber/session-manager policy. Each playback row's OUTPUT
selector instead moves only that stream to a chosen device. A vanished device
shows unavailable; stale move requests are rejected, and failed requests restore
the last confirmed selection. No routing rules are saved by Omnibar.

Updates are event-driven with a three-second resync while subscribed. Hiding
Audio or switching to System stops this client's audio demand. A missing audio
service shows unavailable state and retries automatically; the Go backend and
other features remain available. A failed action leaves an error with Refresh.
Normal playback is never modified merely by opening Audio.

`make e2e-audio` builds the backend and runs a real UI test against isolated
PipeWire core/pulse services, WirePlumber's policy-only profile, virtual outputs,
and a silent stream. It checks volume/mute, default selection, output and stream
removal, independent two-stream routing, volume/mute/default preservation,
stale-target rejection, pulse/core restarts, and hide/show demand. It needs Python 3, `pactl`,
`pacat`, `pipewire`, `pipewire-pulse`, `wireplumber`, `quickshell`, and `hyprctl`.
It never restarts the user's audio stack or changes real playback/output devices.
Run `python3 scripts/e2e-audio.py --capture` after building to additionally save
a cropped panel screenshot (requires `grim`). Artifacts/logs are under `/tmp`.
Physical Bluetooth reconnection still needs hardware verification; the isolated
hotplug test verifies the device-list path with virtual outputs.

RPC examples and the snapshot/event schema are in
[ADR-0012](../architecture/ADR-0012-audio-mixer.md). No levels or device preferences
are persisted by Omnibar; the audio server/session manager owns that behavior.


Stream routing RPC is `audio.moveStream` with `{"key":"…","output":"sink-name"}`,
using the current stream key and output name from a snapshot. It requires the
existing `audio.control` grant. See [ADR-0013](../architecture/ADR-0013-stream-routing.md).
