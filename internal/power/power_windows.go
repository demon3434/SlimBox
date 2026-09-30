//go:build windows

package power

import (
	"log"
	"sync"
	"syscall"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	pSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")

	mu             sync.Mutex
	sleepLockCount int
)

const (
	esSystemRequired   = 0x00000001
	esAwaymodeRequired = 0x00000040
	esContinuous       = 0x80000000
)

// PreventSleep requests the Windows power manager to keep the system awake during active operations.
func PreventSleep(reason string) {
	mu.Lock()
	defer mu.Unlock()

	sleepLockCount++
	if sleepLockCount == 1 {
		// Prevent system idle sleep while allowing display to turn off (away mode)
		ret, _, _ := pSetThreadExecutionState.Call(uintptr(esContinuous | esSystemRequired | esAwaymodeRequired))
		if ret == 0 {
			// Away mode may not be supported on all Windows editions, fallback to standard system required
			pSetThreadExecutionState.Call(uintptr(esContinuous | esSystemRequired))
		}
		log.Printf("[Power] Windows idle sleep prevented (%s)", reason)
	}
}

// AllowSleep clears the sleep prevention request once transcoding tasks finish.
func AllowSleep() {
	mu.Lock()
	defer mu.Unlock()

	if sleepLockCount > 0 {
		sleepLockCount--
	}
	if sleepLockCount == 0 {
		pSetThreadExecutionState.Call(uintptr(esContinuous))
		log.Println("[Power] Windows sleep restriction cleared. Normal power savings restored.")
	}
}
