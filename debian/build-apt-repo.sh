#!/bin/sh
# Stages a flat-format apt repo (no dists/ hierarchy: `deb URL/ ./`) from
# the built .deb, for hosting on raw.githubusercontent.com. Every file here
# -- InRelease, Packages, Packages.gz, and the .deb itself -- gets
# independently verified by apt-cosign-method (it has no special case for
# "this one is the trust anchor"), so each needs its own apt-cosign-sign
# bundle. This script only stages the unsigned InRelease/Packages/
# Packages.gz; signing those is a separate step so staging can run
# without OIDC/network access. If dist/
# already has a signed .deb.sigstore (release.yml signs the .deb once,
# before staging), it's copied along with the .deb rather than left for
# sign-apt-repo.sh to sign a second time -- same bytes, no need for a
# second Rekor entry.
set -eu

cd "$(dirname "$0")/.."

# Named explicitly by the current version rather than picked via a dist/
# glob: dist/ isn't cleaned between local builds, so a glob can silently
# pick a stale .deb left over from a previous version instead of the one
# that actually matches HEAD. .release-please-manifest.json is the same
# single source of truth debian/build.sh uses.
VERSION=$(sed -n 's/.*"\.": *"\([^"]*\)".*/\1/p' .release-please-manifest.json)
# dpkg --print-architecture, not `go env GOARCH`: this script runs in the
# `repo` service (dpkg-dev, no Go toolchain), not `dev`. Debian and Go
# architecture names coincide for amd64/arm64, the only ones this project
# packages (see debian/control).
ARCH=$(dpkg --print-architecture)
DEB="dist/apt-cosign_${VERSION}_${ARCH}.deb"
[ -f "$DEB" ] || { echo "build-apt-repo.sh: $DEB missing; run 'make package' first" >&2; exit 1; }

REPO_DIR=apt-repo
rm -rf "$REPO_DIR"
mkdir -p "$REPO_DIR"
cp "$DEB" "$REPO_DIR/"
[ -f "$DEB.sigstore" ] && cp "$DEB.sigstore" "$REPO_DIR/"

cd "$REPO_DIR"

# dpkg-scanpackages needs an override file argument; /dev/null means "no
# overrides". Filename: entries come out relative to "." (e.g.
# "./apt-cosign_0.2.0_amd64.deb"), which apt resolves against the repo's
# own base URI.
dpkg-scanpackages . /dev/null > Packages

# apt tries a fixed list of compressed variants (gz, xz, bz2, lzma, lz4,
# zst) before falling back to the plain file - without this, every
# apt-get update against this repo pays for six guaranteed-404 round
# trips first (confirmed empirically). -k keeps the plain Packages
# around too (both are independently listed below, and sign-apt-repo.sh
# signs both - apt-cosign-method has no special case for "this one
# doesn't need its own bundle").
gzip -9 -k -f Packages

packages_sha256=$(sha256sum Packages | cut -d' ' -f1)
packages_size=$(wc -c < Packages)
packages_gz_sha256=$(sha256sum Packages.gz | cut -d' ' -f1)
packages_gz_size=$(wc -c < Packages.gz)

cat > InRelease <<EOF
Origin: apt-cosign
Label: apt-cosign
Suite: stable
Codename: stable
Architectures: amd64
Components: main
Description: apt-cosign demo repository (sigstore-verified, no GPG signature -- see README)
Date: $(date -u -R)
SHA256:
 ${packages_sha256} ${packages_size} Packages
 ${packages_gz_sha256} ${packages_gz_size} Packages.gz
EOF

echo "staged $REPO_DIR/ ($(basename "$DEB"), Packages, InRelease)"
