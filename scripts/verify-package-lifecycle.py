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
            ".local/state/omnibar/sentinel": 'preserve state\n',
        }.items():
            path = home / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(contents)
            sentinels[path] = digest(path)

        def check_data():
            assert all(path.exists() and digest(path) == value
                       for path, value in sentinels.items()), "user data changed"

        def install(package):
            pacman("-Udd", str(package))
            check_data()
            assert (root / "usr/bin/omnibar").is_file()
            return pacman("-Q", "omnibar").strip(), digest(root / "usr/bin/omnibar")

        old_version, old_binary = install(older)
        print("PASS: isolated baseline install and user data")
        new_version, new_binary = install(newer)
        assert run("vercmp", new_version.split()[1], old_version.split()[1]).strip() == "1"
        assert new_binary != old_binary, "rollback fixture must contain different code"
        print("PASS: upgrade replaces backend and preserves data")
        assert install(newer) == (new_version, new_binary)
        print("PASS: same-version reinstall")
        assert install(older) == (old_version, old_binary)
        print("PASS: downgrade restores exact previous backend and preserves data")
        install(newer)
        shipped = pacman("-Qlq", "omnibar").splitlines()
        pacman("-R", "omnibar")
        check_data()
        assert not (root / "usr/bin/omnibar").exists()
        # pacman -Ql reports paths prefixed with the configured root.
        for item in shipped:
            path = Path(item)
            assert path.is_relative_to(root), f"unexpected installed path: {item}"
            assert not path.is_file(), f"package file survived removal: {item}"
        assert subprocess.run(args + ["-Q", "omnibar"], capture_output=True).returncode != 0
        print("PASS: uninstall removes package files/database entry and preserves config/data/state")
        install(newer)
        print("PASS: clean reinstall after removal")


if __name__ == "__main__":
    main()
