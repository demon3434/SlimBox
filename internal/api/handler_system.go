package api

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"slimbox/internal/collector"
	"slimbox/internal/domain"
	"slimbox/internal/engine"
	"slimbox/internal/repository"
)

type SystemHandler struct {
	settingsRepo *repository.SettingsRepository
	dataDir      string
	collector    *collector.MetricsCollector
	startTime    time.Time
}

func NewSystemHandler(settingsRepo *repository.SettingsRepository, dataDir string, col *collector.MetricsCollector) *SystemHandler {
	if pref, err := settingsRepo.GetGPUSettings(); err == nil && pref != "" {
		engine.SetPreferredGPU(pref)
	}
	return &SystemHandler{
		settingsRepo: settingsRepo,
		dataDir:      dataDir,
		collector:    col,
		startTime:    time.Now(),
	}
}

type SystemStatsResponse struct {
	// Top-level fields for backwards compatibility
	UptimeSeconds int64   `json:"uptime_seconds"`
	NumCPU        int     `json:"num_cpu"`
	GoRoutines    int     `json:"goroutines"`
	MemoryAllocMB float64 `json:"memory_alloc_mb"`
	MemorySysMB   float64 `json:"memory_sys_mb"`
	DiskTotalGB   float64 `json:"disk_total_gb"`
	DiskFreeGB    float64 `json:"disk_free_gb"`
	DiskUsedGB    float64 `json:"disk_used_gb"`
	DiskUsagePct  float64 `json:"disk_usage_pct"`

	// Structured host, container, and storage statistics
	Host      collector.HostMetrics      `json:"host"`
	Container collector.ContainerMetrics `json:"container"`
	Storage   collector.StorageMetrics   `json:"storage"`

	// Hardware capabilities and network info
	HardwareAccelerators []string           `json:"hardware_accelerators"`
	HostIPs              []string           `json:"host_ips"`
	GPUs                 []engine.GPUDevice `json:"gpus"`
	SelectedGPU          string             `json:"selected_gpu"`
}

func (h *SystemHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	snap := h.collector.GetSnapshot()
	gpuCfg := engine.GetCurrentGPUConfig()
	effectiveAccels := engine.GetCachedAccelerators()
	if gpuCfg.PreferredGPU == "cpu_only" {
		effectiveAccels = []string{}
	} else if gpuCfg.ActiveGPU != nil {
		if gpuCfg.ActiveGPU.Vendor == "intel" && gpuCfg.ActiveGPU.Type == "vaapi" {
			effectiveAccels = []string{"vaapi_intel"}
		} else if gpuCfg.ActiveGPU.Vendor == "amd" && gpuCfg.ActiveGPU.Type == "vaapi" {
			effectiveAccels = []string{"vaapi_amd"}
		} else if gpuCfg.ActiveGPU.Type != "" {
			effectiveAccels = []string{gpuCfg.ActiveGPU.Type}
		}
	}

	stats := SystemStatsResponse{
		UptimeSeconds: snap.Host.UptimeSeconds,
		NumCPU:        snap.Host.NumCPU,
		GoRoutines:    snap.Container.Goroutines,
		MemoryAllocMB: snap.Container.ProcessAllocMB,
		MemorySysMB:   snap.Container.ProcessSysMB,
		DiskTotalGB:   snap.Storage.TotalGB,
		DiskFreeGB:    snap.Storage.FreeGB,
		DiskUsedGB:    snap.Storage.UsedGB,
		DiskUsagePct:  snap.Storage.UsagePct,

		Host:      snap.Host,
		Container: snap.Container,
		Storage:   snap.Storage,

		HardwareAccelerators: effectiveAccels,
		HostIPs:              getHostIPv4s(),
		GPUs:                 gpuCfg.Devices,
		SelectedGPU:          gpuCfg.PreferredGPU,
	}

	WriteJSON(w, http.StatusOK, stats)
}

func (h *SystemHandler) HandleGPUConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := engine.GetCurrentGPUConfig()
		WriteJSON(w, http.StatusOK, cfg)

	case http.MethodPut, http.MethodPost:
		var req struct {
			PreferredGPU string `json:"preferred_gpu"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}

		if err := h.settingsRepo.SaveGPUSettings(req.PreferredGPU); err != nil {
			WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		engine.SetPreferredGPU(req.PreferredGPU)
		cfg := engine.GetCurrentGPUConfig()
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"config":  cfg,
		})

	default:
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
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
		if s.ChunkThresholdMB <= 0 {
			s.ChunkThresholdMB = 200
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

func getHostIPv4s() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ips = append(ips, ipNet.IP.String())
			}
		}
	}
	return ips
}

