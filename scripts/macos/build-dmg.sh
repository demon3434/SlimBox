#!/usr/bin/env bash
set -euo pipefail

# SlimBox macOS Dual-Track Release Packaging Script
# Builds both:
# 1. Headless CLI package: slimbox-macos-arm64.tar.gz
# 2. Desktop Drag-and-Drop GUI package: SlimBox.dmg (containing SlimBox.app)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist/macos"
APP_NAME="SlimBox"
APP_BUNDLE="${DIST_DIR}/${APP_NAME}.app"

echo "=========================================================="
echo "          Building SlimBox for macOS (Apple Silicon)       "
echo "=========================================================="

mkdir -p "${DIST_DIR}"
rm -rf "${APP_BUNDLE}" "${DIST_DIR}/*.tar.gz" "${DIST_DIR}/*.dmg"

# 1. Compile binaries for darwin/arm64
echo "[1/4] Compiling Go backend and Cocoa status bar launcher for darwin/arm64..."
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o "${DIST_DIR}/slimbox-core" "${ROOT_DIR}/cmd/slimbox"
clang -framework Cocoa -O2 "${ROOT_DIR}/cmd/slimbox-tray/tray_darwin.m" -o "${DIST_DIR}/slimbox-tray"

# 2. Assemble Headless CLI Package (Track 1)
echo "[2/4] Packaging Headless CLI archive (slimbox-macos-arm64.tar.gz)..."
CLI_STAGE="${DIST_DIR}/headless_stage"
mkdir -p "${CLI_STAGE}"
cp -f "${DIST_DIR}/slimbox-core" "${CLI_STAGE}/slimbox"
cp -f "${SCRIPT_DIR}/install.sh" "${CLI_STAGE}/"
cp -f "${SCRIPT_DIR}/uninstall.sh" "${CLI_STAGE}/"
cp -f "${SCRIPT_DIR}/com.slimbox.server.plist" "${CLI_STAGE}/"
chmod +x "${CLI_STAGE}"/*.sh "${CLI_STAGE}/slimbox"

(cd "${CLI_STAGE}" && tar -czf "${DIST_DIR}/slimbox-macos-arm64.tar.gz" .)
rm -rf "${CLI_STAGE}"
echo "  -> Created ${DIST_DIR}/slimbox-macos-arm64.tar.gz"

# 3. Assemble Desktop .app Bundle (Track 2)
echo "[3/4] Creating ${APP_NAME}.app bundle structure..."
mkdir -p "${APP_BUNDLE}/Contents/MacOS"
mkdir -p "${APP_BUNDLE}/Contents/Resources"

cp -f "${DIST_DIR}/slimbox-tray" "${APP_BUNDLE}/Contents/MacOS/${APP_NAME}"
cp -f "${DIST_DIR}/slimbox-core" "${APP_BUNDLE}/Contents/MacOS/slimbox-core"
chmod +x "${APP_BUNDLE}/Contents/MacOS/"*

# Write Info.plist
cat <<EOF > "${APP_BUNDLE}/Contents/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleDisplayName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleIdentifier</key>
    <string>com.slimbox.desktop</string>
    <key>CFBundleVersion</key>
    <string>1.0.0</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleExecutable</key>
    <string>${APP_NAME}</string>
    <key>LSUIElement</key>
    <true/>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
EOF

# 4. Generate Drag-and-Drop .dmg (if hdiutil is available on macOS)
echo "[4/4] Generating Drag-and-Drop Disk Image (${APP_NAME}.dmg)..."
if command -v hdiutil &>/dev/null; then
    DMG_STAGE="${DIST_DIR}/dmg_stage"
    rm -rf "${DMG_STAGE}"
    mkdir -p "${DMG_STAGE}"
    cp -R "${APP_BUNDLE}" "${DMG_STAGE}/"
    ln -s /Applications "${DMG_STAGE}/Applications"

    hdiutil create -volname "${APP_NAME}" \
        -srcfolder "${DMG_STAGE}" \
        -ov -format UDZO \
        "${DIST_DIR}/${APP_NAME}.dmg"

    rm -rf "${DMG_STAGE}"
    echo "  -> Created ${DIST_DIR}/${APP_NAME}.dmg"
else
    echo "  (hdiutil not found in current environment. Packaging .app as .zip for macOS transfer)"
    (cd "${DIST_DIR}" && zip -r -q "${APP_NAME}.zip" "${APP_NAME}.app")
    echo "  -> Created ${DIST_DIR}/${APP_NAME}.zip"
fi

echo "=========================================================="
echo "✅ Build complete! Artifacts located in: ${DIST_DIR}"
echo "=========================================================="
