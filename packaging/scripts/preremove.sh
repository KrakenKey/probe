#!/bin/sh
# Stop and disable the service when the package is removed (not on upgrade).
set -e

# deb: $1=remove or purge on removal, upgrade on upgrade
# rpm: $1=0 on removal, 1 or more on upgrade
case "$1" in
	remove|purge|0) ;;
	*) exit 0 ;;
esac

if command -v systemctl >/dev/null 2>&1; then
	if [ -d /run/systemd/system ]; then
		systemctl stop krakenkey-probe.service || true
	fi
	systemctl disable krakenkey-probe.service >/dev/null 2>&1 || true
fi

exit 0
