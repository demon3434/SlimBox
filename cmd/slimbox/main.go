package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"slimbox/internal/api"
	"slimbox/internal/engine"
	"slimbox/internal/repository"
	"slimbox/internal/scheduler"
	"slimbox/web"
)


func main() {
	var port int
	var dataDir string

	flag.IntVar(&port, "port", 8080, "HTTP server listening port")
	flag.StringVar(&dataDir, "data-dir", "", "Path to storage directory (mounted USB drive)")
	flag.Parse()

	// Environment variable fallback
	if dataDir == "" {
		dataDir = os.Getenv("SLIMBOX_DATA_DIR")
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

	uploadDir := filepath.Join(dataDir, "uploads")
	outputDir := filepath.Join(dataDir, "outputs")
	dbPath := filepath.Join(dataDir, "slimbox.db")

	log.Printf("[SlimBox] Initializing server on port %d...", port)
	log.Printf("[SlimBox] Storage root (USB disk mount): %s", dataDir)

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatalf("Failed to create uploads directory %s: %v", uploadDir, err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create outputs directory %s: %v", outputDir, err)
	}

	// Startup cleanup of orphan .part files from previous crashes (ADR-0011)
	engine.CleanOrphanPartials(outputDir)

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

	transcoder := engine.NewTranscoder()
	queue := scheduler.NewSerialQueue(taskRepo, settingsRepo, transcoder)
	queue.Start()
	defer queue.Stop()

	lifecycle := scheduler.NewLifecycleManager(taskRepo, settingsRepo)
	lifecycle.Start()
	defer lifecycle.Stop()

	server := api.NewServer(
		taskRepo,
		profileRepo,
		settingsRepo,
		queue,
		lifecycle,
		uploadDir,
		outputDir,
		dataDir,
		web.Assets,
	)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      server.Handler(),
		ReadTimeout:  120 * time.Minute, // Allow very large movie uploads
		WriteTimeout: 120 * time.Minute, // Allow large movie downloads
	}

	// Graceful shutdown handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[SlimBox] Server listening on http://0.0.0.0:%d", port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	<-stop
	log.Println("[SlimBox] Shutting down gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("[SlimBox] Server exited safely.")
}
