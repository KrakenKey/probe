#!/bin/sh
# Reload systemd so it sees the unit. The service is not enabled or
# started here: edit /etc/krakenkey/probe.yaml first, then run
#   sudo systemctl enable --now krakenkey-probe
# On upgrade, restart the service only if it is already running.
set -e

# deb: $1=configure, $2=previously configured version (empty on first install)
# rpm: $1=1 on first install, 2 or more on upgrade
upgrade=0
case "$1" in
	configure)
		if [ -n "$2" ]; then upgrade=1; fi
		;;
	*)
		if [ "$1" -ge 2 ] 2>/dev/null; then upgrade=1; fi
		;;
esac

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload || true
	if [ "$upgrade" = 1 ]; then
		systemctl try-restart krakenkey-probe.service || true
	fi
fi

if [ "$upgrade" = 0 ]; then
	echo "krakenkey-probe installed. Edit /etc/krakenkey/probe.yaml, then run:"
	echo "  sudo systemctl enable --now krakenkey-probe"
fi

exit 0
