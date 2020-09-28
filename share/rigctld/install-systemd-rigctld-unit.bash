#!/usr/bin/env bash
set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

[ -e "/etc/systemd/system/rigctld.service" ] && rm /etc/systemd/system/rigctld.service
cp "$DIR/rigctld.service" "/lib/systemd/system/rigctld.service"
[ -e "/etc/default/rigctld" ] || cp "$DIR/rigctld.default" "/etc/default/rigctld"
systemctl daemon-reload

echo "Installed. Edit /etc/default/rigctld and start with 'systemctl start rigctld'"
