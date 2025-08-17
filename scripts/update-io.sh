#!/bin/bash

# Script to update io dependency to the latest version

set -e

echo "🔍 Checking for io updates..."

# Get current version
CURRENT_VERSION=$(grep "github.com/zots0127/io" go.mod | awk '{print $2}')
echo "Current version: $CURRENT_VERSION"

# Get latest version from GitHub
LATEST_VERSION=$(curl -s https://api.github.com/repos/zots0127/io/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
echo "Latest version: $LATEST_VERSION"

if [ "$CURRENT_VERSION" == "$LATEST_VERSION" ]; then
    echo "✅ Already using the latest version!"
    exit 0
fi

echo "📦 Updating to $LATEST_VERSION..."

# Update using go get with direct proxy
GOPROXY=direct go get -u github.com/zots0127/io@$LATEST_VERSION

# Tidy up
go mod tidy

echo "✅ Successfully updated io from $CURRENT_VERSION to $LATEST_VERSION"

# Show what changed
echo ""
echo "📝 Changes in go.mod:"
git diff go.mod | head -20

# Test build
echo ""
echo "🔨 Testing build..."
if go build -o /tmp/test-build main.go; then
    rm /tmp/test-build
    echo "✅ Build successful!"
    
    # Offer to run tests
    echo ""
    read -p "Run tests? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        make test-quick
    fi
else
    echo "❌ Build failed! You may need to fix compatibility issues."
    exit 1
fi

echo ""
echo "📌 Next steps:"
echo "1. Review the changes"
echo "2. Run full test suite: make test"
echo "3. Commit the changes: git add go.mod go.sum && git commit -m 'deps: update io to $LATEST_VERSION'"