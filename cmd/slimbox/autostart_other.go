//go:build !windows

package main

func isAutoStartEnabled() bool {
	return false
}

func setAutoStartEnabled(enable bool) error {
	return nil
}
