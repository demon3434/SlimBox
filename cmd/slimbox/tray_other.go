//go:build !windows

package main

import "os"

func startTray(port int, dataDir string, stopCh chan os.Signal) error {
	// No system tray on non-Windows platforms
	return nil
}

func openWebPage(port int) {}
