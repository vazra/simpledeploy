#!/bin/sh
set -e

# Lock down config and state directories. SimpleDeploy still runs as root
# (docker.sock + low ports), so root retains read/write; locking down to
# 0700 prevents other local users from reading the embedded master_secret
# in /etc/simpledeploy/config.yaml or per-app .env files in
# /var/lib/simpledeploy/apps/*/.env.
mkdir -p /etc/simpledeploy
chmod 0700 /etc/simpledeploy
mkdir -p /var/lib/simpledeploy
chmod 0700 /var/lib/simpledeploy

# Tighten any pre-existing config file (idempotent on upgrade).
if [ -f /etc/simpledeploy/config.yaml ]; then
    chmod 0600 /etc/simpledeploy/config.yaml
fi

systemctl daemon-reload
echo "SimpleDeploy installed. Run 'simpledeploy init' to generate config."
