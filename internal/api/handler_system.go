package api

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"slimbox/internal/domain"
	"slimbox/internal/repository"
)

type SystemHandler struct {
	settingsRepo *repository.SettingsRepository
	dataDir      string
	startTime    time.Time
}

func NewSystemHandler(settingsRepo *repository.SettingsRepository, dataDir string) *SystemHandler {
	return &SystemHandler{
		settingsRepo: settingsRepo,
		dataDir:      dataDir,
		startTime:    time.Now(),
	}
}

type SystemStatsResponse struct {
	UptimeSeconds int64   `json:"uptime_seconds"`
	NumCPU        int     `json:"num_cpu"`
	GoRoutines    int     `json:"goroutines"`
	MemoryAllocMB float64 `json:"memory_alloc_mb"`
	MemorySysMB   float64 `json:"memory_sys_mb"`
	DiskTotalGB   float64 `json:"disk_total_gb"`
	DiskFreeGB    float64 `json:"disk_free_gb"`
	DiskUsedGB    float64 `json:"disk_used_gb"`
	DiskUsagePct  float64 `json:"disk_usage_pct"`
}

func (h *SystemHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	totalGB, freeGB := getDiskSpace(h.dataDir)
	usedGB := totalGB - freeGB
	usagePct := 0.0
	if totalGB > 0 {
		usagePct = (usedGB / totalGB) * 100.0
	}

	stats := SystemStatsResponse{
		UptimeSeconds: int64(time.Since(h.startTime).Seconds()),
		NumCPU:        runtime.NumCPU(),
		GoRoutines:    runtime.NumGoroutine(),
		MemoryAllocMB: float64(m.Alloc) / 1024.0 / 1024.0,
		MemorySysMB:   float64(m.Sys) / 1024.0 / 1024.0,
		DiskTotalGB:   round(totalGB, 1),
		DiskFreeGB:    round(freeGB, 1),
		DiskUsedGB:    round(usedGB, 1),
		DiskUsagePct:  round(usagePct, 1),
	}

	WriteJSON(w, http.StatusOK, stats)
}

func (h *SystemHandler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s, err := h.settingsRepo.GetStorageSettings()
		if err != nil {
			WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, s)

	case http.MethodPut, http.MethodPost:
		var s domain.StorageSettings
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			WriteJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		if err := h.settingsRepo.SaveStorageSettings(s); err != nil {
			WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success":  true,
			"settings": s,
		})

	default:
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func round(val float64, decimals int) float64 {
	pow := 1.0
	for i := 0; i < decimals; i++ {
		pow *= 10.0
	}
	return float64(int(val*pow+0.5)) / pow
}
