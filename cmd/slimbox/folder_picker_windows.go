//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	ole32                  = syscall.NewLazyDLL("ole32.dll")
	pSHBrowseForFolderW    = shell32.NewProc("SHBrowseForFolderW")
	pSHGetPathFromIDListW  = shell32.NewProc("SHGetPathFromIDListW")
	pCoTaskMemFree         = ole32.NewProc("CoTaskMemFree")
	pCoInitializeEx        = ole32.NewProc("CoInitializeEx")
	pCoUninitialize        = ole32.NewProc("CoUninitialize")
)

const (
	bifReturnOnlyFsDirs = 0x00000001
	bifNewDialogStyle   = 0x00000040
	coinitAparmentThreaded = 0x2
)

type browseInfoW struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

// pickFolderDialog opens the native Windows Folder Browser dialog
func pickFolderDialog(hwnd uintptr, title string) (string, error) {
	// Initialize COM for the thread
	pCoInitializeEx.Call(0, coinitAparmentThreaded)
	defer pCoUninitialize.Call()

	titlePtr, _ := syscall.UTF16PtrFromString(title)
	var displayName [syscall.MAX_PATH]uint16

	bi := browseInfoW{
		hwndOwner:      hwnd,
		pidlRoot:       0,
		pszDisplayName: &displayName[0],
		lpszTitle:      titlePtr,
		ulFlags:        bifReturnOnlyFsDirs | bifNewDialogStyle,
	}

	pidl, _, _ := pSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", nil // User canceled
	}
	defer pCoTaskMemFree.Call(pidl)

	var path [syscall.MAX_PATH]uint16
	ret, _, _ := pSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&path[0])))
	if ret == 0 {
		return "", nil
	}

	return syscall.UTF16ToString(path[:]), nil
}
