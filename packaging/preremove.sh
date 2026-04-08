#!/bin/sh
set -e
systemctl stop simpledeploy 2>/dev/null || true
systemctl disable simpledeploy 2>/dev/null || true
