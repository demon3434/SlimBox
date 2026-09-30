package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

func main() {
	config := &ssh.ClientConfig{
		User:            "liyichao",
		Auth:            []ssh.AuthMethod{ssh.Password("carefree3434")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", "192.168.7.196:22", config)
	if err != nil {
		log.Fatalf("SSH failed: %v", err)
	}
	defer client.Close()

	session, _ := client.NewSession()
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	// 1. Unload launchd to prevent it from restarting repeatedly
	// 2. Kill any old slimbox processes
	// 3. Launch slimbox with nohup in user session
	cmd := `bash -lc '
echo "--- Stopping launchd agent ---"
launchctl unload ~/Library/LaunchAgents/com.slimbox.server.plist 2>/dev/null || true
pkill -9 slimbox 2>/dev/null || true
sleep 1

echo "--- Launching slimbox in background with nohup ---"
nohup ~/.local/bin/slimbox --port 8080 --data-dir /Volumes/SlimBoxUSB/SlimBox > ~/Library/Logs/slimbox/server.log 2> ~/Library/Logs/slimbox/server-error.log < /dev/null &
sleep 2

echo "--- Checking running process ---"
ps aux | grep slimbox | grep -v grep

echo "--- Checking Port 8080 ---"
lsof -i :8080 || true

echo "--- Checking HTTP Health ---"
curl -s -I http://127.0.0.1:8080/ | head -n 5
'`

	if err := session.Run(cmd); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
