//go:build !windows

package collector

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type linuxCollector struct {
	dataDir     string
	prevIdle    uint64
	prevTotal   uint64
	hasPrev     bool
	prevRxBytes uint64
	prevTxBytes uint64
	prevNetTime time.Time
	hasNetPrev  bool
}

func newPlatformCollector(dataDir string) platformCollector {
	return &linuxCollector{
		dataDir: dataDir,
	}
}

func (l *linuxCollector) getCPUUsage() float64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return 0
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}

	var times []uint64
	for _, field := range fields[1:] {
		val, _ := strconv.ParseUint(field, 10, 64)
		times = append(times, val)
	}

	if len(times) < 4 {
		return 0
	}

	idle := times[3]
	var iowait uint64
	if len(times) > 4 {
		iowait = times[4]
	}

	var total uint64
	for _, t := range times {
		total += t
	}
	idleAll := idle + iowait

	if !l.hasPrev {
		l.prevIdle = idleAll
		l.prevTotal = total
		l.hasPrev = true
		return 0
	}

	deltaIdle := idleAll - l.prevIdle
	deltaTotal := total - l.prevTotal

	l.prevIdle = idleAll
	l.prevTotal = total

	if deltaTotal == 0 {
		return 0
	}

	busy := deltaTotal - deltaIdle
	pct := (float64(busy) / float64(deltaTotal)) * 100.0
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}
	return round(pct, 1)
}

func (l *linuxCollector) getHostMemory() (totalGB, usedGB, freeGB, usagePct float64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, 0
	}
	defer f.Close()

	var memTotalKB, memFreeKB, memAvailKB, buffersKB, cachedKB uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valFields := strings.Fields(parts[1])
		if len(valFields) == 0 {
			continue
		}
		val, _ := strconv.ParseUint(valFields[0], 10, 64)

		switch key {
		case "MemTotal":
			memTotalKB = val
		case "MemFree":
			memFreeKB = val
		case "MemAvailable":
			memAvailKB = val
		case "Buffers":
			buffersKB = val
		case "Cached":
			cachedKB = val
		}
	}

	if memTotalKB == 0 {
		return 0, 0, 0, 0
	}

	total := float64(memTotalKB) / (1024 * 1024)
	var free float64
	if memAvailKB > 0 {
		free = float64(memAvailKB) / (1024 * 1024)
	} else {
		free = float64(memFreeKB+buffersKB+cachedKB) / (1024 * 1024)
	}
	used := total - free
	if used < 0 {
		used = 0
	}
	pct := (used / total) * 100.0
	return round(total, 2), round(used, 2), round(free, 2), round(pct, 1)
}

func (l *linuxCollector) getContainerMetrics(hostTotalGB float64) ContainerMetrics {
	cm := ContainerMetrics{
		IsContainer: isInsideContainer(),
	}

	// Try Cgroup v2 first
	if readCgroupV2(&cm, hostTotalGB) {
		return cm
	}

	// Fallback to Cgroup v1
	readCgroupV1(&cm, hostTotalGB)
	return cm
}

func isInsideContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") || strings.Contains(content, "containerd") || strings.Contains(content, "kubepods") {
			return true
		}
	}
	return false
}

func readCgroupV2(cm *ContainerMetrics, hostTotalGB float64) bool {
	maxBytesStr, err := readTrimmedFile("/sys/fs/cgroup/memory.max")
	if err != nil {
		return false
	}

	cm.CgroupVersion = "v2"
	var limitGB float64
	if maxBytesStr != "max" {
		if limitBytes, parseErr := strconv.ParseUint(maxBytesStr, 10, 64); parseErr == nil {
			limitGB = float64(limitBytes) / (1024 * 1024 * 1024)
			// If limit is practically larger than host, treat as unconstrained
			if hostTotalGB > 0 && limitGB > hostTotalGB*1.5 {
				limitGB = 0
			}
		}
	}
	cm.MemLimitGB = round(limitGB, 2)

	if curBytesStr, curErr := readTrimmedFile("/sys/fs/cgroup/memory.current"); curErr == nil {
		if curBytes, parseErr := strconv.ParseUint(curBytesStr, 10, 64); parseErr == nil {
			cm.MemUsedGB = round(float64(curBytes)/(1024*1024*1024), 2)
			if cm.MemLimitGB > 0 {
				cm.MemUsagePct = round((cm.MemUsedGB/cm.MemLimitGB)*100.0, 1)
			}
		}
	}

	if cpuMaxStr, cpuErr := readTrimmedFile("/sys/fs/cgroup/cpu.max"); cpuErr == nil {
		fields := strings.Fields(cpuMaxStr)
		if len(fields) >= 2 && fields[0] != "max" {
			quota, _ := strconv.ParseFloat(fields[0], 64)
			period, _ := strconv.ParseFloat(fields[1], 64)
			if period > 0 {
				cm.CPUQuota = round(quota/period, 1)
			}
		}
	}
	return true
}

