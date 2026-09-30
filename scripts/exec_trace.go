package main

import (
	"fmt"
	"io"
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

	// Upload trace binary
	stat, _ := os.Stat("trace_startup_darwin")
	f, _ := os.Open("trace_startup_darwin")
	defer f.Close()

	session, _ := client.NewSession()
	stdin, _ := session.StdinPipe()
	go func() {
		defer stdin.Close()
		fmt.Fprintf(stdin, "C0755 %d %s\n", stat.Size(), "trace_startup_darwin")
		io.Copy(stdin, f)
		fmt.Fprint(stdin, "\x00")
	}()
	session.Run("scp -tr /tmp")
	session.Close()

	// Execute trace
	session2, _ := client.NewSession()
	defer session2.Close()
	session2.Stdout = os.Stdout
	session2.Stderr = os.Stderr
	session2.Run("/tmp/trace_startup_darwin")
}
