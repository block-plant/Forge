#!/usr/bin/env bash
# Script to build and publish the Forge TypeScript SDK

set -e

SDK_DIR="sdk/forge-sdk"

if [ ! -d "$SDK_DIR" ]; then
    echo "❌ Error: SDK directory $SDK_DIR not found."
    exit 1
fi

cd "$SDK_DIR"

echo "📦 Installing dependencies..."
npm install

echo "🔨 Building TypeScript SDK..."
npm run build

echo "npm publish will make this public. Are you logged in via 'npm login'? (y/n)"
read answer

if [ "$answer" != "${answer#[Yy]}" ] ;then
    echo "🚀 Publishing to npm..."
    npm publish --access public
    echo "✅ Successfully published Forge SDK to npm!"
else
    echo "Publishing aborted."
fi
