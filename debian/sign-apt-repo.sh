#!/bin/sh
# Signs every file in the staged apt-repo/ that apt-cosign-method will
# independently verify -- InRelease, Packages, Packages.gz (build-apt-repo.sh's
# compressed index -- see its own comment on why it exists), and the .deb
# itself; it has no special case for "this one is the trust anchor", so
# each needs its own apt-cosign-sign bundle. Run after debian/build-apt-repo.sh.
#
# The .deb is skipped here if it already has a .sigstore sidecar --
# build-apt-repo.sh copies one along from dist/ when release.yml has
# already signed the .deb once for the GitHub release. Signing it again
# here would produce a second, different Sigstore bundle (a fresh Rekor
# entry) for the exact same bytes, for no benefit.
#
# Needs a real OIDC identity: GitHub Actions' ambient token (in a workflow
# with `permissions: id-token: write`), -id-token, or an interactive/device
# login -- see apt-cosign-sign -h. Runs natively (it's a static binary),
# not inside a container: a container wouldn't have a browser for the
# interactive flow, and GitHub Actions' ambient-token env vars aren't
# forwarded into docker compose run's containers.
set -eu

cd "$(dirname "$0")/.."

SIGN_BIN=./bin/apt-cosign-sign
[ -x "$SIGN_BIN" ] || { echo "sign-apt-repo.sh: $SIGN_BIN missing; run 'make build' first" >&2; exit 1; }

REPO_DIR=apt-repo
[ -d "$REPO_DIR" ] || { echo "sign-apt-repo.sh: $REPO_DIR missing; run 'make apt-repo-stage' first" >&2; exit 1; }

for f in "$REPO_DIR/InRelease" "$REPO_DIR/Packages" "$REPO_DIR/Packages.gz" "$REPO_DIR"/*.deb; do
  if [ -f "$f.sigstore" ]; then
    echo "already signed, skipping: $f"
    continue
  fi
  echo "signing $f"
  "$SIGN_BIN" "$@" "$f"
done
