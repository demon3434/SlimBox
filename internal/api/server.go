package api

import (
	"io/fs"
	"net/http"
	"os"
	"strings"

	"slimbox/internal/collector"
	"slimbox/internal/repository"
	"slimbox/internal/scheduler"
)

type Server struct {
	mux            *http.ServeMux
	uploadHandler  *UploadHandler
	taskHandler    *TaskHandler
	profileHandler *ProfileHandler
	systemHandler      *SystemHandler
	storageHandler     *StorageHandler
	authHandler        *AuthHandler
	authMiddleware     *AuthMiddleware
	cliDownloadHandler *CLIDownloadHandler
	webFS              fs.FS
}

func NewServer(
	taskRepo *repository.TaskRepository,
	profileRepo *repository.ProfileRepository,
	settingsRepo *repository.SettingsRepository,
	tokenRepo *repository.TokenRepository,
	adminRepo *repository.AdminRepository,
	authEnabled bool,
	queue *scheduler.SerialQueue,
	lifecycle *scheduler.LifecycleManager,
	uploadDir string,
	outputDir string,
	dataDir string,
	webFS fs.FS,
	metricsCollector *collector.MetricsCollector,
) *Server {
	authMid := NewAuthMiddleware(authEnabled, tokenRepo, adminRepo)
	s := &Server{
		mux:                http.NewServeMux(),
		uploadHandler:      NewUploadHandler(taskRepo, uploadDir),
		taskHandler:        NewTaskHandler(taskRepo, profileRepo, queue, lifecycle, outputDir),
		profileHandler:     NewProfileHandler(profileRepo),
		systemHandler:      NewSystemHandler(settingsRepo, dataDir, metricsCollector),
		storageHandler:     NewStorageHandler(taskRepo, uploadDir, outputDir),
		authHandler:        NewAuthHandler(tokenRepo, adminRepo, authMid),
		authMiddleware:     authMid,
		cliDownloadHandler: NewCLIDownloadHandler(),
		webFS:              webFS,
	}

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// API Endpoints
	s.mux.HandleFunc("/api/v1/upload", s.uploadHandler.HandleUpload)
	s.mux.HandleFunc("/api/v1/upload/chunk", s.uploadHandler.HandleUploadChunk)
	s.mux.HandleFunc("/api/v1/upload/complete", s.uploadHandler.HandleUploadComplete)
	s.mux.HandleFunc("/api/v1/upload/status", s.uploadHandler.HandleUploadStatus)
	s.mux.HandleFunc("/api/v1/cli/download", s.cliDownloadHandler.HandleDownload)

	// Auth routes
	s.mux.HandleFunc("/api/v1/auth/status", s.authHandler.HandleStatus)
	s.mux.HandleFunc("/api/v1/auth/login", s.authHandler.HandleLogin)
	s.mux.HandleFunc("/api/v1/auth/setup", s.authHandler.HandleSetup)
	s.mux.HandleFunc("/api/v1/auth/verify", s.authHandler.HandleVerify)
	s.mux.HandleFunc("/api/v1/auth/tokens", s.authMiddleware.RequireAdmin(s.authHandler.HandleTokens))
	s.mux.HandleFunc("/api/v1/auth/tokens/", s.authMiddleware.RequireAdmin(s.authHandler.HandleTokenSubroute))

	// Task routes: /api/v1/tasks, /api/v1/tasks/{id}, /api/v1/tasks/{id}/start, /api/v1/tasks/{id}/abort, /api/v1/tasks/{id}/download
	s.mux.HandleFunc("/api/v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/tasks" || r.URL.Path == "/api/v1/tasks/" {
			s.taskHandler.HandleListTasks(w, r)
			return
		}
		s.dispatchTaskSubroutes(w, r)
	})
	s.mux.HandleFunc("/api/v1/tasks/", s.dispatchTaskSubroutes)

	// Profiles routes - mutating profiles requires admin
	s.mux.HandleFunc("/api/v1/profiles", s.profileHandler.HandleProfiles)
	s.mux.HandleFunc("/api/v1/profiles/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
			s.authMiddleware.RequireAdmin(s.profileHandler.HandleProfiles)(w, r)
			return
		}
		s.profileHandler.HandleProfiles(w, r)
	})

	// System routes - changing settings requires admin
	s.mux.HandleFunc("/api/v1/system/stats", s.systemHandler.HandleStats)
	s.mux.HandleFunc("/api/v1/system/gpu-config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			s.authMiddleware.RequireAdmin(s.systemHandler.HandleGPUConfig)(w, r)
			return
		}
		s.systemHandler.HandleGPUConfig(w, r)
	})
	s.mux.HandleFunc("/api/v1/system/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			s.authMiddleware.RequireAdmin(s.systemHandler.HandleSettings)(w, r)
			return
		}
		s.systemHandler.HandleSettings(w, r)
	})

	// Storage maintenance routes (Admin required for clean)
	s.mux.HandleFunc("/api/v1/system/orphans/scan", s.storageHandler.HandleScanOrphans)
	s.mux.HandleFunc("/api/v1/system/orphans/clean", func(w http.ResponseWriter, r *http.Request) {
		s.authMiddleware.RequireAdmin(s.storageHandler.HandleCleanOrphans)(w, r)
	})

	// Frontend static assets (prioritize adjacent web/ directory if present, fallback to embedded webFS)
	var fsHandler http.Handler
	if fi, err := os.Stat("web"); err == nil && fi.IsDir() {
		fsHandler = http.FileServer(http.Dir("web"))
	} else if s.webFS != nil {
		fsHandler = http.FileServer(http.FS(s.webFS))
	}
	if fsHandler != nil {
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			// Disable cache for HTML and entry files to prevent stale dashboard loads
			if r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, ".html") || strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
				w.Header().Set("Pragma", "no-cache")
				w.Header().Set("Expires", "0")
			}
			fsHandler.ServeHTTP(w, r)
		})
	}
}

