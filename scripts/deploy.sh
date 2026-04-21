#!/usr/bin/env bash
# Forge deployment script for remote servers (e.g., Oracle Free Tier or DigitalOcean)

set -e

if [ -z "$1" ]; then
  echo "❌ Usage: ./scripts/deploy.sh <SERVER_IP>"
  echo "Example: ./scripts/deploy.sh 192.168.1.100"
  exit 1
fi

# Configuration
SERVER_IP=$1
SERVER_USER="ubuntu"  # Change to 'root' if using DigitalOcean
CONFIG_FILE="forge.prod.json"
RULES_FILE="forge.rules"

echo "🔥 Building Forge for Linux ARM64..."
# NOTE: If using Intel/AMD CPUs (like standard DigitalOcean droplet), change GOARCH to amd64
GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o forge ./main.go

echo "🔥 Deploying to $SERVER_USER@$SERVER_IP..."

# Set up required directories on the server
ssh $SERVER_USER@$SERVER_IP << EOF
  sudo mkdir -p /opt/forge
  sudo chown -R $SERVER_USER:$SERVER_USER /opt/forge
  sudo mkdir -p /var/lib/forge-data
  sudo chown -R $SERVER_USER:$SERVER_USER /var/lib/forge-data
EOF

# Copy binary, config, and systemd service
scp forge $SERVER_USER@$SERVER_IP:/opt/forge/
scp $CONFIG_FILE $SERVER_USER@$SERVER_IP:/opt/forge/forge.json
scp $RULES_FILE $SERVER_USER@$SERVER_IP:/opt/forge/forge.rules
scp scripts/forge.service $SERVER_USER@$SERVER_IP:/tmp/

echo "🔥 Installing and Restarting Forge service..."
ssh $SERVER_USER@$SERVER_IP << EOF
  sudo mv /tmp/forge.service /etc/systemd/system/forge.service
  sudo systemctl daemon-reload
  sudo systemctl enable forge
  sudo systemctl restart forge
  sudo systemctl status forge --no-pager
EOF

echo "✅ Done! Forge is now live on $SERVER_IP:8080"
