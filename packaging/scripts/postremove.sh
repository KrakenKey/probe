#!/bin/sh
# Let systemd forget the removed unit. The krakenkey-probe user, the
# config in /etc/krakenkey and the state in /var/lib/krakenkey-probe are
# kept so a reinstall picks up where it left off.
set -e

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload || true
fi

exit 0