func (s *Server) dispatchTaskSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	if path == "" {
		s.taskHandler.HandleListTasks(w, r)
		return
	}

	if (path == "batch-delete" || path == "batch-delete/") && r.Method == http.MethodPost {
		s.taskHandler.HandleBatchDelete(w, r)
		return
	}
	if (path == "batch-start" || path == "batch-start/") && r.Method == http.MethodPost {
		s.taskHandler.HandleBatchStart(w, r)
		return
	}
	if (path == "batch-retry" || path == "batch-retry/") && r.Method == http.MethodPost {
		s.taskHandler.HandleBatchRetry(w, r)
		return
	}
	if (path == "batch-pause" || path == "batch-pause/") && r.Method == http.MethodPost {
		s.taskHandler.HandleBatchPause(w, r)
		return
	}
	if (path == "batch-resume" || path == "batch-resume/") && r.Method == http.MethodPost {
		s.taskHandler.HandleBatchResume(w, r)
		return
	}
	if (path == "reorder" || path == "reorder/") && (r.Method == http.MethodPut || r.Method == http.MethodPost) {
		s.taskHandler.HandleReorderTasks(w, r)
		return
	}

	if strings.HasSuffix(path, "/start") && r.Method == http.MethodPost {
		s.taskHandler.HandleStartTask(w, r)
		return
	}
	if strings.HasSuffix(path, "/pause") && r.Method == http.MethodPost {
		s.taskHandler.HandlePauseTask(w, r)
		return
	}
	if strings.HasSuffix(path, "/resume") && r.Method == http.MethodPost {
		s.taskHandler.HandleResumeTask(w, r)
		return
	}
	if strings.HasSuffix(path, "/abort") && r.Method == http.MethodPost {
		s.taskHandler.HandleAbortTask(w, r)
		return
	}
	if strings.HasSuffix(path, "/retry") && r.Method == http.MethodPost {
		s.taskHandler.HandleRetryTask(w, r)
		return
	}
	if strings.HasSuffix(path, "/priority") && r.Method == http.MethodPost {
		s.taskHandler.HandleAdjustPriority(w, r)
		return
	}
	if strings.HasSuffix(path, "/download") && r.Method == http.MethodGet {
		s.taskHandler.HandleDownload(w, r)
		return
	}

	// Single task operations
	switch r.Method {
	case http.MethodGet:
		s.taskHandler.HandleGetTask(w, r)
	case http.MethodDelete:
		s.taskHandler.HandleDeleteTask(w, r)
	default:
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// Handler returns the HTTP handler wrapped with optional auth middleware.
func (s *Server) Handler() http.Handler {
	return s.authMiddleware.Middleware(s.mux)
}
