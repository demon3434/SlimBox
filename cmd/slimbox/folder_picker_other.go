//go:build !windows

package main

func pickFolderDialog(hwnd uintptr, title string) (string, error) {
	return "", nil
}
