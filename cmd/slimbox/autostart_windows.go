//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const runKeyName = "SlimBox"

func isAutoStartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	val, _, err := k.GetStringValue(runKeyName)
	if err != nil {
		return false
	}
	return strings.TrimSpace(val) != ""
}

func setAutoStartEnabled(enable bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if enable {
		exePath, err := os.Executable()
		if err != nil {
			return err
		}
		absExe, err := filepath.Abs(exePath)
		if err != nil {
			absExe = exePath
		}
		regVal := `"` + absExe + `"`
		return k.SetStringValue(runKeyName, regVal)
	}

	_ = k.DeleteValue(runKeyName)
	return nil
}
