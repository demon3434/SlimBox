package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"slimbox/internal/api"
	"slimbox/internal/collector"
	"slimbox/internal/engine"
	"slimbox/internal/repository"
	"slimbox/internal/scheduler"
	"slimbox/web"
)


func main() {
	var port int
	var dataDir string
	var uploadDirFlag string
	var outputDirFlag string
	var installSvcFlag bool
	var uninstallSvcFlag bool
	var noTrayFlag bool

	flag.IntVar(&port, "port", 8080, "HTTP server listening port")
	flag.StringVar(&dataDir, "data-dir", "", "Path to storage directory (mounted USB drive)")
	flag.StringVar(&uploadDirFlag, "upload-dir", "", "Path to upload directory (overrides data-dir/uploads)")
	flag.StringVar(&outputDirFlag, "output-dir", "", "Path to output directory (overrides data-dir/outputs)")
	flag.BoolVar(&installSvcFlag, "install-service", false, "Install and start as Windows System Service (UAC elevated)")
	flag.BoolVar(&uninstallSvcFlag, "uninstall-service", false, "Stop and uninstall Windows System Service (UAC elevated)")
	flag.BoolVar(&noTrayFlag, "no-tray", false, "Run in console mode without system tray icon")
	flag.Parse()

	// Load persistent configuration from config.json (if present)
	cfg := loadAppConfig()
	if port == 8080 && cfg.Port > 0 {
		port = cfg.Port
	}
	if dataDir == "" && cfg.DataDir != "" {
		dataDir = cfg.DataDir
	}

	// Environment variable fallback
	if dataDir == "" {
		dataDir = os.Getenv("SLIMBOX_DATA_DIR")
	}
	if uploadDirFlag == "" {
		uploadDirFlag = os.Getenv("SLIMBOX_UPLOAD_DIR")
	}
	if outputDirFlag == "" {
		outputDirFlag = os.Getenv("SLIMBOX_OUTPUT_DIR")
	}
	if dataDir == "" {
		dataDir = "/data"
		// If running locally without root /data access, fallback to ./data
		if _, err := os.Stat("/data"); os.IsNotExist(err) {
			if os.Getenv("DOCKER_CONTAINER") == "" {
				dataDir = "./data"
			}
		}
	}

	// Single instance mutex guard (Windows desktop interactive run)
	if !isWindowsService() && !installSvcFlag && !uninstallSvcFlag {
		alreadyRunning, releaseMutex := acquireSingleInstanceMutex()
		if alreadyRunning {
			openWebPage(port)
			showMsgBox("SlimBox 已在运行", fmt.Sprintf("SlimBox 已经在后台运行中。\n已为您自动打开 Web 控制台 (http://localhost:%d)。\n无需重复启动。", port), false)
			os.Exit(0)
		}
		defer releaseMutex()
	}

	// Service installation or uninstallation handling (Windows UAC elevated execution)
	if installSvcFlag {
		absDataDir, _ := filepath.Abs(dataDir)
		if err := installService(port, absDataDir); err != nil {
			showMsgBox("SlimBox 服务安装失败", fmt.Sprintf("无法注册并启动 Windows 系统服务:\n\n%v", err), true)
			os.Exit(1)
		}
		_ = setAutoStartEnabled(true)
		showMsgBox("SlimBox 服务安装成功", fmt.Sprintf("SlimBox Windows 系统服务已成功安装并启动！\n\n• 监听端口: %d\n• 数据目录: %s\n• 启动类型: 开机自动启动 (后台 24 小时待命)\n• 桌面托盘: 已自动开启开机自启常驻\n• 防火墙规则: 已自动放行 TCP %d 端口\n\n局域网内任意客户端现在均可直接访问转码服务。", port, absDataDir, port), false)
		os.Exit(0)
	}

	if uninstallSvcFlag {
		if err := uninstallService(); err != nil {
			showMsgBox("SlimBox 服务卸载失败", fmt.Sprintf("无法停止或删除 Windows 系统服务:\n\n%v", err), true)
			os.Exit(1)
		}
		_ = setAutoStartEnabled(false)
		showMsgBox("SlimBox 服务卸载成功", "SlimBox Windows 系统服务已成功停止并彻底删除！\n已自动清除防火墙入站放行规则与托盘自启动项。", false)
		os.Exit(0)
	}

	// If running interactively and the Windows Service is already running in background,
	// do not double-bind DB or ports. Directly enter Tray Controller mode.
	if !isWindowsService() && isServiceRunning() {
		log.Printf("[SlimBox] Windows background service is already active on port %d.", port)
		openWebPage(port)
		if !noTrayFlag {
			stopCh := make(chan os.Signal, 1)
			_ = startTray(port, dataDir, stopCh)
		}
		return
	}

	uploadDir := uploadDirFlag
	if uploadDir == "" {
		uploadDir = filepath.Join(dataDir, "uploads")
	}
	outputDir := outputDirFlag
	if outputDir == "" {
		outputDir = filepath.Join(dataDir, "outputs")
	}
	dbPath := filepath.Join(dataDir, "slimbox.db")

	log.Printf("[SlimBox] Initializing server on port %d...", port)
	log.Printf("[SlimBox] Storage root (USB disk mount): %s", dataDir)
	log.Printf("[SlimBox] Uploads path: %s", uploadDir)
	log.Printf("[SlimBox] Outputs path: %s", outputDir)

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatalf("Failed to create uploads directory %s: %v", uploadDir, err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create outputs directory %s: %v", outputDir, err)
	}

	// Startup cleanup of orphan .part files from previous crashes (ADR-0011)
	engine.CleanOrphanPartials(outputDir)
	// Startup cleanup of abandoned chunk sessions from previous crashes
	if cCount, cBytes := engine.CleanOrphanTempUploads(uploadDir, 24*time.Hour); cCount > 0 {
		log.Printf("[SlimBox] Startup cleanup removed %d abandoned chunk upload sessions (%d bytes)", cCount, cBytes)
	}

	// Initialize SQLite Database
	db, err := repository.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database at %s: %v", dbPath, err)
	}
	defer db.Close()
	log.Printf("[SlimBox] SQLite database initialized successfully at %s", dbPath)

	taskRepo := repository.NewTaskRepository(db)
	profileRepo := repository.NewProfileRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)
	tokenRepo := repository.NewTokenRepository(db)
	adminRepo := repository.NewAdminRepository(db)

	authEnabled := strings.ToLower(strings.TrimSpace(os.Getenv("SLIMBOX_AUTH_ENABLED"))) == "true"
	if authEnabled {
		log.Printf("[Security] SLIMBOX_AUTH_ENABLED=true. Strict security mode is active.")
		adminPasswordEnv := strings.TrimSpace(os.Getenv("SLIMBOX_ADMIN_PASSWORD"))
		if adminPasswordEnv != "" {
			if err := adminRepo.SetAdminPassword(adminPasswordEnv); err != nil {
				log.Fatalf("[Security] Failed to pre-seed admin password: %v", err)
			}
			log.Printf("[Security] Master administrator password successfully configured from SLIMBOX_ADMIN_PASSWORD.")
		} else {
			hasPassword, err := adminRepo.HasAdminPassword()
			if err != nil {
				log.Fatalf("[Security] Failed to inspect admin credentials: %v", err)
			}
			if !hasPassword {
				pin, err := adminRepo.GenerateSetupPIN()
				if err != nil {
					log.Fatalf("[Security] Failed to generate setup PIN: %v", err)
				}
				log.Println("======================================================================")
				log.Println("[Security] No master administrator password found.")
				log.Printf("[Security] Single-use Web Setup PIN: >>> %s <<<", pin)
				log.Println("[Security] Open Web UI and enter this PIN code to set master password.")
				log.Println("======================================================================")
			}
		}
	} else {
		log.Printf("[Security] SLIMBOX_AUTH_ENABLED=false. Running in open local network mode.")
	}

	metricsCollector := collector.NewMetricsCollector(dataDir)
	metricsCollector.Start()
	defer metricsCollector.Stop()

	transcoder := engine.NewTranscoder()
	queue := scheduler.NewSerialQueue(taskRepo, settingsRepo, transcoder)
	queue.Start()
	defer queue.Stop()

	lifecycle := scheduler.NewLifecycleManager(taskRepo, settingsRepo, uploadDir)
	lifecycle.Start()
	defer lifecycle.Stop()

	// Pre-warm hardware acceleration detection asynchronously at boot (eliminates runtime probe delay)
	go func() {
		log.Println("[SlimBox] Pre-warming hardware accelerators (GPU/NPU) at startup...")
		accels := engine.GetCachedAccelerators()
		log.Printf("[SlimBox] Hardware accelerators ready: %v", accels)
	}()

	server := api.NewServer(
		taskRepo,
		profileRepo,
		settingsRepo,
		tokenRepo,
		adminRepo,
		authEnabled,
		queue,
		lifecycle,
		uploadDir,
		outputDir,
		dataDir,
		web.Assets,
		metricsCollector,
	)


	httpServer := &http.Server{
		Handler:      server.Handler(),
		ReadTimeout:  120 * time.Minute, // Allow very large movie uploads
		WriteTimeout: 120 * time.Minute, // Allow large movie downloads
	}

	// Adaptive port binding: probe port, fallback to port+1... if busy
	var listener net.Listener
	actualPort := port
	for attempt := 0; attempt < 20; attempt++ {
		candidatePort := port + attempt
		l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", candidatePort))
		if err == nil {
			listener = l
			actualPort = candidatePort
			break
		}
		log.Printf("[SlimBox] Port %d is occupied or unavailable: %v. Probing next port...", candidatePort, err)
	}

	if listener == nil {
		log.Fatalf("Failed to bind any HTTP port from %d to %d", port, port+19)
	}

	if actualPort != port {
		log.Printf("[SlimBox] Notice: default port %d was occupied. Successfully bound to available port %d.", port, actualPort)
	}
	port = actualPort
	httpServer.Addr = fmt.Sprintf("0.0.0.0:%d", port)

	// Graceful shutdown handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[SlimBox] Server listening on http://0.0.0.0:%d", port)
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	if isWindowsService() {
		log.Println("[SlimBox] Detected Windows Service environment. Connecting to SCM...")
		go func() {
			if err := runWindowsService("SlimBox", func() {
				stop <- os.Interrupt
			}); err != nil {
				log.Printf("[SlimBox Service] SCM dispatch error: %v", err)
				stop <- os.Interrupt
			}
		}()
	} else if !noTrayFlag {
		go func() {
			if err := startTray(port, dataDir, stop); err != nil {
				log.Printf("[SlimBox Tray] Tray error or unsupported: %v", err)
			}
		}()
	}

	<-stop
	log.Println("[SlimBox] Shutting down gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("[SlimBox] Server exited safely.")
	os.Exit(0)
}
