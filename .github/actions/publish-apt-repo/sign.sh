#!/bin/sh
# Signs every file under the staged $REPO_DIR (recursively, for the
# per-codename case) that apt-cosign-method will independently verify --
# InRelease, Packages, Packages.gz (stage.sh's compressed index - see its
# own comment on why it exists), and each .deb. Run after stage.sh.
#
# A file already carrying a .sigstore sidecar (stage.sh copies one along
# when a .deb was already signed once, e.g. for a GitHub release, before
# staging) is skipped: signing it again would produce a second, different
# Sigstore bundle (a fresh Rekor entry) for the exact same bytes, for no
# benefit.
set -eu

: "${REPO_DIR:?REPO_DIR must be set}"
: "${SIGN_BIN:?SIGN_BIN must be set}"
: "${SIGN_ARGS:=}"

[ -x "$SIGN_BIN" ] || { echo "sign.sh: $SIGN_BIN missing or not executable" >&2; exit 1; }
[ -d "$REPO_DIR" ] || { echo "sign.sh: $REPO_DIR missing; run stage.sh first" >&2; exit 1; }

find "$REPO_DIR" -type f \( -name InRelease -o -name Packages -o -name 'Packages.gz' -o -name '*.deb' \) | sort | while read -r f; do
  if [ -f "$f.sigstore" ]; then
    echo "already signed, skipping: $f"
    continue
  fi
  echo "signing $f"
  # shellcheck disable=SC2086 # SIGN_ARGS is an intentionally word-split arg list
  "$SIGN_BIN" $SIGN_ARGS "$f"
done
