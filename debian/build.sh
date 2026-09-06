#!/bin/sh
# Stages a binary .deb from the already-built ./bin binaries and
# debian/{control,changelog,postinst,postrm}.
#
# This does NOT use dpkg-buildpackage/debhelper: sigstore-go needs Go
# >=1.25.8, newer than the golang-go package on several of the Ubuntu
# releases in our support matrix (jammy/22.04, noble/24.04), so the binary
# is built once with the pinned toolchain in Dockerfile/docker-compose.yml
# and packaged directly. debian/control is still real metadata (its
# Description is copied below), not just documentation.
set -eu

cd "$(dirname "$0")/.."

PKG_NAME=apt-cosign
# .release-please-manifest.json is the single source of truth for the
# version -- release-please updates it, and nothing else does, so nothing
# else should be read for VERSION here (debian/changelog's own version line
# was a second, independently-tracked copy that drifted out of sync the
# first time a release actually shipped).
VERSION=$(sed -n 's/.*"\.": *"\([^"]*\)".*/\1/p' .release-please-manifest.json)
ARCH=$(go env GOARCH)

for bin in "bin/sigstore+https" "bin/apt-cosign-sign"; do
  test -x "$bin" || { echo "build.sh: $bin missing; run 'make build' first" >&2; exit 1; }
done

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
chmod 0755 "$STAGE"

install -d "$STAGE/DEBIAN"
install -d "$STAGE/usr/lib/apt/methods"
install -d "$STAGE/usr/bin"
install -d "$STAGE/etc/apt/apt.conf.d"

install -m 0755 "bin/sigstore+https" "$STAGE/usr/lib/apt/methods/sigstore+https"
install -m 0755 "bin/apt-cosign-sign" "$STAGE/usr/bin/apt-cosign-sign"
install -m 0644 "debian/99sigstore-policy.example" "$STAGE/etc/apt/apt.conf.d/99sigstore-policy.example"
install -m 0755 "debian/postinst" "$STAGE/DEBIAN/postinst"
install -m 0755 "debian/postrm" "$STAGE/DEBIAN/postrm"

# Written out directly rather than parsed from debian/control: dpkg-deb's
# DEBIAN/control has no stanza inheritance (that's a debhelper feature), so
# the fields that live on the Source stanza there (Section, Priority,
# Maintainer) have to be repeated here anyway. debian/control stays the
# authoritative human-readable reference for what's shipped; keep the two in
# sync by hand if either changes.
cat > "$STAGE/DEBIAN/control" <<EOF
Package: $PKG_NAME
Version: $VERSION
Section: admin
Priority: optional
Architecture: $ARCH
Maintainer: Vladimir Berezhnoy <non7top@gmail.com>
Depends: apt (>= 1.1)
Description: Sigstore-verifying apt transport method
 apt-cosign installs /usr/lib/apt/methods/sigstore+https, a native apt
 acquire method that fetches repository metadata and packages, verifies an
 accompanying Sigstore bundle (Rekor transparency log entry and a
 Fulcio-issued OIDC certificate identity) against a configured policy, and
 only then hands the file back to apt.
 .
 apt-cosign-sign, also included, is the repo-maintainer counterpart: it
 keylessly signs a file with Sigstore and writes the resulting bundle next
 to it as FILE.sigstore, the sidecar apt-cosign-method fetches and verifies.
EOF

mkdir -p dist
OUT="dist/${PKG_NAME}_${VERSION}_${ARCH}.deb"
dpkg-deb --build --root-owner-group "$STAGE" "$OUT"
echo "built $OUT"
