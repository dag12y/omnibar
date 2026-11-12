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
`libpulse` (provides `pactl`), and `pipewire-pulse`. The audio module uses the
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

`SKIP` is only there so `make package` can substitute a tarball of the working
tree.

## Publishing to OPR

Committing a PKGBUILD does not publish anything. Omarchy Package Repository
(OPR) publication is a separate step: contribute a package recipe to the OPR and
have its maintainers accept it. That is outside this repository.
