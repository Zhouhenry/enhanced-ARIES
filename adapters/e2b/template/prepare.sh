#!/bin/sh
set -eu

# Runs inside the E2B template build, never on the controller host.
test "$(id -u)" = 0
test -f /app/openclaw.mjs
test -x /usr/local/bin/node
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends python3 python3-pytest git passwd
rm -rf /var/lib/apt/lists/*

if getent passwd 60000 >/dev/null || getent passwd aries-agent >/dev/null; then
    test "$(getent passwd aries-agent | cut -d: -f3)" = 60000
    test "$(getent passwd aries-agent | cut -d: -f4)" = 60000
else
    if getent group 60000 >/dev/null || getent group aries-agent >/dev/null; then
        test "$(getent group aries-agent | cut -d: -f3)" = 60000
    else
        groupadd --gid 60000 aries-agent
    fi
    useradd --uid 60000 --gid 60000 --create-home --home-dir /home/aries-agent --shell /bin/bash aries-agent
fi

# Preserve the published image's Node and OpenClaw build; no npm reinstall.
chown -hR root:root /app
# The pinned image retains CLI links for packages pruned from its runtime tree.
find /app/node_modules/.bin -maxdepth 1 -xtype l -printf '%p -> %l\n' -delete \
    > /usr/local/share/aries-template/pruned-cli-links.txt
chmod 0444 /usr/local/share/aries-template/pruned-cli-links.txt
chmod -R a-w /app
chmod 0555 /app/openclaw.mjs
ln -sfn /app/openclaw.mjs /usr/local/bin/openclaw
chown -h root:root /usr/local/bin/openclaw
chown root:root /usr/local/bin/node /usr/local /usr/local/bin
chmod go-w /usr/local/bin/node /usr/local /usr/local/bin
install -d -m 0755 -o root -g root /workspace /workspace/aries-case /usr/local/share/aries-template
chown root:root /usr/local/share/aries-template/check_runtime.py
chmod 0555 /usr/local/share/aries-template/check_runtime.py
python3 -I /usr/local/share/aries-template/check_runtime.py --expected-version "$1" > /usr/local/share/aries-template/image-evidence.json || {
    ARIES_PROBE_STATUS=$?
    cat /usr/local/share/aries-template/image-evidence.json >&2
    exit "$ARIES_PROBE_STATUS"
}
chmod 0444 /usr/local/share/aries-template/image-evidence.json

# The provider's finalize step rewrites /usr/local; keep its repair outside it.
test ! -L /opt
test ! -L /opt/aries-template
chown root:root /opt /opt/aries-template /opt/aries-template/start.sh
chmod go-w /opt
chmod 0755 /opt/aries-template
chmod 0500 /opt/aries-template/start.sh
# Ready runs concurrently with start, so remove inherited markers before finalize.
rm -f /opt/aries-template/ready.json /opt/aries-template/ready.json.tmp

# cgroup mounts and kernel capabilities belong to E2B, not this filesystem.
# The separate verify command probes them in a newly booted sandbox.
