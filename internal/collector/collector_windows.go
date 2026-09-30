//go:build windows

package collector

import (
	"syscall"
	"unsafe"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	getSystemTimes      = kernel32.NewProc("GetSystemTimes")
	globalMemoryStatus  = kernel32.NewProc("GlobalMemoryStatusEx")
	getDiskFreeSpaceExW = kernel32.NewProc("GetDiskFreeSpaceExW")
)

type memoryStatusEx struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

type windowsCollector struct {
	dataDir    string
	prevIdle   int64
	prevKernel int64
	prevUser   int64
	hasPrev    bool
}

func newPlatformCollector(dataDir string) platformCollector {
	return &windowsCollector{
		dataDir: dataDir,
	}
}

func fileTimeToTime(ft syscall.Filetime) int64 {
	return int64(ft.HighDateTime)<<32 + int64(ft.LowDateTime)
}

func (w *windowsCollector) getCPUUsage() float64 {
	var idle, kernel, user syscall.Filetime
	r, _, _ := getSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r == 0 {
		return 0
	}

	idleT := fileTimeToTime(idle)
	kernelT := fileTimeToTime(kernel)
	userT := fileTimeToTime(user)

	if !w.hasPrev {
		w.prevIdle = idleT
		w.prevKernel = kernelT
		w.prevUser = userT
		w.hasPrev = true
		return 0
	}

	deltaIdle := idleT - w.prevIdle
	deltaKernel := kernelT - w.prevKernel
	deltaUser := userT - w.prevUser

	w.prevIdle = idleT
	w.prevKernel = kernelT
	w.prevUser = userT

	deltaTotal := deltaKernel + deltaUser
	if deltaTotal <= 0 {
		return 0
	}

	// Windows kernel time includes idle time
	busy := deltaTotal - deltaIdle
	if busy < 0 {
		busy = 0
	}

	pct := (float64(busy) / float64(deltaTotal)) * 100.0
	if pct > 100.0 {
		pct = 100.0
	}
	return round(pct, 1)
}

func (w *windowsCollector) getMemory() (totalGB, usedGB, freeGB, usagePct float64) {
	var status memoryStatusEx
	status.dwLength = uint32(unsafe.Sizeof(status))
	r, _, _ := globalMemoryStatus.Call(uintptr(unsafe.Pointer(&status)))
	if r == 0 {
		return 0, 0, 0, 0
	}

	total := float64(status.ullTotalPhys) / (1024 * 1024 * 1024)
	free := float64(status.ullAvailPhys) / (1024 * 1024 * 1024)
	used := total - free
	pct := float64(status.dwMemoryLoad)
	if total > 0 && pct == 0 {
		pct = (used / total) * 100.0
	}
	return round(total, 2), round(used, 2), round(free, 2), round(pct, 1)
}

func (w *windowsCollector) getDiskSpace(path string) (totalGB, usedGB, freeGB, usagePct float64) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, 0, 0
	}

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes int64
	r, _, _ := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if r == 0 {
		return 0, 0, 0, 0
	}

	total := float64(totalNumberOfBytes) / (1024 * 1024 * 1024)
	free := float64(freeBytesAvailable) / (1024 * 1024 * 1024)
	used := total - free
	pct := 0.0
	if total > 0 {
		pct = (used / total) * 100.0
	}
	return round(total, 1), round(used, 1), round(free, 1), round(pct, 1)
}

func (w *windowsCollector) Collect() (HostMetrics, ContainerMetrics, StorageMetrics) {
	cpuPct := w.getCPUUsage()
	memTotal, memUsed, memFree, memPct := w.getMemory()
	diskTotal, diskUsed, diskFree, diskPct := w.getDiskSpace(w.dataDir)

	host := HostMetrics{
		CPUUsagePct: cpuPct,
		MemTotalGB:  memTotal,
		MemUsedGB:   memUsed,
		MemFreeGB:   memFree,
		MemUsagePct: memPct,
		NetRxBps:    0,
		NetTxBps:    0,
		NetRxRate:   "0 B/s",
		NetTxRate:   "0 B/s",
	}

	container := ContainerMetrics{
		IsContainer:   false,
		CgroupVersion: "",
		MemLimitGB:    0,
		MemUsedGB:     0,
		MemUsagePct:   0,
		CPUQuota:      0,
	}

	storage := StorageMetrics{
		TotalGB:  diskTotal,
		UsedGB:   diskUsed,
		FreeGB:   diskFree,
		UsagePct: diskPct,
	}

	return host, container, storage
}
