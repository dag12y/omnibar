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
The v0.1.0 release verification is still pending.

Run the backend with `make run` and launch the panel with
`quickshell -n -p ui/qml/shell.qml`. Click the CPU or memory card for details.
The panel opens at startup; its close button hides it while monitoring stays active.
Toggle it from a keybinding or bar action with:

```sh
quickshell -p /absolute/path/to/omnibar/ui/qml/shell.qml ipc call panel toggle
```

The same IPC target supports `show` and `hide`. Omarchy colors are read from
`$XDG_STATE_HOME/omarchy/current/theme/colors.toml` (default
`~/.local/state/omarchy/current/theme/colors.toml`), with a built-in fallback.
Graphs retain up to 40 readings in memory and reset on reconnect.

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
