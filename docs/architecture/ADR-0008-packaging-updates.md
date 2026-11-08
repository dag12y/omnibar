# ADR-0008: Packaging and Updates — Arch PKGBUILD + OPR

Status: Accepted

## Context

Omnibar needs an installation, upgrade, rollback, and uninstallation story for Arch Linux / Omarchy. The roadmap calls for an update manager, GitHub releases, and installation documentation. We must decide whether to use Arch package management only, or also introduce a self-updater.

## Decision

Use **Arch PKGBUILD + Omarchy Package Repository (OPR)** for distribution and updates. No self-updater binary is introduced.

## Lifecycle

| Phase | Mechanism |
|-------|-----------|
| Install | `makepkg -si` from PKGBUILD, or `omarchy pkg add omnibar` via OPR (package installation command) |
| Upgrade individual package | `pacman -Syu` |
| Omarchy system update | `omarchy update` (accounts for Omarchy's update guard and migration workflow) |
| Rollback (single package) | `pacman -U <previous-package>.pkg.tar.zst` from the package cache |
| Rollback (entire system) | Roll back an entire Omarchy system update using a system snapshot |
| Uninstall | `pacman -Rs omnibar` (user data in XDG directories is preserved) |
| Complete data removal | Manual deletion of `$XDG_CONFIG_HOME/omnibar`, `$XDG_DATA_HOME/omnibar`, `$XDG_STATE_HOME/omnibar` |

## Database migration compatibility

- New package versions may include SQLite schema migrations.
- Migrations run automatically on first start after upgrade.
- Migrations are transactional and additive.
- A safe SQLite backup (online backup API or controlled checkpoint+shutdown) is performed before major migrations.

## User-data preservation

- Config: `$XDG_CONFIG_HOME/omnibar/config.toml` — preserved across upgrades and uninstalls
- Data: `$XDG_DATA_HOME/omnibar/omnibar.db` — preserved across upgrades and uninstalls
- Logs/state: `$XDG_STATE_HOME/omnibar/` — preserved across upgrades and uninstalls

## Why no self-updater

- Arch users expect pacman/OPR to manage software updates
- A self-updater adds complexity, a security surface (code execution with user privileges), and redundancy
- Package manager rollback (`pacman -U`) is a well-understood mechanism
- Revisit only if OPR distribution proves insufficient

## OPR publication

Creating a PKGBUILD alone does not publish the package. OPR publication requires a package recipe contribution and acceptance by the OPR maintainers.

## Rollback distinction

- **Downgrading one package:** `pacman -U <previous-version>.pkg.tar.zst` reverts only the omnibar package to a previous version.
- **Rolling back an entire Omarchy system update:** Uses a system snapshot (e.g., filesystem snapshot or boot environment) to restore the entire system state. These are distinct operations with different scopes.

## Consequences

**Positive:**
- Fits the Arch/Omarchy user's existing workflow
- Rollback is native to pacman or system snapshots
- User data survives upgrades and uninstalls
- No self-updater security surface to maintain

**Negative:**
- Users must use pacman or OPR to update (no in-app update button)
- Offline updates require manual package handling
- OPR publication is not automatic; it requires maintainer acceptance

## References

- Arch PKGBUILD documentation: https://wiki.archlinux.org/title/PKGBUILD
- Omarchy update flow: `omarchy update`
