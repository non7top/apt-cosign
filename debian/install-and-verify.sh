#!/bin/sh
# Installs the .deb bind-mounted at /dist (see docker-compose.matrix.yml)
# and runs verify-install.sh. Installing at container-start rather than
# baking the .deb in at image-build time means the image never needs
# rebuilding just because dist/*.deb changed, and avoids a real bug we hit
# testing this under `act`: its checkout step snapshots the repo once via
# `docker cp`, so a later `docker build`'s context upload (read from that
# act-runner-local snapshot) never sees a dist/ directory a *sibling*
# bind-mounted container created on the host afterward. A bind mount at
# container-start time, same as the rest of this project's dev container,
# doesn't have that staleness problem.
set -eu

DEB=$(find /dist -maxdepth 1 -name '*.deb' | head -1)
if [ -z "$DEB" ]; then
  echo "FAIL: no .deb found under /dist (mount ./dist:/dist:ro)" >&2
  exit 1
fi

apt-get update
apt-get install -y "$DEB"

exec /usr/local/bin/verify-install.sh
