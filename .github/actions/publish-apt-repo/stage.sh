#!/bin/sh
# Stages a flat-format apt repo (no dists/ hierarchy: `deb URL/ ./`) from
# the .deb(s) in $DIST_DIR, into $REPO_DIR. Every file staged here --
# InRelease, Packages, and the .deb itself -- gets independently verified
# by apt-cosign-method (it has no special case for "this one is the trust
# anchor"), so each needs its own apt-cosign-sign bundle; that's sign.sh's
# job, run after this one.
#
# When $CODENAME_PATTERN is empty, every .deb is staged flat into
# $REPO_DIR itself (one Packages/InRelease for the lot). When set, it's a
# sed -E script applied to each .deb's filename to extract an OS codename,
# and each codename gets its own subdirectory -- builds for different
# codenames aren't byte-identical and can't share one Packages index.
set -eu

: "${DIST_DIR:=dist}"
: "${REPO_DIR:=apt-repo}"
: "${CODENAME_PATTERN:=}"
: "${ORIGIN:?ORIGIN must be set}"
: "${DESCRIPTION:?DESCRIPTION must be set}"

[ -n "$(find "$DIST_DIR" -maxdepth 1 -name '*.deb' -print -quit)" ] || {
  echo "stage.sh: no .deb in $DIST_DIR/" >&2
  exit 1
}

rm -rf "$REPO_DIR"

stage_one() {
  # $1 = leaf directory to stage this .deb's Packages/InRelease into
  # $2 = codename for the InRelease Codename field ("stable" in flat mode)
  dir=$1
  codename=$2
  mkdir -p "$dir"
  ( cd "$dir"
    # dpkg-scanpackages needs an override file argument; /dev/null means
    # "no overrides". Filename entries come out relative to "." (e.g.
    # "./pkg_1.0_amd64.deb"), which apt resolves against the repo's own
    # base URI.
    dpkg-scanpackages . /dev/null > Packages

    # apt tries a fixed list of compressed variants (gz, xz, bz2, lzma,
    # lz4, zst) before falling back to the plain file - without this, every
    # apt-get update against this repo pays for six guaranteed-404 round
    # trips first (confirmed empirically). -k keeps the plain Packages
    # around too (both are independently listed below, and sign.sh signs
    # both - apt-cosign-method has no special case for "this one doesn't
    # need its own bundle").
    gzip -9 -k -f Packages

    packages_sha256=$(sha256sum Packages | cut -d' ' -f1)
    packages_size=$(wc -c < Packages)
    packages_gz_sha256=$(sha256sum Packages.gz | cut -d' ' -f1)
    packages_gz_size=$(wc -c < Packages.gz)
    description=$(echo "$DESCRIPTION" | sed "s/%CODENAME%/$codename/g")

    cat > InRelease <<EOF
Origin: ${ORIGIN}
Label: ${ORIGIN}
Suite: stable
Codename: ${codename}
Architectures: amd64
Components: main
Description: ${description}
Date: $(date -u -R)
SHA256:
 ${packages_sha256} ${packages_size} Packages
 ${packages_gz_sha256} ${packages_gz_size} Packages.gz
EOF
  )
  echo "staged $dir/ ($(find "$dir" -maxdepth 1 -name '*.deb' | wc -l) .deb, Packages, InRelease)"
}

if [ -z "$CODENAME_PATTERN" ]; then
  mkdir -p "$REPO_DIR"
  for deb in "$DIST_DIR"/*.deb; do
    cp "$deb" "$REPO_DIR/"
    [ -f "$deb.sigstore" ] && cp "$deb.sigstore" "$REPO_DIR/"
  done
  stage_one "$REPO_DIR" stable
else
  for deb in "$DIST_DIR"/*.deb; do
    codename=$(basename "$deb" | sed -E "$CODENAME_PATTERN")
    dir="$REPO_DIR/$codename"
    mkdir -p "$dir"
    cp "$deb" "$dir/"
    [ -f "$deb.sigstore" ] && cp "$deb.sigstore" "$dir/"
  done
  for dir in "$REPO_DIR"/*/; do
    stage_one "${dir%/}" "$(basename "$dir")"
  done
fi
