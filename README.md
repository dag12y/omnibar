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
Feature modules (system monitoring and later phases) are not implemented yet.

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
