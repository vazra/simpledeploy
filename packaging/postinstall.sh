#!/bin/sh
set -e
mkdir -p /etc/simpledeploy
mkdir -p /var/lib/simpledeploy
systemctl daemon-reload
echo "SimpleDeploy installed. Run 'simpledeploy init' to generate config."