func readCgroupV1(cm *ContainerMetrics, hostTotalGB float64) bool {
	limitStr, err := readTrimmedFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	if err != nil {
		return false
	}

	cm.CgroupVersion = "v1"
	if limitBytes, parseErr := strconv.ParseUint(limitStr, 10, 64); parseErr == nil {
		// In cgroups v1, 9223372036854771712 / 0x7FFFFFFFFFFFF000 represents no limit
		if limitBytes < 9000000000000000000 {
			limitGB := float64(limitBytes) / (1024 * 1024 * 1024)
			if hostTotalGB == 0 || limitGB <= hostTotalGB*1.5 {
				cm.MemLimitGB = round(limitGB, 2)
			}
		}
	}

	if curStr, curErr := readTrimmedFile("/sys/fs/cgroup/memory/memory.usage_in_bytes"); curErr == nil {
		if curBytes, parseErr := strconv.ParseUint(curStr, 10, 64); parseErr == nil {
			cm.MemUsedGB = round(float64(curBytes)/(1024*1024*1024), 2)
			if cm.MemLimitGB > 0 {
				cm.MemUsagePct = round((cm.MemUsedGB/cm.MemLimitGB)*100.0, 1)
			}
		}
	}

	if quotaStr, qErr := readTrimmedFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us"); qErr == nil {
		if periodStr, pErr := readTrimmedFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us"); pErr == nil {
			quota, _ := strconv.ParseFloat(quotaStr, 64)
			period, _ := strconv.ParseFloat(periodStr, 64)
			if quota > 0 && period > 0 {
				cm.CPUQuota = round(quota/period, 1)
			}
		}
	}
	return true
}

func readTrimmedFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (l *linuxCollector) getDiskSpace(path string) (totalGB, usedGB, freeGB, usagePct float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, 0
	}
	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	usedBytes := totalBytes - freeBytes

	total := float64(totalBytes) / (1024 * 1024 * 1024)
	free := float64(freeBytes) / (1024 * 1024 * 1024)
	used := float64(usedBytes) / (1024 * 1024 * 1024)

	pct := 0.0
	if total > 0 {
		pct = (used / total) * 100.0
	}
	return round(total, 1), round(used, 1), round(free, 1), round(pct, 1)
}

func (l *linuxCollector) getNetworkRates() (rxBps, txBps float64, rxFormatted, txFormatted string) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0, "0 B/s", "0 B/s"
	}
	defer f.Close()

	var totalRx, totalTx uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		totalRx += rx
		totalTx += tx
	}

	now := time.Now()
	if !l.hasNetPrev {
		l.prevRxBytes = totalRx
		l.prevTxBytes = totalTx
		l.prevNetTime = now
		l.hasNetPrev = true
		return 0, 0, "0 B/s", "0 B/s"
	}

	elapsed := now.Sub(l.prevNetTime).Seconds()
	l.prevNetTime = now

	if elapsed <= 0 {
		return 0, 0, "0 B/s", "0 B/s"
	}

	var deltaRx, deltaTx uint64
	if totalRx >= l.prevRxBytes {
		deltaRx = totalRx - l.prevRxBytes
	}
	if totalTx >= l.prevTxBytes {
		deltaTx = totalTx - l.prevTxBytes
	}
	l.prevRxBytes = totalRx
	l.prevTxBytes = totalTx

	rxBps = float64(deltaRx) / elapsed
	txBps = float64(deltaTx) / elapsed

	return round(rxBps, 1), round(txBps, 1), FormatSpeed(rxBps), FormatSpeed(txBps)
}

func (l *linuxCollector) Collect() (HostMetrics, ContainerMetrics, StorageMetrics) {
	cpuPct := l.getCPUUsage()
	memTotal, memUsed, memFree, memPct := l.getHostMemory()
	rxBps, txBps, rxFormatted, txFormatted := l.getNetworkRates()
	container := l.getContainerMetrics(memTotal)
	diskTotal, diskUsed, diskFree, diskPct := l.getDiskSpace(l.dataDir)

	host := HostMetrics{
		CPUUsagePct: cpuPct,
		MemTotalGB:  memTotal,
		MemUsedGB:   memUsed,
		MemFreeGB:   memFree,
		MemUsagePct: memPct,
		NetRxBps:    rxBps,
		NetTxBps:    txBps,
		NetRxRate:   rxFormatted,
		NetTxRate:   txFormatted,
	}

	storage := StorageMetrics{
		TotalGB:  diskTotal,
		UsedGB:   diskUsed,
		FreeGB:   diskFree,
		UsagePct: diskPct,
	}

	return host, container, storage
}
