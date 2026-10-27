#!/bin/sh
set -eu

# E2B runs this after provider configuration and before taking the template snapshot.
test "$(/usr/bin/id -u)" = 0
ARIES_READY=/opt/aries-template/ready.json
ARIES_PENDING=/opt/aries-template/ready.json.tmp
/bin/rm -f "$ARIES_READY" "$ARIES_PENDING"
trap '/bin/rm -f "$ARIES_PENDING"' 0

# Provider finalization makes /usr/local writable; retain the native probe's policy.
/usr/bin/chmod -R go-w /usr/local
/usr/bin/python3 -I /usr/local/share/aries-template/check_runtime.py --expected-version "$1" > "$ARIES_PENDING" || {
    ARIES_PROBE_STATUS=$?
    /bin/cat "$ARIES_PENDING" >&2
    exit "$ARIES_PROBE_STATUS"
}
/usr/bin/chmod 0444 "$ARIES_PENDING"
/bin/mv -f "$ARIES_PENDING" "$ARIES_READY"
exec /bin/sleep infinity
