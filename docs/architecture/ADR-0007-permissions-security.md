# ADR-0007: Permissions and Security — Capability-Based Policy Enforcement

Status: Accepted

## Context

Omnibar modules will access sensitive resources: clipboard, screen capture, audio, window management, processes, filesystem, network, and packages. We need a permission model that follows least privilege without introducing unneeded complexity in Phase 1.

## Decision

Use a **declarative capability-based policy enforcement model**:

- Each module declares the permissions it requires in its `ModuleInfo`.
- The core enforces that a module cannot use a backend capability without the declared permission.
- Permissions are granted by the user via configuration (`config.toml`).
- Omnibar never uses `sudo` or escalates privileges; all operations run as the invoking user.

## Permissions

| Permission | Description | Risk |
|------------|-------------|------|
| `system.info` | Read CPU, RAM, disk, battery from /proc and sysfs | Low |
| `window.manage` | Query and control windows via Hyprland IPC | Medium |
| `clipboard.read` | Read clipboard contents | Medium |
| `clipboard.write` | Write to clipboard | Medium |
| `audio.control` | Volume, routing via PipeWire | Medium |
| `screen.capture` | Screenshot/screencast via xdg-desktop-portal | High |
| `process.manage` | List, signal, kill processes | High |
| `filesystem.access` | Read/write user files | High |
| `network.access` | Outgoing network connections | Medium |
| `package.manage` | Install/remove packages via pacman | Critical |

## Fault isolation vs security sandboxing

This is an important distinction:

- **Fault isolation** (out-of-process services): Prevents a module crash from taking down the entire application. A crashing clipboard manager cannot corrupt the core or the UI. **Implemented in Phase 1.**

- **Security sandboxing** (OS-level restrictions): Prevents a module from accessing OS resources beyond its permission. **NOT implemented in Phase 1.**

All Omnibar modules run as the same user. Same-user processes are **not** isolated from each other's OS-level permissions. A module with `filesystem.access` can read any file the user can read, regardless of permission declarations. The permission model is a policy enforcement layer within Omnibar, not an OS sandbox.

Actual OS-level sandboxing (systemd `RestrictNamespaces=`, `PrivateTmp=`, `ProtectSystem=`, `ProtectHome=`) is explicitly deferred to future work.

## Permission enforcement

- Permissions are checked at the core boundary when a module requests a capability.
- A module without `screen.capture` cannot initiate a screenshot.
- Changing permissions requires editing `config.toml` and restarting Omnibar (Phase 1); a settings UI for permission management may come later.
- Modules are encouraged to request only the permissions they need (least privilege).

## Consequences

**Positive:**
- Simple, understandable model
- No OS sandboxing complexity in Phase 1
- User has explicit control over what modules can do
- Clear upgrade path to OS sandboxing later

**Negative:**
- Permissions are not enforced at the OS level in Phase 1
- A compromised or buggy module has the same OS access as the user

## Trade-offs

We accept the lack of OS-level sandboxing in Phase 1 to keep the architecture simple and avoid systemd hardening complexity. The fault-isolation boundary (out-of-process services) still provides meaningful crash protection. OS sandboxing is a well-understood future enhancement.

## References

- systemd.exec sandboxing options: https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html
