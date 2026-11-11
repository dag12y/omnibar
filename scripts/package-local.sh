#!/usr/bin/env bash
# Build the Arch package from the current working tree (tracked and untracked,
# but not ignored, files), without needing a published release tarball.
#
# The package is written to dist/. Nothing is installed; install it yourself
# with `sudo pacman -U dist/omnibar-<version>-<rel>-<arch>.pkg.tar.zst`.
#
# Usage: scripts/package-local.sh [extra makepkg flags, e.g. --nocheck]

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

pkgver="$(sed -n "s/^pkgver=//p" pkgbuild/PKGBUILD)"
pkgname="$(sed -n "s/^pkgname=//p" pkgbuild/PKGBUILD)"
[[ -n "$pkgver" && -n "$pkgname" ]] || {
    echo "package-local: could not read pkgname/pkgver from pkgbuild/PKGBUILD" >&2
    exit 1
}

work="$(mktemp -d "${TMPDIR:-/tmp}/omnibar-pkg-XXXXXX")"
trap 'rm -rf "$work"' EXIT

# Stage the tree under the directory name the PKGBUILD expects.
stage="$work/$pkgname-$pkgver"
mkdir -p "$stage"
while IFS= read -r -d '' file; do
    [[ -e "$file" ]] || continue # tracked but deleted in the working tree
    mkdir -p "$stage/$(dirname "$file")"
    cp -p "$file" "$stage/$file"
done < <(git ls-files -z --cached --others --exclude-standard)

tar -C "$work" -czf "$work/$pkgname-$pkgver.tar.gz" "$pkgname-$pkgver"
rm -rf "$stage"
cp pkgbuild/PKGBUILD "$work/PKGBUILD"

mkdir -p dist
(
    cd "$work"
    PKGDEST="$root/dist" makepkg --force --cleanbuild "$@"
)

echo "package-local: built packages in $root/dist:"
ls -1 "$root"/dist/*.pkg.tar.zst
