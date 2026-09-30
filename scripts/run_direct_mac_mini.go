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
		User: "liyichao",
		Auth: []ssh.AuthMethod{
			ssh.Password("carefree3434"),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", "192.168.7.196:22", config)
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
pkill -9 slimbox || true
sleep 1
echo "--- Running slimbox directly in foreground for 5 seconds ---"
~/.local/bin/slimbox --port 8080 --data-dir /Volumes/SlimBoxUSB/SlimBox &
PID=$!
echo "Started slimbox with PID $PID"
sleep 3
ps aux | grep slimbox | grep -v grep
lsof -i :8080 || true
curl -s http://127.0.0.1:8080/api/v1/system/stats || true
'
`
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := session.Run(cmd); err != nil {
		fmt.Printf("Run error: %v\n", err)
	}
}
