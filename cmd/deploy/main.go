package main

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	serverIP      = "192.168.7.186"
	serverPort    = 22
	sshUser       = "docker"
	sshPass       = "docker"
	remoteSrcDir  = "/opt/docker/slimbox/src"
	remoteBaseDir = "/opt/docker/slimbox"
	zipFile       = "slimbox_deploy_186.zip"
)

func main() {
	fmt.Println("==================================================")
	fmt.Printf("Deploying SlimBox to 186 Server (%s:8083)...\n", serverIP)
	fmt.Println("==================================================")

	// 1. Package source code
	fmt.Println("[1/4] Packaging local source code...")
	if err := packageZip(zipFile); err != nil {
		log.Fatalf("Packaging failed: %v", err)
	}
	defer os.Remove(zipFile)

	fi, err := os.Stat(zipFile)
	if err != nil {
		log.Fatalf("Failed to stat zip: %v", err)
	}
	fmt.Printf("Local package created: %s (%d bytes)\n", zipFile, fi.Size())

	// 2. Connect via SSH
	fmt.Printf("[2/4] Connecting to %s via SSH...\n", serverIP)
	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(sshPass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", serverIP, serverPort), config)
	if err != nil {
		log.Fatalf("SSH connection failed: %v", err)
	}
	defer client.Close()

	// 3. Upload Zip to remote /tmp
	fmt.Printf("Uploading %s to %s:/tmp...\n", zipFile, serverIP)
	sessionUpload, err := client.NewSession()
	if err != nil {
		log.Fatalf("Failed to create upload session: %v", err)
	}
	f, err := os.Open(zipFile)
	if err != nil {
		log.Fatalf("Failed to open local zip: %v", err)
	}
	defer f.Close()

	stdinPipe, err := sessionUpload.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to get stdin pipe: %v", err)
	}

	go func() {
		defer stdinPipe.Close()
		_, _ = io.Copy(stdinPipe, f)
	}()

	uploadCmd := fmt.Sprintf("cat > /tmp/%s", zipFile)
	if err := sessionUpload.Run(uploadCmd); err != nil {
		log.Fatalf("Failed to write remote file: %v", err)
	}
	sessionUpload.Close()

	// 4. Remote extraction and Docker build
	fmt.Println("[3/4] Extracting, Building and Launching SlimBox on port 8083...")
	sessionExec, err := client.NewSession()
	if err != nil {
		log.Fatalf("Failed to create exec session: %v", err)
	}
	defer sessionExec.Close()

	sessionExec.Stdout = os.Stdout
	sessionExec.Stderr = os.Stderr

	remoteCommands := fmt.Sprintf(`bash -lc '
set -e
mkdir -p %s %s/data
echo "Extracting fresh code to %s..."
mv -f /tmp/%s %s/%s
cd %s
unzip -o %s
rm -f %s

cat << "EOF" > .env
IMAGE_TAG=crpi-tyyqcg8a2rpatesk.cn-shanghai.personal.cr.aliyuncs.com/zixidaxian/slimbox:latest
WEB_PORT=8083
CONFIG_DATA_PATH=/opt/docker/slimbox/data
VIDEO_STORAGE_PATH=/mnt/usbdata/slimbox
TZ=Asia/Shanghai
DOCKER_LOG_MAX_SIZE=10m
DOCKER_LOG_MAX_FILE=3
SLIMBOX_AUTH_ENABLED=true
EOF

echo "Stopping old slimbox containers..."
docker compose down 2>/dev/null || true
echo "Building SlimBox image..."
docker compose build
echo "Starting container on port 8083..."
docker compose up -d
sleep 4
echo "=== Container Status ==="
docker compose ps
echo "=== Recent Container Logs (Check PIN code) ==="
docker compose logs --tail=30
echo "=== Health Check ==="
curl -i http://localhost:8083/api/v1/auth/status
'`, remoteSrcDir, remoteBaseDir, remoteSrcDir, zipFile, remoteSrcDir, zipFile, remoteSrcDir, zipFile, zipFile)

	if err := sessionExec.Run(remoteCommands); err != nil {
		log.Fatalf("Remote execution error: %v", err)
	}

	fmt.Println("\n==================================================")
	fmt.Printf("[4/4] SlimBox Successfully Deployed to %s:8083!\n", serverIP)
	fmt.Println("==================================================")
}

func packageZip(targetZip string) error {
	_ = os.Remove(targetZip)
	zipFile, err := os.Create(targetZip)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	w := zip.NewWriter(zipFile)
	defer w.Close()

	includedDirs := []string{"cmd", "internal", "web", "docs"}
	includedFiles := []string{"Dockerfile", "docker-compose.yml", "go.mod", "go.sum", "README.md", "CONTEXT.md"}

	for _, d := range includedDirs {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(d, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			for _, ignored := range []string{"__pycache__", ".venv", ".git", ".idea", "node_modules", "bin"} {
				if strings.Contains(path, ignored) {
					return nil
				}
			}
			if info.IsDir() {
				return nil
			}
			relPath, err := filepath.Rel(".", path)
			if err != nil {
				return err
			}
			relPath = strings.ReplaceAll(relPath, "\\", "/")
			zw, err := w.Create(relPath)
			if err != nil {
				return err
			}
			sf, err := os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(zw, sf)
			sf.Close()
			return err
		})
		if err != nil {
			return err
		}
	}

	for _, f := range includedFiles {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			continue
		}
		zw, err := w.Create(f)
		if err != nil {
			return err
		}
		sf, err := os.Open(f)
		if err != nil {
			return err
		}
		_, err = io.Copy(zw, sf)
		sf.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
