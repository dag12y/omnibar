# Omnibar

A modular desktop utility suite for Linux, inspired by
Vorssaint and designed initially for Omarchy and Hyprland.

## Vision

Bring system monitoring, window management, productivity,
media utilities, developer tools, and desktop controls
together in one unified application.

## Principles

- Modular and extensible architecture.
- Independent and configurable features.
- Native Linux integration.
- Privacy-first and local-first.
- Reliable updates and rollback support.
- Well-tested and maintainable code.

## Project status

Phase 1 (core foundation) is complete: a Go backend and a Quickshell UI
connect over a Unix socket using JSON-RPC 2.0, with a module registry,
TOML configuration, and an Arch package with a systemd user service.
System monitoring is implemented: CPU, RAM, swap, disk capacity and I/O,
network throughput, and battery status/power. The floating Quickshell dashboard
uses the active Omarchy theme, live CPU/memory graphs, and expandable details.
Timed Keep Awake and a playback audio mixer are also implemented. System and
Audio pages share the same panel. The v0.1.0 release verification is still pending.

Run the backend with `make run` and launch the panel with
`quickshell -n -p ui/qml/shell.qml`. Click the CPU or memory card for details.
The panel opens at startup; its close button hides it and pauses page updates.
Active Keep Awake sessions continue.
Use `./scripts/toggle-panel.sh` during development, or the installed
`omnibar-toggle` command from a keybinding or bar action. Direct IPC also works:

```sh
quickshell -p /absolute/path/to/omnibar/ui/qml/shell.qml ipc call -- panel toggle
```

The same IPC target supports `show` and `hide` after the `--` separator.
Escape hides the panel; Tab and Enter/Space navigate and activate controls.
Hiding pauses the panel’s system-monitor subscription. Omarchy colors are read from
`$XDG_STATE_HOME/omarchy/current/theme/colors.toml` (default
`~/.local/state/omarchy/current/theme/colors.toml`), with a built-in fallback.
Graphs retain up to 40 readings in memory and reset on reconnect or reopening.

Keep Awake offers 15-, 30-, and 60-minute sessions with a countdown and Stop
control. It requires `"power.inhibit" = true` under `[permissions]` in your
Omnibar config (included in the example). Restart the backend after changing
grants. Sessions survive hiding the panel and end on Stop, expiry, or backend
shutdown. The Wayland display inhibitor requires the UI to remain running; the
backend holds the logind idle/sleep inhibitor. Low-battery auto-stop is not yet
implemented.

## Audio mixer

Select **Audio** to control playback streams by app name, icon, and media title.
Adjust volume from 0–100%, mute/unmute, or choose the default playback output.
Apps appear when they have an audio stream; an app with multiple streams gets
multiple rows. Paused streams may remain visible. Volume drags apply on release;
arrow keys adjust a focused slider. Each row's **OUTPUT** selector moves just
that playback stream to another connected device, preserving its volume/mute
and leaving the default output and other streams alone. The separate **DEFAULT
OUTPUT** selector changes the system default; movement of existing streams then
follows the session manager's policy. Microphone controls remain future work.

A disappeared device displays "Output unavailable" until the audio server picks
a fallback or you select another output. Omnibar stores no routing rules; new
streams and device reconnections follow the audio server/session manager policy.

The mixer uses `pactl` with PipeWire's `pipewire-pulse` service. On Arch, `libpulse`
provides `pactl`; both are package dependencies. Grant `"audio.control" = true`
under `[permissions]` and restart the backend (the example config includes it).
No audio is changed merely by opening the page. Missing grants show an
explanation; unavailable audio services reconnect automatically. Audio watchers
stop when the page is hidden or you switch back to System.

Run `make e2e-audio` for a real backend/UI test using isolated PipeWire services,
virtual outputs, and silent test streams. Your normal playback is left alone.
See [the audio design and protocol](docs/architecture/ADR-0012-audio-mixer.md)
and [stream routing](docs/architecture/ADR-0013-stream-routing.md).

## Documentation

- [Requirements](REQUIREMENTS.md)
- [Architecture](ARCHITECTURE.md)
- [Roadmap](ROADMAP.md)
- [Development guidelines](DEVELOPMENT.md)
- [Local development](docs/development/local-development.md)
- [Packaging, install, upgrade, and rollback](docs/development/packaging.md)

## License

[MIT](LICENSE)
- [Changelog](CHANGELOG.md)
