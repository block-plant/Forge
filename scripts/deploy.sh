#!/usr/bin/env bash
# Forge deployment script for remote servers (e.g., Oracle Free Tier ARM)

set -e

# Configuration
SERVER_IP="your-server-ip"
SERVER_USER="ubuntu"
PORT="8080"
CONFIG_FILE="forge.yaml"

echo "🔥 Building Forge for Linux ARM64..."
GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o forge ./main.go

echo "🔥 Deploying to $SERVER_USER@$SERVER_IP..."
scp forge $SERVER_USER@$SERVER_IP:/home/$SERVER_USER/
scp $CONFIG_FILE $SERVER_USER@$SERVER_IP:/home/$SERVER_USER/

echo "🔥 Restarting Forge service..."
ssh $SERVER_USER@$SERVER_IP << EOF
  sudo systemctl restart forge
  sudo systemctl status forge --no-pager
EOF

echo "Done! Forge is now live."
