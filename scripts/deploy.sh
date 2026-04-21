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

echo "🔥 Building Forge for Linux AMD64 (x86_64)..."
# Using amd64 for the VM.Standard.E2.1.Micro AMD instance
GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o forge ./main.go

echo "🔥 Transporting files to $SERVER_USER@$SERVER_IP..."

# Copy binary, config, systemd service, and frontend dashboard
scp -r ./forge ./forge.prod.json ./forge.rules ./scripts/forge.service ./dashboard $SERVER_USER@$SERVER_IP:~/

echo "🔥 Installing and Restarting Forge service..."
ssh $SERVER_USER@$SERVER_IP << EOF
  # Setup engine
  sudo mkdir -p /opt/forge
  sudo mv ~/forge /opt/forge/
  sudo mv ~/forge.prod.json /opt/forge/forge.json
  sudo mv ~/forge.rules /opt/forge/
  sudo chown -R $SERVER_USER:$SERVER_USER /opt/forge
  
  # Setup database and hosting folders
  sudo mkdir -p /var/lib/forge-data/hosting/projects
  sudo rm -rf /var/lib/forge-data/hosting/projects/dashboard
  sudo mv ~/dashboard /var/lib/forge-data/hosting/projects/
  sudo chown -R $SERVER_USER:$SERVER_USER /var/lib/forge-data

  # Reload daemon
  sudo mv ~/forge.service /etc/systemd/system/forge.service
  sudo systemctl daemon-reload
  sudo systemctl enable forge
  sudo systemctl restart forge
  sudo systemctl status forge --no-pager
EOF

echo "✅ Done! Forge is now live on $SERVER_IP:8080"
