#!/bin/sh
# Create the krakenkey-probe system user and group before files are
# unpacked, so /etc/krakenkey/probe.yaml gets the right group.
set -e

USER_NAME=krakenkey-probe
GROUP_NAME=krakenkey-probe
HOME_DIR=/var/lib/krakenkey-probe

if [ -x /usr/sbin/nologin ]; then
	NOLOGIN=/usr/sbin/nologin
elif [ -x /sbin/nologin ]; then
	NOLOGIN=/sbin/nologin
else
	NOLOGIN=/bin/false
fi

if ! getent group "$GROUP_NAME" >/dev/null 2>&1; then
	groupadd --system "$GROUP_NAME"
fi

if ! getent passwd "$USER_NAME" >/dev/null 2>&1; then
	useradd --system --gid "$GROUP_NAME" --home-dir "$HOME_DIR" \
		--no-create-home --shell "$NOLOGIN" \
		--comment "KrakenKey probe" "$USER_NAME"
fi

exit 0
