#!/bin/sh
# Smoke-tests an installed apt-cosign .deb: binaries present and executable,
# postinst's cache directory created with correct ownership, and the method
# binary actually runs (emits a real 100 Capabilities handshake) on this
# specific Ubuntu release/libc -- not just that dpkg accepted the metadata.
set -eu

fail() { echo "FAIL: $*" >&2; exit 1; }

METHOD_BIN=/usr/lib/apt/methods/sigstore+https
SIGN_BIN=/usr/bin/apt-cosign-sign
CACHE_DIR=/var/lib/apt-cosign

[ -x "$METHOD_BIN" ] || fail "$METHOD_BIN missing or not executable"
[ -x "$SIGN_BIN" ] || fail "$SIGN_BIN missing or not executable"

getent passwd _apt >/dev/null || fail "_apt user missing"

[ -d "$CACHE_DIR" ] || fail "$CACHE_DIR was not created by postinst"
owner=$(stat -c '%U' "$CACHE_DIR")
[ "$owner" = "_apt" ] || fail "$CACHE_DIR owned by $owner, want _apt"

out=$(printf '' | "$METHOD_BIN" || true)
case "$out" in
  "100 Capabilities"*) ;;
  *) fail "unexpected capabilities output: $out" ;;
esac

# Verify that the method drops privileges to _apt when given 601 Configuration as root
out=$(printf '601 Configuration\nConfig-Item: Acquire::sigstore::Debug=true\n\n' | "$METHOD_BIN" || true)
case "$out" in
  "100 Capabilities"*) ;;
  *) fail "unexpected capabilities output with configuration: $out" ;;
esac

log_file="$CACHE_DIR/debug.log"
[ -f "$log_file" ] || fail "debug log was not created at $log_file"
log_owner=$(stat -c '%U' "$log_file")
[ "$log_owner" = "_apt" ] || fail "debug log owned by $log_owner, want _apt"

# shellcheck disable=SC1091 # /etc/os-release only exists on the target system, not at lint time
echo "OK: $(. /etc/os-release && echo "$PRETTY_NAME")"
