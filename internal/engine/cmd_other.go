//go:build !windows

package engine

import "os/exec"

// PrepareCmd is a no-op on non-Windows platforms.
func PrepareCmd(cmd *exec.Cmd) *exec.Cmd {
	return cmd
}
