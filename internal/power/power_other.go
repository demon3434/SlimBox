//go:build !windows

package power

// PreventSleep is a no-op on non-Windows platforms.
func PreventSleep(reason string) {}

// AllowSleep is a no-op on non-Windows platforms.
func AllowSleep() {}
