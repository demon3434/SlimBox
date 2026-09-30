package engine

import (
	"sync"
)

// GPUDevice represents an enumerated graphics accelerator adapter.
type GPUDevice struct {
	ID          string   `json:"id"`          // Unique identifier, e.g. "renderD128", "win_gpu_0", "videotoolbox"
	Name        string   `json:"name"`        // Human readable name, e.g. "Intel(R) UHD Graphics 770"
	Vendor      string   `json:"vendor"`      // "intel", "amd", "nvidia", "apple", or "unknown"
	Type        string   `json:"type"`        // Accelerator family: "vaapi", "nvenc", "qsv", "amf", "videotoolbox"
	DevicePath  string   `json:"device_path"` // Device node or index, e.g. "/dev/dri/renderD128" or "0"
	Operational bool     `json:"operational"` // Whether dry-run probe succeeded
	IsSelected  bool     `json:"is_selected"` // Whether this device is currently chosen
	Codecs      []string `json:"codecs"`      // Supported hardware codecs, e.g. ["hevc_vaapi", "h264_vaapi"]
}

// GPUConfig represents the persisted user preference and active GPU state.
type GPUConfig struct {
	PreferredGPU     string      `json:"preferred_gpu"`     // "auto", "cpu_only", or specific device ID
	ActiveGPU        *GPUDevice  `json:"active_gpu,omitempty"`
	EffectiveEncoder string      `json:"effective_encoder"` // e.g. "hevc_vaapi", "hevc_nvenc", "libx265"
	Devices          []GPUDevice `json:"devices"`
}

var (
	gpuConfigMu   sync.RWMutex
	cachedDevices []GPUDevice
	probing       bool
	preferredGPU  = "auto" // Default to auto-detect
)

func init() {
	go RefreshGPUDevices()
}

// RefreshGPUDevices initiates GPU probe in background or foreground.
func RefreshGPUDevices() []GPUDevice {
	gpuConfigMu.Lock()
	if probing {
		devices := cachedDevices
		gpuConfigMu.Unlock()
		return devices
	}
	probing = true
	gpuConfigMu.Unlock()

	discovered := EnumeratePlatformGPUs(FindFFmpeg())
	if discovered == nil {
		discovered = make([]GPUDevice, 0)
	}

	gpuConfigMu.Lock()
	cachedDevices = discovered
	probing = false
	gpuConfigMu.Unlock()

	return discovered
}

// GetPreferredGPU returns the currently configured preferred GPU identifier.
func GetPreferredGPU() string {
	gpuConfigMu.RLock()
	defer gpuConfigMu.RUnlock()
	return preferredGPU
}

// SetPreferredGPU updates the user's preferred GPU identifier.
func SetPreferredGPU(pref string) {
	gpuConfigMu.Lock()
	defer gpuConfigMu.Unlock()
	if pref == "" {
		pref = "auto"
	}
	preferredGPU = pref
}

// GetCachedGPUDevices returns all enumerated GPU devices without blocking.
func GetCachedGPUDevices() []GPUDevice {
	gpuConfigMu.RLock()
	devices := cachedDevices
	gpuConfigMu.RUnlock()

	if devices == nil {
		go RefreshGPUDevices()
		return make([]GPUDevice, 0)
	}
	return devices
}

// InvalidateGPUCache triggers a background re-probe.
func InvalidateGPUCache() {
	go RefreshGPUDevices()
}

// GetCurrentGPUConfig builds the full GPUConfig payload for API and worker dispatch.
func GetCurrentGPUConfig() GPUConfig {
	devices := GetCachedGPUDevices()
	pref := GetPreferredGPU()

	cfg := GPUConfig{
		PreferredGPU:     pref,
		Devices:          make([]GPUDevice, len(devices)),
		EffectiveEncoder: "libx265",
	}

	copy(cfg.Devices, devices)

	if pref == "cpu_only" {
		cfg.EffectiveEncoder = "libx265"
		return cfg
	}

	// Match preferred device
	var chosen *GPUDevice
	if pref != "auto" {
		for i := range cfg.Devices {
			if cfg.Devices[i].ID == pref && cfg.Devices[i].Operational {
				chosen = &cfg.Devices[i]
				cfg.Devices[i].IsSelected = true
				break
			}
		}
	}

	// Auto fallback: pick first operational GPU by priority
	if chosen == nil {
		for i := range cfg.Devices {
			if cfg.Devices[i].Operational {
				chosen = &cfg.Devices[i]
				cfg.Devices[i].IsSelected = true
				break
			}
		}
	}

	if chosen != nil {
		cfg.ActiveGPU = chosen
		if len(chosen.Codecs) > 0 {
			cfg.EffectiveEncoder = chosen.Codecs[0]
		}
	}

	return cfg
}
