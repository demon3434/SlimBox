//go:build darwin

package collector

import (
	"math"
	"runtime"
	"syscall"
)

type darwinCollector struct {
	dataDir string
}

func newPlatformCollector(dataDir string) platformCollector {
	return &darwinCollector{
		dataDir: dataDir,
	}
}

func roundDarwin(val float64, precision int) float64 {
	p := math.Pow(10, float64(precision))
	return math.Round(val*p) / p
}

func (d *darwinCollector) getDiskSpace(path string) (total, used, free, pct float64) {
	if path == "" {
		path = "."
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, 0
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	t := float64(totalBytes) / (1024 * 1024 * 1024)
	f := float64(freeBytes) / (1024 * 1024 * 1024)
	u := t - f
	p := 0.0
	if t > 0 {
		p = (u / t) * 100.0
	}
	return roundDarwin(t, 1), roundDarwin(u, 1), roundDarwin(f, 1), roundDarwin(p, 1)
}

func (d *darwinCollector) Collect() (HostMetrics, ContainerMetrics, StorageMetrics) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	procAllocMB := roundDarwin(float64(m.Alloc)/(1024*1024), 1)
	procSysMB := roundDarwin(float64(m.Sys)/(1024*1024), 1)

	diskTotal, diskUsed, diskFree, diskPct := d.getDiskSpace(d.dataDir)

	host := HostMetrics{
		CPUUsagePct:   0,
		NumCPU:        runtime.NumCPU(),
		MemTotalGB:    0,
		MemUsedGB:     roundDarwin(float64(m.Sys)/(1024*1024*1024), 2),
		MemFreeGB:     0,
		MemUsagePct:   0,
		NetRxBps:      0,
		NetTxBps:      0,
		NetRxRate:     "0 B/s",
		NetTxRate:     "0 B/s",
		UptimeSeconds: 0,
	}

	container := ContainerMetrics{
		IsContainer:    false,
		CgroupVersion:  "",
		MemLimitGB:     0,
		MemUsedGB:      roundDarwin(float64(m.Alloc)/(1024*1024*1024), 2),
		MemUsagePct:    0,
		CPUQuota:       0,
		ProcessAllocMB: procAllocMB,
		ProcessSysMB:   procSysMB,
		Goroutines:     runtime.NumGoroutine(),
	}

	storage := StorageMetrics{
		Path:     d.dataDir,
		TotalGB:  diskTotal,
		UsedGB:   diskUsed,
		FreeGB:   diskFree,
		UsagePct: diskPct,
	}

	return host, container, storage
}
