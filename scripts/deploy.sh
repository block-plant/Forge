#!/usr/bin/env bash
# ═══════════════════════════════════════════════════
# Forge — Full First-Time Deployment
# Sets up Forge on a fresh server from scratch.
#
# Usage: ./scripts/deploy.sh <SERVER_IP>
# ═══════════════════════════════════════════════════

set -e
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"

if [ -z "$1" ]; then
  echo ""
  echo "  ⚡ Forge — First-Time Deploy"
  echo "  ─────────────────────────────"
  echo "  Usage: ./scripts/deploy.sh <SERVER_IP>"
  echo "  Example: ./scripts/deploy.sh 129.153.10.50"
  echo ""
  exit 1
fi

SERVER_IP=$1
SERVER_USER="${FORGE_SERVER_USER:-ubuntu}"

echo ""
echo "  ⚡ Forge — First-Time Deploy → $SERVER_IP"
echo "  ─────────────────────────────"
echo ""

# Step 1: Build (dashboard embedded via go:embed)
echo "  [1/4] Building binary (linux/amd64)..."
GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o forge-linux .
echo "        ✓ Built $(du -h forge-linux | cut -f1)"

# Step 2: Upload binary + config files
echo "  [2/4] Uploading files..."
scp -o StrictHostKeyChecking=no \
  forge-linux \
  forge.prod.json \
  forge.rules \
  scripts/forge.service \
  $SERVER_USER@$SERVER_IP:~/

# Step 3: Install on server
echo "  [3/4] Installing Forge on server..."
ssh -o StrictHostKeyChecking=no $SERVER_USER@$SERVER_IP << 'REMOTE'
  # Setup directories
  sudo mkdir -p /opt/forge
  sudo mkdir -p /var/lib/forge-data

  # Install binary
  sudo mv ~/forge-linux /opt/forge/forge
  sudo chmod +x /opt/forge/forge

  # Install config
  sudo mv ~/forge.prod.json /opt/forge/forge.json
  sudo mv ~/forge.rules /opt/forge/
  sudo chown -R ubuntu:ubuntu /opt/forge /var/lib/forge-data

  # Install systemd service
  sudo mv ~/forge.service /etc/systemd/system/forge.service
  sudo systemctl daemon-reload
  sudo systemctl enable forge
REMOTE

# Step 4: Start
echo "  [4/4] Starting Forge..."
ssh -o StrictHostKeyChecking=no $SERVER_USER@$SERVER_IP << 'REMOTE'
  sudo systemctl restart forge
  sleep 2
  sudo systemctl status forge --no-pager -l | head -10
REMOTE

# Cleanup
rm -f forge-linux

echo ""
echo "  ✅ Forge is live!"
echo "  Dashboard: http://$SERVER_IP:8080/dashboard"
echo ""
