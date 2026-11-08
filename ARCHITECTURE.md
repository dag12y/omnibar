# Architecture

## Proposed architecture

The application will have three main layers.

### 1. Desktop interface
Responsible for:
- Main control center
- Floating panels
- Widgets
- Notifications
- User interaction

### 2. Core services
Responsible for:
- Module lifecycle
- Settings
- Inter-module communication
- Logging
- Updates
- Permissions

### 3. Linux integration
Responsible for:
- Hyprland
- PipeWire
- systemd
- D-Bus
- Linux system interfaces
- External utilities

## Proposed technologies

- UI: Quickshell / QML
- Core: Go
- Storage: SQLite
- Configuration: JSON
- Scripts: Bash

## Important

These are initial proposals, not final technology
decisions. We will validate them before implementation.
