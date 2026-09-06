#!/bin/sh
# Stages a flat-format apt repo (no dists/ hierarchy: `deb URL/ ./`) from
# the built .deb, for hosting on raw.githubusercontent.com. Every file here
# -- InRelease, Packages, and the .deb itself -- gets independently
# verified by apt-cosign-method (it has no special case for "this one is
# the trust anchor"), so each needs its own apt-cosign-sign bundle. This
# script only stages the unsigned files; signing is a separate step so
# staging can run without OIDC/network access.
set -eu

cd "$(dirname "$0")/.."

DEB=$(find dist -maxdepth 1 -name '*.deb' | head -1)
[ -n "$DEB" ] || { echo "build-apt-repo.sh: no .deb in dist/; run 'make package' first" >&2; exit 1; }

REPO_DIR=apt-repo
rm -rf "$REPO_DIR"
mkdir -p "$REPO_DIR"
cp "$DEB" "$REPO_DIR/"

cd "$REPO_DIR"

# dpkg-scanpackages needs an override file argument; /dev/null means "no
# overrides". Filename: entries come out relative to "." (e.g.
# "./apt-cosign_0.2.0_amd64.deb"), which apt resolves against the repo's
# own base URI.
dpkg-scanpackages . /dev/null > Packages

packages_sha256=$(sha256sum Packages | cut -d' ' -f1)
packages_size=$(wc -c < Packages)

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
EOF

echo "staged $REPO_DIR/ ($(basename "$DEB"), Packages, InRelease)"
