#!/usr/bin/env bash
set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

[ -e "/etc/systemd/system/ax25.service" ] && rm /etc/systemd/system/ax25.service
cp "$DIR/ax25.service" "/lib/systemd/system/ax25.service"
[ -e "/etc/default/ax25" ] || cp "$DIR/ax25.default" "/etc/default/ax25"
systemctl daemon-reload

echo "Installed. Edit /etc/default/ax25 and start with 'systemctl start ax25'"
