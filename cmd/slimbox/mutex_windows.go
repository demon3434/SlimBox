//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	pCreateMutexW = kernel32.NewProc("CreateMutexW")
	pCloseHandle  = kernel32.NewProc("CloseHandle")
)

const (
	errorAlreadyExists = 183
)

// acquireSingleInstanceMutex creates a named Win32 mutex in the local session namespace.
// If the mutex already exists, it means another instance is already running.
func acquireSingleInstanceMutex() (alreadyRunning bool, release func()) {
	name, _ := syscall.UTF16PtrFromString("Local\\SlimBox_SingleInstance_Mutex")
	handle, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return false, func() {}
	}

	if errno, ok := err.(syscall.Errno); ok && errno == errorAlreadyExists {
		pCloseHandle.Call(handle)
		return true, func() {}
	}

	return false, func() {
		pCloseHandle.Call(handle)
	}
}
