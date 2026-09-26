package api

import (
	"io/fs"
	"net/http"
	"strings"

	"slimbox/internal/repository"
	"slimbox/internal/scheduler"
)

type Server struct {
	mux            *http.ServeMux
	uploadHandler  *UploadHandler
	taskHandler    *TaskHandler
	profileHandler *ProfileHandler
	systemHandler  *SystemHandler
	webFS          fs.FS
}

func NewServer(
	taskRepo *repository.TaskRepository,
	profileRepo *repository.ProfileRepository,
	settingsRepo *repository.SettingsRepository,
	queue *scheduler.SerialQueue,
	lifecycle *scheduler.LifecycleManager,
	uploadDir string,
	outputDir string,
	dataDir string,
	webFS fs.FS,
) *Server {
	s := &Server{
		mux:            http.NewServeMux(),
		uploadHandler:  NewUploadHandler(taskRepo, uploadDir),
		taskHandler:    NewTaskHandler(taskRepo, profileRepo, queue, lifecycle, outputDir),
		profileHandler: NewProfileHandler(profileRepo),
		systemHandler:  NewSystemHandler(settingsRepo, dataDir),
		webFS:          webFS,
	}

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// API Endpoints
	s.mux.HandleFunc("/api/v1/upload", s.uploadHandler.HandleUpload)

	// Task routes: /api/v1/tasks, /api/v1/tasks/{id}, /api/v1/tasks/{id}/start, /api/v1/tasks/{id}/abort, /api/v1/tasks/{id}/download
	s.mux.HandleFunc("/api/v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/tasks" || r.URL.Path == "/api/v1/tasks/" {
			s.taskHandler.HandleListTasks(w, r)
			return
		}
		s.dispatchTaskSubroutes(w, r)
	})
	s.mux.HandleFunc("/api/v1/tasks/", s.dispatchTaskSubroutes)

	// Profiles routes
	s.mux.HandleFunc("/api/v1/profiles", s.profileHandler.HandleProfiles)
	s.mux.HandleFunc("/api/v1/profiles/", s.profileHandler.HandleProfiles)

	// System routes
	s.mux.HandleFunc("/api/v1/system/stats", s.systemHandler.HandleStats)
	s.mux.HandleFunc("/api/v1/system/settings", s.systemHandler.HandleSettings)

	// Frontend static assets
	if s.webFS != nil {
		fileServer := http.FileServer(http.FS(s.webFS))
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			fileServer.ServeHTTP(w, r)
		})
	}
}

func (s *Server) dispatchTaskSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	if path == "" {
		s.taskHandler.HandleListTasks(w, r)
		return
	}

	if strings.HasSuffix(path, "/start") && r.Method == http.MethodPost {
		s.taskHandler.HandleStartTask(w, r)
		return
	}
	if strings.HasSuffix(path, "/abort") && r.Method == http.MethodPost {
		s.taskHandler.HandleAbortTask(w, r)
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
	return OptionalAuthMiddleware(s.mux)
}
