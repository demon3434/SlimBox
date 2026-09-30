//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type slimboxWindowsService struct {
	stopFunc func()
}

func (s *slimboxWindowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	// SCM requires reporting Running quickly to avoid service startup timeout
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	log.Println("[SlimBox Service] Windows service started and entered running state.")

	for req := range r {
		switch req.Cmd {
		case svc.Interrogate:
			changes <- req.CurrentStatus
		case svc.Stop, svc.Shutdown:
			log.Println("[SlimBox Service] Windows service stop/shutdown signal received...")
			changes <- svc.Status{State: svc.StopPending}
			if s.stopFunc != nil {
				s.stopFunc()
			}
			return false, 0
		default:
			log.Printf("[SlimBox Service] Unexpected service control request #%d", req.Cmd)
		}
	}
	return false, 0
}

func isWindowsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}

func runWindowsService(serviceName string, stopFunc func()) error {
	s := &slimboxWindowsService{stopFunc: stopFunc}
	return svc.Run(serviceName, s)
}

func isServiceInstalled() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()

	s, err := m.OpenService("SlimBox")
	if err != nil {
		return false
	}
	s.Close()
	return true
}

func isServiceRunning() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()

	s, err := m.OpenService("SlimBox")
	if err != nil {
		return false
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return false
	}
	return status.State == svc.Running
}

func installService(port int, dataDir string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("无法连接到 Windows 服务控制管理器 (需管理员权限): %w", err)
	}
	defer m.Disconnect()

	// If already exists, cleanly remove first
	s, err := m.OpenService("SlimBox")
	if err == nil {
		s.Close()
		_ = uninstallService()
	}

	args := []string{fmt.Sprintf("--port=%d", port)}
	if dataDir != "" {
		args = append(args, fmt.Sprintf("--data-dir=%s", dataDir))
	}

	s, err = m.CreateService("SlimBox", exePath, mgr.Config{
		StartType:   mgr.StartAutomatic,
		DisplayName: "SlimBox Video Server",
		Description: "SlimBox 局域网硬件加速视频压制微服务 (24小时待命转码中心)",
	}, args...)
	if err != nil {
		return fmt.Errorf("创建系统服务失败: %w", err)
	}
	defer s.Close()

	// Automatically configure Windows Defender Firewall rule for port 8080
	_ = runHiddenCmd("netsh", "advfirewall", "firewall", "delete", "rule", "name=SlimBox Video Server")
	_ = runHiddenCmd("netsh", "advfirewall", "firewall", "add", "rule",
		"name=SlimBox Video Server", "dir=in", "action=allow", "protocol=TCP",
		fmt.Sprintf("localport=%d", port), "profile=private,domain")

	_ = s.Start()
	return nil
}

func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("无法连接到 Windows 服务控制管理器 (需管理员权限): %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService("SlimBox")
	if err != nil {
		return nil // Service not installed
	}
	defer s.Close()

	_, _ = s.Control(svc.Stop)
	time.Sleep(500 * time.Millisecond)

	if err := s.Delete(); err != nil {
		return fmt.Errorf("删除系统服务失败: %w", err)
	}

	_ = runHiddenCmd("netsh", "advfirewall", "firewall", "delete", "rule", "name=SlimBox Video Server")
	return nil
}

func stopWindowsService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService("SlimBox")
	if err != nil {
		return err
	}
	defer s.Close()

	_, err = s.Control(svc.Stop)
	return err
}

func runHiddenCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Run()
}

func showMsgBox(title, msg string, isError bool) {
	user32 := syscall.NewLazyDLL("user32.dll")
	msgBox := user32.NewProc("MessageBoxW")
	tPtr, _ := syscall.UTF16PtrFromString(title)
	mPtr, _ := syscall.UTF16PtrFromString(msg)
	var flag uintptr = 0 // MB_OK
	if isError {
		flag = 0x10 // MB_ICONERROR
	} else {
		flag = 0x40 // MB_ICONINFORMATION
	}
	msgBox.Call(0, uintptr(unsafe.Pointer(mPtr)), uintptr(unsafe.Pointer(tPtr)), flag)
}
