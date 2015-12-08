#!/usr/bin/env bash
set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cp "$DIR/ax25.service" "/etc/systemd/system/ax25.service"
cp "$DIR/ax25.default" "/etc/default/ax25"
systemctl daemon-reload

echo "Installed. Edit /etc/default/ax25 and start with 'systemctl start ax25'"
