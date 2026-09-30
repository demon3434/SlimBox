//go:build !windows

package main

func acquireSingleInstanceMutex() (alreadyRunning bool, release func()) {
	return false, func() {}
}
