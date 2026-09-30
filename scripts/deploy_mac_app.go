package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	serverIP   = "192.168.7.196"
	serverPort = "22"
	sshUser    = "liyichao"
	sshPass    = "carefree3434"
)

func scpFile(client *ssh.Client, localPath, remotePath string, mode string) error {
	stat, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		fmt.Fprintf(stdin, "C%s %d %s\n", mode, stat.Size(), stat.Name())
		io.Copy(stdin, f)
		fmt.Fprint(stdin, "\x00")
	}()

	return session.Run(fmt.Sprintf("scp -t %s", remotePath))
}

func main() {
	log.Println("=== Building and Installing SlimBox.app to macOS /Applications ===")

	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(sshPass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(serverIP, serverPort), config)
	if err != nil {
		log.Fatalf("SSH connection failed: %v", err)
	}
	defer client.Close()
	log.Println("[*] SSH connection established to Mac mini.")

	// 1. Upload slimbox-core
	log.Println("[1/4] Uploading slimbox-core...")
	if err := scpFile(client, "dist/macos/slimbox-core", "/tmp", "0755"); err != nil {
		log.Fatalf("Failed to upload slimbox-core: %v", err)
	}

	// 2. Upload tray_darwin.m
	log.Println("[2/4] Uploading tray_darwin.m...")
	if err := scpFile(client, "cmd/slimbox-tray/tray_darwin.m", "/tmp", "0644"); err != nil {
		log.Fatalf("Failed to upload tray_darwin.m: %v", err)
	}

	// 3. Upload web/logo.png
	log.Println("[3/5] Uploading web/logo.png...")
	if err := scpFile(client, "web/logo.png", "/tmp", "0644"); err != nil {
		log.Fatalf("Failed to upload web/logo.png: %v", err)
	}

	// 4. Compile, Generate .icns and Assemble /Applications/SlimBox.app
	log.Println("[4/5] Generating AppIcon.icns, compiling Cocoa launcher and assembling /Applications/SlimBox.app...")
	session, err := client.NewSession()
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}
	defer session.Close()

	assembleScript := `bash -lc '
set -e
echo "-> Compiling Cocoa Status Bar launcher..."
clang -framework Cocoa -O2 /tmp/tray_darwin.m -o /tmp/SlimBox-Launcher
chmod +x /tmp/SlimBox-Launcher

echo "-> Generating macOS AppIcon.icns from logo.png..."
rm -rf /tmp/AppIcon.iconset /tmp/AppIcon.icns
mkdir -p /tmp/AppIcon.iconset
sips -s format png -z 16 16     /tmp/logo.png --out /tmp/AppIcon.iconset/icon_16x16.png >/dev/null
sips -s format png -z 32 32     /tmp/logo.png --out /tmp/AppIcon.iconset/icon_16x16@2x.png >/dev/null
sips -s format png -z 32 32     /tmp/logo.png --out /tmp/AppIcon.iconset/icon_32x32.png >/dev/null
sips -s format png -z 64 64     /tmp/logo.png --out /tmp/AppIcon.iconset/icon_32x32@2x.png >/dev/null
sips -s format png -z 128 128   /tmp/logo.png --out /tmp/AppIcon.iconset/icon_128x128.png >/dev/null
sips -s format png -z 256 256   /tmp/logo.png --out /tmp/AppIcon.iconset/icon_128x128@2x.png >/dev/null
sips -s format png -z 256 256   /tmp/logo.png --out /tmp/AppIcon.iconset/icon_256x256.png >/dev/null
sips -s format png -z 512 512   /tmp/logo.png --out /tmp/AppIcon.iconset/icon_256x256@2x.png >/dev/null
sips -s format png -z 512 512   /tmp/logo.png --out /tmp/AppIcon.iconset/icon_512x512.png >/dev/null
sips -s format png -z 1024 1024 /tmp/logo.png --out /tmp/AppIcon.iconset/icon_512x512@2x.png >/dev/null
iconutil -c icns /tmp/AppIcon.iconset -o /tmp/AppIcon.icns

echo "-> Assembling /Applications/SlimBox.app..."
APP_DIR="/Applications/SlimBox.app"
rm -rf "$APP_DIR" 2>/dev/null || true
mkdir -p "$APP_DIR/Contents/MacOS"
mkdir -p "$APP_DIR/Contents/Resources"

mv -f /tmp/SlimBox-Launcher "$APP_DIR/Contents/MacOS/SlimBox"
mv -f /tmp/slimbox-core "$APP_DIR/Contents/MacOS/slimbox-core"
cp -f /tmp/AppIcon.icns "$APP_DIR/Contents/Resources/AppIcon.icns"
cp -f /tmp/logo.png "$APP_DIR/Contents/Resources/logo.png"
chmod +x "$APP_DIR/Contents/MacOS/"*

echo "-> Generating Info.plist and PkgInfo..."
cat << "EOF" > "$APP_DIR/Contents/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>SlimBox</string>
    <key>CFBundleDisplayName</key>
    <string>SlimBox</string>
    <key>CFBundleIdentifier</key>
    <string>com.slimbox.desktop</string>
    <key>CFBundleVersion</key>
    <string>1.0.0</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleExecutable</key>
    <string>SlimBox</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>LSUIElement</key>
    <true/>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
EOF

echo -n "APPL????" > "$APP_DIR/Contents/PkgInfo"

# Clear quarantine flags
xattr -cr "$APP_DIR" 2>/dev/null || true

# Register with LaunchServices
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$APP_DIR" 2>/dev/null || true

echo "-> Successfully assembled /Applications/SlimBox.app"
ls -la "$APP_DIR/Contents/MacOS"
'`
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	if err := session.Run(assembleScript); err != nil {
		log.Fatalf("Assembly failed: %v", err)
	}

	// 4. Launch SlimBox.app on Mac mini
	log.Println("[4/4] Launching /Applications/SlimBox.app...")
	launchSession, err := client.NewSession()
	if err != nil {
		log.Fatalf("Failed to create launch session: %v", err)
	}
	defer launchSession.Close()

	launchScript := `bash -lc '
pkill -f "/Applications/SlimBox.app/Contents/MacOS/SlimBox" 2>/dev/null || true
open /Applications/SlimBox.app
sleep 1
ps aux | grep "[S]limBox"
'`
	launchSession.Stdout = os.Stdout
	launchSession.Stderr = os.Stderr
	_ = launchSession.Run(launchScript)

	log.Println("=== SlimBox.app Successfully Installed to /Applications and Launched! ===")

	// 5. Generate SlimBox.dmg on Mac mini and download to dist/macos/SlimBox.dmg
	log.Println("[5/5] Generating Drag-and-Drop SlimBox.dmg on Mac mini...")
	dmgGenSession, err := client.NewSession()
	if err == nil {
		defer dmgGenSession.Close()
		dmgScript := `bash -lc '
DMG_STAGE="/tmp/dmg_stage"
rm -rf "$DMG_STAGE" /tmp/SlimBox.dmg
mkdir -p "$DMG_STAGE"
cp -R "/Applications/SlimBox.app" "$DMG_STAGE/"
ln -s /Applications "$DMG_STAGE/Applications"
hdiutil create -volname "SlimBox" -srcfolder "$DMG_STAGE" -ov -format UDZO /tmp/SlimBox.dmg >/dev/null
rm -rf "$DMG_STAGE"
ls -lh /tmp/SlimBox.dmg
'`
		dmgGenSession.Stdout = os.Stdout
		dmgGenSession.Stderr = os.Stderr
		if err := dmgGenSession.Run(dmgScript); err == nil {
			log.Println("[*] Downloading SlimBox.dmg to dist/macos/SlimBox.dmg...")
			dlSession, err := client.NewSession()
			if err == nil {
				defer dlSession.Close()
				outFile, err := os.Create("dist/macos/SlimBox.dmg")
				if err == nil {
					dlSession.Stdout = outFile
					if err := dlSession.Run("cat /tmp/SlimBox.dmg"); err == nil {
						outFile.Close()
						log.Println("[*] Successfully downloaded dist/macos/SlimBox.dmg")
					} else {
						outFile.Close()
						_ = os.Remove("dist/macos/SlimBox.dmg")
					}
				}
			}
		}
	}
}
