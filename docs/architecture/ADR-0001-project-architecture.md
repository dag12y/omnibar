# ADR-0001: Initial Project Architecture

Status: Superseded

Superseded by: ADR-0002 through ADR-0008

## Context

Omnibar aims to provide a unified suite of desktop utilities for Linux, initially targeting Omarchy and Hyprland. The original proposal evaluated a modular architecture consisting of Quickshell/QML for the desktop interface, Go for core services and system integrations, and SQLite for persistent application data.

## Decision

The original proposal's technology choices (Quickshell/QML, Go, SQLite) are confirmed and refined in ADR-0002 through ADR-0008. This ADR is retained for historical context only.

## Consequences

- UI and system services evolve independently.
- Modules can be developed and tested separately.
- Linux-specific integrations need compatibility checks.
- Communication between components needs a defined protocol.

## Open questions (resolved)

- Should modules run in-process or as separate services? → Resolved: hybrid model (ADR-0004). Built-in modules are in-process by default; out-of-process when reliability or security demands it.
- How should module permissions work? → Resolved: declarative capability-based policy enforcement (ADR-0007).
- How should module updates and compatibility be handled? → Resolved: Arch package management; no self-updater (ADR-0008).

## Alternatives considered

Documented in ADR-0002 (UI framework), ADR-0003 (IPC protocol), ADR-0004 (module execution model), ADR-0005 (configuration format), ADR-0006 (storage), ADR-0007 (permissions), ADR-0008 (packaging).

## Trade-offs

Documented in the respective ADRs.
