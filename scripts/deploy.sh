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

# Swap binary
mv /opt/ground-bin.new /opt/ground-bin

# Systemd service
cat > /etc/systemd/system/ground.service <<'SVC'
[Unit]
Description=ground.ehrlich.dev
After=network.target

[Service]
Type=simple
ExecStart=/opt/ground-bin serve --port 8081 --db /root/.ground/ground.db
EnvironmentFile=/root/.ground/env
Environment=HOME=/root
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
SVC

systemctl daemon-reload
systemctl enable ground
systemctl restart ground
sleep 1
systemctl is-active ground

# Nginx
cat > /etc/nginx/sites-enabled/ground.ehrlich.dev.conf <<'NGX'
server {
    listen 80;
    listen [::]:80;

    server_name ground.ehrlich.dev;

    location / {
        proxy_pass http://127.0.0.1:8081;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
NGX

nginx -t && systemctl reload nginx
echo "=== deployed ==="
REMOTE

echo ""
echo "done. site: https://ground.ehrlich.dev"
