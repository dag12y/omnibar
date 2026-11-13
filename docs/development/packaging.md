# Packaging, install, upgrade, and rollback

Omnibar is distributed as an Arch package built from `pkgbuild/PKGBUILD`
(ADR-0008). There is no self-updater: pacman owns installs, upgrades, and
rollbacks.

## What the package installs

| Path | Purpose |
|------|---------|
| `/usr/bin/omnibar` | Go backend |
| `/usr/bin/omnibar-toggle` | Toggle the panel or launch it if absent |
| `/usr/bin/omnibar-ui` | Launcher: `quickshell -n -p /usr/share/omnibar/qml/shell.qml` |
| `/usr/lib/systemd/user/omnibar.service` | Per-user backend service (not a template unit) |
| `/usr/share/omnibar/qml/` | Quickshell UI (`shell.qml`, `components/`) |
| `/usr/share/doc/omnibar/config.toml.example` | Example configuration |
| `/usr/share/doc/omnibar/README.md` | README |
| `/usr/share/licenses/omnibar/LICENSE` | MIT license |

The package writes nothing to `$HOME` or any XDG directory. User data lives in:

- `$XDG_CONFIG_HOME/omnibar/config.toml`
- `$XDG_DATA_HOME/omnibar/`
- `$XDG_STATE_HOME/omnibar/`

The UI is launched with `quickshell -p` and an absolute path rather than
`quickshell -c omnibar`. That keeps it independent of `XDG_CONFIG_DIRS` and
stops a stray `~/.config/quickshell/omnibar` from shadowing the packaged UI. It
is also how Omarchy launches its own shell (`/usr/share/omarchy/shell`).

Runtime dependencies are `quickshell`, `qt6-declarative`, `glibc`, `systemd`,
`libpulse` (provides `pactl`), `pipewire-pulse`, `wl-clipboard`, and
`xdg-utils`. The audio module uses the
user's existing PipeWire service and session manager. Installing Omnibar does
not restart audio services or change output selection. The only build dependency
is `go`. Go dependencies (`BurntSushi/toml` and `godbus/dbus/v5`) are downloaded
in `prepare()` into `$srcdir`, so `makepkg` needs network access. Existing user
configs are preserved: add an `audio.control` grant explicitly after upgrading
if it is absent/false, then restart the backend.

## Build a package from your working tree

```sh
make package
# equivalent to scripts/package-local.sh
```

This tars the working tree (tracked and untracked files that are not
git-ignored), runs `makepkg` against it, runs `go test ./...` in `check()`, and
writes `dist/omnibar-<version>-<rel>-<arch>.pkg.tar.zst`. Nothing is installed.
Pass extra `makepkg` flags after the script, for example
`scripts/package-local.sh --nocheck`.

Inspect the result without installing:

```sh
pacman -Qip dist/omnibar-*.pkg.tar.zst   # metadata and dependencies
pacman -Qlp dist/omnibar-*.pkg.tar.zst   # file list
```

## Install

```sh
sudo pacman -U dist/omnibar-0.1.0-1-x86_64.pkg.tar.zst
```

Then start the backend and the UI:

```sh
systemctl --user enable --now omnibar.service
omnibar-ui
```

Notes:

- Installing does not enable or start anything. Enabling the service is your
  choice.
- The UI is not autostarted. Run `omnibar-ui` yourself, or add it to your
  Hyprland autostart. If the backend is down it shows "Backend unavailable" and
  reconnects on its own.
- Optional: copy the example config into place and edit the permissions table
  (missing entries are denied):

  ```sh
  mkdir -p ~/.config/omnibar
  cp /usr/share/doc/omnibar/config.toml.example ~/.config/omnibar/config.toml
  ```

Check it:

```sh
systemctl --user status omnibar.service
journalctl --user -u omnibar.service -f
ls -l "$XDG_RUNTIME_DIR/omnibar/omnibar.sock"
omnibar --version
```

## Upgrade

```sh
sudo pacman -Syu            # once published, or
sudo pacman -U dist/omnibar-<new>.pkg.tar.zst
systemctl --user restart omnibar.service
```

The running service keeps using the old binary until restarted. Restart the UI
too, because it loads QML at start. Omarchy system updates go through
`omarchy update`.

## Rollback

Downgrade only Omnibar to a package still in the pacman cache or in `dist/`:

```sh
ls /var/cache/pacman/pkg/omnibar-*
sudo pacman -U /var/cache/pacman/pkg/omnibar-<previous>.pkg.tar.zst
systemctl --user restart omnibar.service
```

Rolling back a whole Omarchy system update is a separate operation that uses a
system snapshot.

## Uninstall

```sh
systemctl --user disable --now omnibar.service
sudo pacman -Rs omnibar
```

Your configuration, data, and state are kept. To remove them as well:

```sh
rm -rf ~/.config/omnibar ~/.local/share/omnibar ~/.local/state/omnibar
```

## Releasing

1. Bump `pkgver` in `pkgbuild/PKGBUILD` and `ServerVersion` in
   `internal/ipc/handshake.go` together. `tests/packaging` fails if they differ.
2. Update `CHANGELOG.md`, commit, then tag and push: `git tag v<version>`,
   `git push origin v<version>`.
3. In `pkgbuild/`, run `updpkgsums` to replace `sha256sums=('SKIP')` with the
   checksum of the GitHub tag tarball, and reset `pkgrel=1`.
