//go:build windows

package engine

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// PrepareCmd sets Windows process creation flags to hide child console windows.
func PrepareCmd(cmd *exec.Cmd) *exec.Cmd {
	if cmd == nil {
		return nil
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
	return cmd
}
