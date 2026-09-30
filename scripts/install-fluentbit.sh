#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

if command -v fluent-bit >/dev/null 2>&1; then
    exit 0
fi

apt-get update
apt-get install -y ca-certificates curl
curl --fail --silent --show-error https://raw.githubusercontent.com/fluent/fluent-bit/master/install.sh | bash
apt-get install -y fluent-bit