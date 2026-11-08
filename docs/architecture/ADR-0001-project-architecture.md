# ADR-0001: Initial Project Architecture

Status: Proposed

## Context

Omnibar aims to provide a unified suite of desktop utilities
for Linux, initially targeting Omarchy and Hyprland.

## Decision

Evaluate a modular architecture consisting of:

- Quickshell/QML for the desktop interface.
- Go for core services and system integrations.
- SQLite for persistent application data.

## Consequences

- UI and system services can evolve independently.
- Modules can be developed and tested separately.
- Linux-specific integrations need compatibility checks.
- Communication between components needs a defined protocol.

## Open questions

- Should modules run in-process or as separate services?
- How should module permissions work?
- How should module updates and compatibility be handled?
