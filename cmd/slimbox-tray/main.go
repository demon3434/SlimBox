package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

type Config struct {
	DataDir string `json:"data_dir"`
	Port    int    `json:"port"`
}

func main() {
	log.Println("[SlimBox App] Starting Desktop Agent...")

	userHome, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to resolve home directory: %v", err)
	}

	configDir := filepath.Join(userHome, ".slimbox")
	configFile := filepath.Join(configDir, "config.json")
	_ = os.MkdirAll(configDir, 0755)

	config := loadConfig(configFile)

	// First run wizard: prompt user to pick storage directory via native dialog
	if config.DataDir == "" {
		selectedDir := promptFolderSelection()
		if selectedDir != "" {
			config.DataDir = selectedDir
		} else {
			// Fallback if user cancels dialog
			config.DataDir = defaultDataDir(userHome)
		}
		if config.Port == 0 {
			config.Port = 8080
		}
		saveConfig(configFile, config)
	}

	// Find the server executable
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	slimboxBin := findServerBinary(execDir)

	cmd := prepareChildCmd(exec.Command(slimboxBin,
		"--data-dir", config.DataDir,
		"--port", fmt.Sprintf("%d", config.Port),
	))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		notifyError(fmt.Sprintf("启动 SlimBox 核心服务失败: %v", err))
		log.Fatalf("Failed to start server process: %v", err)
	}

	log.Printf("[SlimBox App] Server process started (PID: %d). Opening browser...", cmd.Process.Pid)

	// Wait briefly for HTTP listener, then open default web browser
	time.AfterFunc(1200*time.Millisecond, func() {
		webURL := fmt.Sprintf("http://localhost:%d", config.Port)
		if err := openBrowser(webURL); err != nil {
			log.Printf("[SlimBox App] Failed to open browser: %v", err)
		}
		notifySuccess("SlimBox 硬件加速转码服务已就绪！")
	})

	// Graceful child process management on shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("[SlimBox App] Terminating server child process...")
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
	}()

	_ = cmd.Wait()
	log.Println("[SlimBox App] Process finished cleanly.")
}

func loadConfig(path string) Config {
	var cfg Config
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	return cfg
}

func saveConfig(path string, cfg Config) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}
