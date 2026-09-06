#!/bin/sh
# Signs every file in the staged apt-repo/ that apt-cosign-method will
# independently verify -- InRelease, Packages, and the .deb itself; it has
# no special case for "this one is the trust anchor", so each needs its own
# apt-cosign-sign bundle. Run after debian/build-apt-repo.sh.
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

for f in "$REPO_DIR/InRelease" "$REPO_DIR/Packages" "$REPO_DIR"/*.deb; do
  echo "signing $f"
  "$SIGN_BIN" "$@" "$f"
done
