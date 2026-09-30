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
	localBin   = "slimbox-darwin-arm64"
	remoteTmp  = "/tmp/slimbox-darwin-arm64"
)

func main() {
	log.Println("=== Deploying SlimBox to Mac mini (192.168.7.196) ===")

	// 1. Check local binary
	stat, err := os.Stat(localBin)
	if err != nil {
		log.Fatalf("Local binary %s not found: %v", localBin, err)
	}
	log.Printf("[1/4] Found local binary: %s (%d bytes)", localBin, stat.Size())

	// 2. SSH Client config
	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(sshPass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(serverIP, serverPort)
	log.Printf("[2/4] Connecting to Mac mini at %s...", addr)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		log.Fatalf("Failed to dial SSH: %v", err)
	}
	defer client.Close()
	log.Println("[*] SSH connection established.")

	// 3. Upload file via SCP / standard pipe
	log.Printf("[3/4] Uploading %s to %s on Mac mini...", localBin, remoteTmp)
	session, err := client.NewSession()
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}

	file, err := os.Open(localBin)
	if err != nil {
		log.Fatalf("Failed to open local binary: %v", err)
	}
	defer file.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to get stdin pipe: %v", err)
	}

	go func() {
		defer stdin.Close()
		fmt.Fprintf(stdin, "C0755 %d %s\n", stat.Size(), "slimbox-darwin-arm64")
		io.Copy(stdin, file)
		fmt.Fprint(stdin, "\x00")
	}()

	if err := session.Run("scp -tr /tmp"); err != nil {
		log.Fatalf("Failed to upload via scp: %v", err)
	}
	session.Close()
	log.Println("[*] Upload completed successfully.")

	// 4. Install and restart service on Mac mini
	log.Println("[4/4] Installing binary and updating SlimBox on Mac mini...")
	execSession, err := client.NewSession()
	if err != nil {
		log.Fatalf("Failed to create exec session: %v", err)
	}
	defer execSession.Close()

	remoteScript := `bash -lc '
set -e
echo "--- Installing binary to ~/.local/bin/slimbox ---"
mkdir -p ~/.local/bin
mv -f /tmp/slimbox-darwin-arm64 ~/.local/bin/slimbox
chmod +x ~/.local/bin/slimbox
xattr -d com.apple.quarantine ~/.local/bin/slimbox 2>/dev/null || true

echo "--- Updating launchd plist to ensure KeepAlive true ---"
mkdir -p ~/Library/LaunchAgents
cat << "EOF" > ~/Library/LaunchAgents/com.slimbox.server.plist
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.slimbox.server</string>
    <key>ProgramArguments</key>
    <array>
        <string>/Users/liyichao/.local/bin/slimbox</string>
        <string>--port</string>
        <string>8080</string>
        <string>--data-dir</string>
        <string>/Volumes/SlimBoxUSB/SlimBox</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>
    <key>StandardOutPath</key>
    <string>/Users/liyichao/Library/Logs/slimbox/server.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/liyichao/Library/Logs/slimbox/server-error.log</string>
    <key>ProcessType</key>
    <string>Standard</string>
</dict>
</plist>
EOF

echo "--- Unloading & Killing any old process ---"
launchctl unload ~/Library/LaunchAgents/com.slimbox.server.plist 2>/dev/null || true
pkill -9 slimbox 2>/dev/null || true
sleep 1

echo "--- Loading launchd service ---"
launchctl load -w ~/Library/LaunchAgents/com.slimbox.server.plist
sleep 2

echo "--- Process Check ---"
ps aux | grep slimbox | grep -v grep || true

echo "--- Port Listen Check ---"
lsof -i :8080 || true

echo "--- Health Check (http://localhost:8080/api/v1/system/stats) ---"
curl -s http://localhost:8080/api/v1/system/stats || true
echo ""
echo "=== Deployment to Mac mini Completed Successfully! ==="
'`

	execSession.Stdout = os.Stdout
	execSession.Stderr = os.Stderr

	if err := execSession.Run(remoteScript); err != nil {
		log.Fatalf("Remote command execution failed: %v", err)
	}
}
