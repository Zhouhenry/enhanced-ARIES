#!/bin/sh
set -eu
# E2B finalize changes /usr/local permissions. Repair before snapshot readiness.
test "$(id -u)" = 0
rm -f /opt/aries-template/ready.json /opt/aries-template/ready.json.tmp
chown -hR root:root /usr/local
chmod -R go-w /usr/local
python3 -I /opt/aries-template/verify.py > /opt/aries-template/ready.json.tmp
chmod 0444 /opt/aries-template/ready.json.tmp
mv /opt/aries-template/ready.json.tmp /opt/aries-template/ready.json
exec sleep infinity
