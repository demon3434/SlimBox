package collector

import (
	"runtime"
	"strconv"
	"sync"
	"time"
)

type HostMetrics struct {
	CPUUsagePct   float64 `json:"cpu_usage_pct"`
	NumCPU        int     `json:"num_cpu"`
	MemTotalGB    float64 `json:"mem_total_gb"`
	MemUsedGB     float64 `json:"mem_used_gb"`
	MemFreeGB     float64 `json:"mem_free_gb"`
	MemUsagePct   float64 `json:"mem_usage_pct"`
	NetRxBps      float64 `json:"net_rx_bps"`
	NetTxBps      float64 `json:"net_tx_bps"`
	NetRxRate     string  `json:"net_rx_rate"`
	NetTxRate     string  `json:"net_tx_rate"`
	UptimeSeconds int64   `json:"uptime_seconds"`
}

type ContainerMetrics struct {
	IsContainer    bool    `json:"is_container"`
	CgroupVersion  string  `json:"cgroup_version"`
	MemLimitGB     float64 `json:"mem_limit_gb"`
	MemUsedGB      float64 `json:"mem_used_gb"`
	MemUsagePct    float64 `json:"mem_usage_pct"`
	CPUQuota       float64 `json:"cpu_quota"`
	ProcessAllocMB float64 `json:"process_alloc_mb"`
	ProcessSysMB   float64 `json:"process_sys_mb"`
	Goroutines     int     `json:"goroutines"`
}

type StorageMetrics struct {
	Path     string  `json:"path"`
	TotalGB  float64 `json:"total_gb"`
	UsedGB   float64 `json:"used_gb"`
	FreeGB   float64 `json:"free_gb"`
	UsagePct float64 `json:"usage_pct"`
}

type Snapshot struct {
	Host      HostMetrics      `json:"host"`
	Container ContainerMetrics `json:"container"`
	Storage   StorageMetrics   `json:"storage"`
}

type MetricsCollector struct {
	dataDir   string
	startTime time.Time

	mu        sync.RWMutex
	current   Snapshot
	platform  platformCollector
	stopCh    chan struct{}
	closeOnce sync.Once
}

func NewMetricsCollector(dataDir string) *MetricsCollector {
	c := &MetricsCollector{
		dataDir:   dataDir,
		startTime: time.Now(),
		stopCh:    make(chan struct{}),
	}
	c.platform = newPlatformCollector(dataDir)
	// Initial synchronous sample
	c.sample()
	return c
}

func (c *MetricsCollector) Start() {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.sample()
			case <-c.stopCh:
				return
			}
		}
	}()
}

func (c *MetricsCollector) Stop() {
	c.closeOnce.Do(func() {
		close(c.stopCh)
	})
}

func (c *MetricsCollector) GetSnapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

func (c *MetricsCollector) sample() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	host, container, storage := c.platform.Collect()

	host.NumCPU = runtime.NumCPU()
	host.UptimeSeconds = int64(time.Since(c.startTime).Seconds())

	container.ProcessAllocMB = round(float64(m.Alloc)/(1024*1024), 2)
	container.ProcessSysMB = round(float64(m.Sys)/(1024*1024), 2)
	container.Goroutines = runtime.NumGoroutine()

	storage.Path = c.dataDir

	c.mu.Lock()
	c.current = Snapshot{
		Host:      host,
		Container: container,
		Storage:   storage,
	}
	c.mu.Unlock()
}

func round(val float64, decimals int) float64 {
	pow := 1.0
	for i := 0; i < decimals; i++ {
		pow *= 10.0
	}
	return float64(int(val*pow+0.5)) / pow
}

func FormatSpeed(bytesPerSec float64) string {
	if bytesPerSec <= 0 {
		return "0 B/s"
	}
	if bytesPerSec < 1024 {
		return strconv.FormatFloat(bytesPerSec, 'f', 0, 64) + " B/s"
	} else if bytesPerSec < 1024*1024 {
		return strconv.FormatFloat(bytesPerSec/1024.0, 'f', 1, 64) + " KB/s"
	} else if bytesPerSec < 1024*1024*1024 {
		return strconv.FormatFloat(bytesPerSec/(1024.0*1024.0), 'f', 1, 64) + " MB/s"
	}
	return strconv.FormatFloat(bytesPerSec/(1024.0*1024.0*1024.0), 'f', 2, 64) + " GB/s"
}
