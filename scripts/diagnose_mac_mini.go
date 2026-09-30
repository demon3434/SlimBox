package main

import (
	"fmt"
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

func main() {
	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(sshPass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(serverIP, serverPort)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		log.Fatalf("Failed to dial SSH: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}
	defer session.Close()

	cmd := `bash -lc '
echo "=== Clean Reloading Launchd Service ==="
launchctl unload ~/Library/LaunchAgents/com.slimbox.server.plist 2>/dev/null || true
pkill -9 slimbox 2>/dev/null || true
sleep 1
launchctl load -w ~/Library/LaunchAgents/com.slimbox.server.plist
sleep 2

echo "=== Launchd Service Status ==="
launchctl list | grep slimbox || true

echo "=== Process Status ==="
ps aux | grep slimbox | grep -v grep

echo "=== Server Error Log ==="
tail -n 40 ~/Library/Logs/slimbox/server-error.log
'`

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := session.Run(cmd); err != nil {
		fmt.Printf("Run error: %v\n", err)
	}
}
