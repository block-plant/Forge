#!/usr/bin/env bash
# Forge — One-Command Deploy
# Usage: ./deploy.sh <SERVER_IP>
# Example: ./deploy.sh 129.153.10.50

set -e
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"

if [ -z "$1" ]; then
  echo ""
  echo "  ⚡ Forge Deploy"
  echo "  ─────────────────────────────"
  echo "  Usage: ./deploy.sh <SERVER_IP>"
  echo "  Example: ./deploy.sh 129.153.10.50"
  echo ""
  exit 1
fi

SERVER_IP=$1
SERVER_USER="ubuntu"

echo ""
echo "  ⚡ Forge Deploy → $SERVER_IP"
echo "  ─────────────────────────────"
echo ""

# Step 1: Build for Linux (dashboard is embedded via go:embed)
echo "  [1/3] Building binary (linux/amd64)..."
GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o forge-linux .
echo "        ✓ Built forge-linux ($(du -h forge-linux | cut -f1))"

# Step 2: Upload binary
echo "  [2/3] Uploading to $SERVER_USER@$SERVER_IP..."
scp forge-linux $SERVER_USER@$SERVER_IP:~/forge-new

# Step 3: Swap binary and restart
echo "  [3/3] Restarting service..."
ssh $SERVER_USER@$SERVER_IP << 'EOF'
  sudo systemctl stop forge 2>/dev/null || true
  sudo mv ~/forge-new /opt/forge/forge
  sudo chmod +x /opt/forge/forge
  sudo systemctl start forge
  echo "        Service status:"
  sudo systemctl status forge --no-pager -l | head -8
EOF

# Cleanup local build artifact
rm -f forge-linux

echo ""
echo "  ✅ Live at http://$SERVER_IP:8080/dashboard"
echo ""
