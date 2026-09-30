//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func defaultDataDir(userHome string) string {
	return filepath.Join(userHome, "Movies", "SlimBox")
}

func findServerBinary(execDir string) string {
	slimboxBin := filepath.Join(execDir, "slimbox")
	if _, err := exec.LookPath(slimboxBin); err == nil {
		return slimboxBin
	}
	// Check inside .app bundle Resources
	resBin := filepath.Join(execDir, "..", "Resources", "slimbox")
	if _, err := exec.LookPath(resBin); err == nil {
		return resBin
	}
	return "slimbox"
}

func openBrowser(url string) error {
	return exec.Command("open", url).Start()
}

func promptFolderSelection() string {
	script := `POSIX path of (choose folder with prompt "【SlimBox 初次设置】请选择视频暂存与转码输出目录 (建议选择外接 USB 硬盘以保护内置 SSD 寿命):")`
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func notifySuccess(msg string) {
	script := fmt.Sprintf(`display notification "%s" with title "SlimBox" subtitle "转码服务已启动"`, msg)
	_ = exec.Command("osascript", "-e", script).Start()
}

func notifyError(msg string) {
	script := fmt.Sprintf(`display alert "SlimBox 启动失败" message "%s" as critical`, msg)
	_ = exec.Command("osascript", "-e", script).Run()
}

func prepareChildCmd(cmd *exec.Cmd) *exec.Cmd {
	return cmd
}
