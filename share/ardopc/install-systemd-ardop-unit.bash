#!/usr/bin/env bash
set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

[ -e "/etc/systemd/system/ardop@.service" ] && rm /etc/systemd/system/ardop@.service
cp "$DIR/ardop@.service" "/lib/systemd/system/ardop@.service"
systemctl daemon-reload

echo "Installed. Install (pi)ardopc as /usr/local/bin/ardopc and start it with 'systemctl start ardop@username'"