4. Verify a clean build with `makepkg -f` in `pkgbuild/`.

The release recipe verifies the published tag tarball. `make package` computes
a checksum for its working-tree tarball and substitutes that checksum only in
the temporary recipe; it leaves `pkgbuild/PKGBUILD` unchanged.

## Publishing to OPR

Committing a PKGBUILD does not publish anything. Omarchy Package Repository
(OPR) publication is a separate step: contribute a package recipe to the OPR and
have its maintainers accept it. That is outside this repository.

### Clipboard history dependency

M20 adds `wl-clipboard` as a runtime dependency for text collection and copy-back.
The example config enables the module but leaves `clipboard.read` and
`clipboard.write` false. Installing/upgrading starts no collection. After granting
read access and restarting the backend, select Start history in the Clipboard
page for that session. Copy requires the separate write grant. History is
memory-only and lost on backend restart; the current system selection may persist.
The private compositor test is `make e2e-clipboard`; it installs no package.


### Saved clipboard favorites

M21 saves only explicitly selected favorites in
`$XDG_DATA_HOME/omnibar/clipboard-favorites.json` (default
`~/.local/share/omnibar/clipboard-favorites.json`), using private permissions and
atomic replacement. No new dependency or permission grant is needed. Recent
copies remain session-only; saved favorites restore after upgrade/restart while
collection stays paused. Package removal preserves user data. Back up this JSON
file for favorites; it contains unencrypted saved text. Invalid saved data is
reported and preserved. See [ADR-0016](../architecture/ADR-0016-clipboard-favorites.md).


### File shelf dependency and grants

M22 adds `xdg-utils` for directory opening. File shelf uses the existing
`wl-clipboard` dependency to copy path text. All shelf access requires
`filesystem.access`; copying additionally requires `clipboard.write`. The
example leaves both false and enables the module without accessing user paths.
Users set grants in their existing config and restart the backend; upgrades do
not replace that config. References remain session-only. Clear/remove/package
removal never remove originals. `make e2e-shelf` verifies synthetic files in a
private Wayland environment and records opener arguments without opening a host
file manager. See [ADR-0017](../architecture/ADR-0017-file-shelf.md).


### Clipboard utilities

M23 reuses `wl-clipboard` and the existing `clipboard.read`/`clipboard.write`
grants for Transform & copy. Package/config/storage formats are unchanged.
Reinstall the rebuilt package, restart the backend, and relaunch the UI to use
the new actions. See [ADR-0018](../architecture/ADR-0018-clipboard-transforms.md).


### Running-window switcher

M24 uses hyprctl from the optional Hyprland package dependency. No new required
package is introduced. The example enables the module but leaves window.manage
false; listing/focus require the grant and a compositor instance in the backend
session environment. After upgrading, restart the service and relaunch the UI.
No desktop keybindings/config are installed or changed. Titles are not persisted.
See [ADR-0019](../architecture/ADR-0019-window-switcher.md) and the local development
troubleshooting note for the user-service compositor environment.


## v0.1.0 release verification

See [the verification record](../releases/v0.1.0-verification.md) for the
2026-11-13 published release, final source/package digests, and verification
limits. M15 is complete for x86_64. Source verification and isolated package
lifecycle checks are recorded separately from the earlier working-tree candidate.


### Isolated package lifecycle verification

Run `python3 scripts/verify-package-lifecycle.py OLDER_PACKAGE NEWER_PACKAGE`
with pacman, fakeroot, and libarchive installed. The older fixture must have a
lower package version/release and different package contents. The script uses
a temporary root/database and synthetic home files, checks upgrade/reinstall,
restores the exact old files and modes on downgrade, and checks that removal preserves
config/data/state before reinstalling. Dependencies are skipped because this is
a package transaction check, not a bootable guest; runtime integration is checked
separately with the installed service and package UI smoke test. Packages with
install scriptlets or libalpm hooks are rejected.


### Application launcher (0.2.0 development)

M25 adds `glib2` for `gio launch` and the Apps page. The application_launcher
module is enabled by default; its `process.manage` grant is false in the
example. Existing configs are preserved. Add the grant explicitly to enable
catalog access and app launching, then restart the backend and relaunch the UI.
Terminal/D-Bus applications rely on the desktop's GIO setup. No launch history
or autostart changes are installed. Build working-tree packages with
`make package`; the future 0.2.0 tag checksum will be finalized at release.


### Unified search (0.2.0 development, package release 2)

M26 adds Search using existing app/window snapshots and grants. No new runtime
dependency or config migration is required. Rebuild/reinstall the working-tree
package and restart the UI to load the new components. The backend protocol
and version remain compatible with M25. `make e2e-search` checks keyboard app
launch and window focus in a private compositor; `make ui-test` covers partial
source permissions, stable selection, bounds, and query disposal.


### Window previews (0.2.0 development, package release 3)

M27 adds optional `grim>=1.5.0` for exact-window preview capture. Both
window.manage and screen.capture are required; the example leaves capture
false. Upgrading preserves existing configs. Grant capture explicitly and
restart the backend/UI to load the new RPC and controls. The standalone Windows
and Search pages expose Preview per running window; there is no automatic
capture on startup or page navigation and no image storage directory. The
private `make e2e-previews` check also needs ImageMagick to verify fixture pixels.
