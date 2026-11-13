#!/usr/bin/env python3
"""Verify install/reinstall/downgrade/removal in a private pacman root.

Usage: python3 scripts/verify-package-lifecycle.py OLDER_PACKAGE NEWER_PACKAGE
Requires pacman and fakeroot. Dependencies are intentionally skipped: this
checks package transactions and user-data preservation, not a bootable guest.
"""
import hashlib
from pathlib import Path
import subprocess
import sys
import tempfile


def run(*args):
    return subprocess.check_output(args, text=True)


def main():
    if len(sys.argv) != 3:
        raise SystemExit(__doc__)
    older, newer = [Path(arg).resolve(strict=True) for arg in sys.argv[1:]]
    for package in (older, newer):
        # Hooks and scriptlets aren't needed by Omnibar and would require a
        # bootable root. Reject them rather than silently skipping behavior.
        names = run("bsdtar", "-tf", str(package)).splitlines()
        assert ".INSTALL" not in names, "unexpected install scriptlet"
        assert not any("libalpm/hooks/" in name for name in names), "unexpected hook"
    with tempfile.TemporaryDirectory(prefix="omnibar-pacman-") as temp:
        base = Path(temp)
        root = base / "root"
        db = root / "var/lib/pacman"
        db.mkdir(parents=True)
        hooks = base / "hooks"
        hooks.mkdir()
        cache = base / "cache"
        cache.mkdir()
        config = base / "pacman.conf"
        config.write_text("[options]\nArchitecture = auto\nSigLevel = Never\n")
        args = ["fakeroot", "pacman", "--root", str(root), "--dbpath", str(db),
                "--config", str(config), "--hookdir", str(hooks),
                "--cachedir", str(cache),
                "--logfile", str(base / "pacman.log"), "--noconfirm"]

        def pacman(*operation):
            return run(*args, *operation)

        def digest(path):
            return hashlib.sha256(path.read_bytes()).hexdigest()

        home = root / "home/tester"
        sentinels = {}
        for relative, contents in {
            ".config/omnibar/config.toml": 'config_version = 1\n',
            ".local/share/omnibar/clipboard-favorites.json": '[]\n',
            ".local/share/omnibar/window-layouts.json": '{"version":1,"layouts":[]}\n',
            ".local/state/omnibar/sentinel": 'preserve state\n',
        }.items():
            path = home / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(contents)
            path.chmod(0o600)
            sentinels[path] = (digest(path), path.stat().st_mode)

        def check_data():
            assert all(path.exists() and (digest(path), path.stat().st_mode) == value
                       for path, value in sentinels.items()), "user data changed"

        def install(package):
            pacman("-Udd", str(package))
            check_data()
            assert (root / "usr/bin/omnibar").is_file()
            payload = {}
            for item in pacman("-Qlq", "omnibar").splitlines():
                path = Path(item)
                assert path.is_relative_to(root), f"unexpected installed path: {item}"
                if path.is_symlink():
                    payload[str(path.relative_to(root))] = (path.lstat().st_mode, str(path.readlink()))
                elif path.is_file():
                    payload[str(path.relative_to(root))] = (path.stat().st_mode, digest(path))
            return pacman("-Q", "omnibar").strip(), payload

        old_version, old_payload = install(older)
        print("PASS: isolated baseline install and user data")
        new_version, new_payload = install(newer)
        assert run("vercmp", new_version.split()[1], old_version.split()[1]).strip() == "1"
        assert new_payload != old_payload, "rollback fixture must contain different package contents"
        print("PASS: upgrade replaces package contents and preserves data")
        assert install(newer) == (new_version, new_payload)
        print("PASS: same-version reinstall")
        assert install(older) == (old_version, old_payload)
        for relative in new_payload.keys() - old_payload.keys():
            path = root / relative
            assert not path.exists() and not path.is_symlink(), f"new file survived downgrade: {relative}"
        print("PASS: downgrade restores exact previous files/modes, removes added files, and preserves data")
        install(newer)
        shipped = pacman("-Qlq", "omnibar").splitlines()
        pacman("-R", "omnibar")
        check_data()
        assert not (root / "usr/bin/omnibar").exists()
        # pacman -Ql reports paths prefixed with the configured root.
        for item in shipped:
            path = Path(item)
            assert path.is_relative_to(root), f"unexpected installed path: {item}"
            assert not path.is_file() and not path.is_symlink(), f"package file survived removal: {item}"
        assert subprocess.run(args + ["-Q", "omnibar"], capture_output=True).returncode != 0
        print("PASS: uninstall removes package files/database entry and preserves config/data/state")
        install(newer)
        print("PASS: clean reinstall after removal")


if __name__ == "__main__":
    main()
