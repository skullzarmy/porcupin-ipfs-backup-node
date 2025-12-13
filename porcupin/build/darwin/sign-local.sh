#!/bin/bash
# Local code signing script for development/testing
# Signs the app with entitlements so it works when launched from Finder

APP_PATH="./build/bin/Porcupin.app"
ENTITLEMENTS_PATH="./build/darwin/entitlements.plist"

if [ ! -d "$APP_PATH" ]; then
    echo "Error: $APP_PATH not found. Build the app first with 'wails build'"
    exit 1
fi

if [ ! -f "$ENTITLEMENTS_PATH" ]; then
    echo "Error: $ENTITLEMENTS_PATH not found"
    exit 1
fi

echo "Removing quarantine attribute..."
xattr -cr "$APP_PATH" 2>/dev/null || true

echo "Signing app with entitlements (ad-hoc)..."
codesign --force --deep --sign - --entitlements "$ENTITLEMENTS_PATH" "$APP_PATH"

if [ $? -eq 0 ]; then
    echo ""
    echo "✅ App signed successfully!"
    echo ""
    echo "You can now launch Porcupin.app from Finder."
    echo "Location: $APP_PATH"
else
    echo ""
    echo "❌ Signing failed"
    exit 1
fi
