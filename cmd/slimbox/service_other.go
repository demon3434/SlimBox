//go:build !windows

package main

import "fmt"

func isWindowsService() bool {
	return false
}

func runWindowsService(serviceName string, stopFunc func()) error {
	return nil
}

func isServiceInstalled() bool {
	return false
}

func isServiceRunning() bool {
	return false
}

func installService(port int, dataDir string) error {
	return fmt.Errorf("Windows service installation is only supported on Windows")
}

func uninstallService() error {
	return fmt.Errorf("Windows service uninstallation is only supported on Windows")
}

func stopWindowsService() error {
	return nil
}

func showMsgBox(title, msg string, isError bool) {
	fmt.Printf("[%s] %s\n", title, msg)
}
