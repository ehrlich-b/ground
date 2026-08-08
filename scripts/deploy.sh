#!/bin/bash
set -euo pipefail

# Deploy ground to ground.ehrlich.dev
# Usage: ./scripts/deploy.sh
#
# First deploy: create /root/.ground/env on the server with:
#   GROUND_JWT_SECRET=your-secret
#   OPENAI_API_KEY=sk-...

HOST="root@104.131.94.68"
REPO="$(cd "$(dirname "$0")/.." && pwd)"

echo "=== building linux/amd64 ==="
GOOS=linux GOARCH=amd64 go build -o /tmp/ground-linux "$REPO/cmd/ground"

echo "=== uploading binary ==="
scp /tmp/ground-linux "$HOST:/opt/ground-bin.new"

echo "=== deploying on server ==="
ssh "$HOST" bash -s <<'REMOTE'
set -euo pipefail

chmod +x /opt/ground-bin.new
mkdir -p /root/.ground

# Check env file exists
if [ ! -f /root/.ground/env ]; then
    echo "ERROR: /root/.ground/env not found"
    echo "Create it with GROUND_JWT_SECRET and OPENAI_API_KEY"
    exit 1
fi

# Stop service before swapping
systemctl stop ground 2>/dev/null || true

# Swap binary. Plumbing (systemd unit, nginx vhost) is owned by ~/repos/infra.
mv /opt/ground-bin.new /opt/ground-bin

systemctl restart ground
sleep 1
systemctl is-active ground
echo "=== deployed ==="
REMOTE

echo ""
echo "done. site: https://ground.ehrlich.dev"
