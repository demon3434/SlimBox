//go:build windows

package main

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

//go:embed logo.ico
var logoIcoBytes []byte

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pRegisterClassExW          = user32.NewProc("RegisterClassExW")
	pCreateWindowExW            = user32.NewProc("CreateWindowExW")
	pDefWindowProcW            = user32.NewProc("DefWindowProcW")
	pGetMessageW               = user32.NewProc("GetMessageW")
	pTranslateMessage          = user32.NewProc("TranslateMessage")
	pDispatchMessageW          = user32.NewProc("DispatchMessageW")
	pPostQuitMessage           = user32.NewProc("PostQuitMessage")
	pPostMessageW              = user32.NewProc("PostMessageW")
	pCreatePopupMenu           = user32.NewProc("CreatePopupMenu")
	pAppendMenuW               = user32.NewProc("AppendMenuW")
	pTrackPopupMenu            = user32.NewProc("TrackPopupMenu")
	pDestroyMenu               = user32.NewProc("DestroyMenu")
	pGetCursorPos              = user32.NewProc("GetCursorPos")
	pSetForegroundWindow       = user32.NewProc("SetForegroundWindow")
	pLoadIconW                 = user32.NewProc("LoadIconW")
	pCreateIconFromResourceEx  = user32.NewProc("CreateIconFromResourceEx")

	pShell_NotifyIconW         = shell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW             = shell32.NewProc("ShellExecuteW")
	pGetModuleHandleW          = kernel32.NewProc("GetModuleHandleW")
	pSetMenuItemInfoW          = user32.NewProc("SetMenuItemInfoW")
)

const (
	miimBitmap = 0x00000080
)

type menuItemInfoW struct {
	cbSize        uint32
	fMask         uint32
	fType         uint32
	fState        uint32
	wID           uint32
	hSubMenu      uintptr
	hbmpChecked   uintptr
	hbmpUnchecked uintptr
	dwItemData    uintptr
	dwTypeData    *uint16
	cch           uint32
	hbmpItem      uintptr
}

func loadLogoIcon() uintptr {
	// 1. Attempt to load from PE resources (compiled via syso)
	hInst, _, _ := pGetModuleHandleW.Call(0)
	hIcon, _, _ := pLoadIconW.Call(hInst, uintptr(1))
	if hIcon != 0 {
		return hIcon
	}

	// 2. Fallback: Parse embedded logoIcoBytes (32x32 entry)
	if len(logoIcoBytes) > 22 {
		entryOffset := 6 + 1*16 // 2nd entry (32x32)
		bytesInRes := binary.LittleEndian.Uint32(logoIcoBytes[entryOffset+8 : entryOffset+12])
		imgOffset := binary.LittleEndian.Uint32(logoIcoBytes[entryOffset+12 : entryOffset+16])
		if int(imgOffset+bytesInRes) <= len(logoIcoBytes) {
			bits := logoIcoBytes[imgOffset : imgOffset+bytesInRes]
			resIcon, _, _ := pCreateIconFromResourceEx.Call(
				uintptr(unsafe.Pointer(&bits[0])),
				uintptr(len(bits)),
				1,          // TRUE = icon
				0x00030000, // Version 3.0
				32, 32,
				0,
			)
			if resIcon != 0 {
				return resIcon
			}
		}
	}

	// 3. Fallback: system application icon
	defaultIcon, _, _ := pLoadIconW.Call(0, uintptr(32512))
	return defaultIcon
}

const (
	wmUser        = 0x0400
	wmTrayIcon    = wmUser + 101
	wmLButtonUp   = 0x0202
	wmLButtonDbl  = 0x0203
	wmRButtonUp   = 0x0205
	wmCommand     = 0x0111
	wmDestroy     = 0x0002

	nimAdd        = 0x00000000
	nimModify     = 0x00000001
	nimDelete     = 0x00000002

	nifMessage    = 0x00000001
	nifIcon       = 0x00000002
	nifTip        = 0x00000004
	nifInfo       = 0x00000010

	mfString      = 0x00000000
	mfSeparator   = 0x00000800
	tpmBottomAlign= 0x0020
	tpmRightButton= 0x0002

	idMenuOpenWeb         = 1001
	idMenuOpenData        = 1002
	idMenuPickData        = 1003
	idMenuToggleAutoStart = 1004
	idMenuInstallSvc      = 1005
	idMenuUninstallSvc    = 1006
	idMenuExit            = 1007
)

type point struct {
	x, y int32
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type notifyIconDataW struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uTimeoutOrVersion uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

var (
	trayHWnd   uintptr
	trayNID    notifyIconDataW
	trayPort   int
	trayDataDir string
	trayStopCh chan os.Signal
)

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmTrayIcon:
		switch lParam {
		case wmLButtonUp, wmLButtonDbl:
			openWebPage(trayPort)
		case wmRButtonUp:
			showTrayMenu(hwnd)
		}
		return 0

	case wmCommand:
		menuID := int(wParam & 0xffff)
		switch menuID {
		case idMenuOpenWeb:
			openWebPage(trayPort)
		case idMenuOpenData:
			openStorageFolder(trayDataDir)
		case idMenuPickData:
			newDir, err := pickFolderDialog(hwnd, "请选择 SlimBox 视频存储根目录 (将用于保存上传与压缩产物):")
			if err == nil && newDir != "" {
				trayDataDir = newDir
				cfg := loadAppConfig()
				cfg.DataDir = newDir
				_ = saveAppConfig(cfg)
				showMsgBox("存储目录已更新", fmt.Sprintf("SlimBox 存储目录已更改为:\n%s\n\n新任务将保存在此目录下。\n已自动保存至 config.json，重启后依然有效。", newDir), false)
			}
		case idMenuToggleAutoStart:
			enabled := isAutoStartEnabled()
			next := !enabled
			if err := setAutoStartEnabled(next); err != nil {
				showMsgBox("设置失败", fmt.Sprintf("无法修改开机启动设置:\n%v", err), true)
			} else {
				if next {
					showMsgBox("开机启动已开启", "SlimBox 托盘已开启开机自启。\n每次开机登录系统后，将自动在系统托盘就绪。", false)
				} else {
					showMsgBox("开机启动已关闭", "已取消 SlimBox 托盘开机自启动。", false)
				}
			}
		case idMenuInstallSvc:
			elevateServiceAction("--install-service", trayPort, trayDataDir)
		case idMenuUninstallSvc:
			elevateServiceAction("--uninstall-service", trayPort, trayDataDir)
		case idMenuExit:
			removeTrayIcon()
			if isServiceRunning() {
				_ = stopWindowsService()
			}
			go func() {
				time.Sleep(1 * time.Second)
				os.Exit(0)
			}()
			pPostQuitMessage.Call(0)
			if trayStopCh != nil {
				trayStopCh <- os.Interrupt
			}
		}
		return 0

	case wmDestroy:
		removeTrayIcon()
		pPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := pDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

func showTrayMenu(hwnd uintptr) {
	hMenu, _, _ := pCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer pDestroyMenu.Call(hMenu)

	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	installed := isServiceInstalled()
	autoStart := isAutoStartEnabled()

	bmpWeb := createMenuIcon("web")
	defer deleteMenuIcon(bmpWeb)
	bmpFolder := createMenuIcon("folder")
	defer deleteMenuIcon(bmpFolder)
	bmpGear := createMenuIcon("gear")
	defer deleteMenuIcon(bmpGear)

	var bmpAuto uintptr
	if autoStart {
		bmpAuto = createMenuIcon("check")
	} else {
		bmpAuto = createMenuIcon("uncheck")
	}
	defer deleteMenuIcon(bmpAuto)

	bmpSvc := createMenuIcon("service")
	defer deleteMenuIcon(bmpSvc)
	bmpExit := createMenuIcon("exit")
	defer deleteMenuIcon(bmpExit)

	appendMenuItemWithIcon(hMenu, idMenuOpenWeb, "打开 Web 控制台", bmpWeb)
	appendMenuItemWithIcon(hMenu, idMenuOpenData, "打开输出目录 (outputs)", bmpFolder)
	appendMenuItemWithIcon(hMenu, idMenuPickData, "设置视频存储目录...", bmpGear)
	appendSeparator(hMenu)

	if autoStart {
		appendMenuItemWithIcon(hMenu, idMenuToggleAutoStart, "开机自动启动托盘 (已开启)", bmpAuto)
	} else {
		appendMenuItemWithIcon(hMenu, idMenuToggleAutoStart, "开机自动启动托盘 (已关闭)", bmpAuto)
	}

	if installed {
		appendMenuItemWithIcon(hMenu, idMenuUninstallSvc, "卸载系统服务 (需要提权)", bmpSvc)
	} else {
		appendMenuItemWithIcon(hMenu, idMenuInstallSvc, "安装系统服务 (开机自启, 提权)", bmpSvc)
	}

	appendSeparator(hMenu)
	appendMenuItemWithIcon(hMenu, idMenuExit, "退出 SlimBox", bmpExit)

	pSetForegroundWindow.Call(hwnd)
	pTrackPopupMenu.Call(hMenu, tpmBottomAlign|tpmRightButton, uintptr(pt.x), uintptr(pt.y), 0, hwnd, 0)
}

func appendMenuItemWithIcon(hMenu uintptr, id int, text string, hBmp uintptr) {
	ptr, _ := syscall.UTF16PtrFromString(text)
	pAppendMenuW.Call(hMenu, mfString, uintptr(id), uintptr(unsafe.Pointer(ptr)))
	if hBmp != 0 {
		var mii menuItemInfoW
		mii.cbSize = uint32(unsafe.Sizeof(mii))
		mii.fMask = miimBitmap
		mii.hbmpItem = hBmp
		pSetMenuItemInfoW.Call(hMenu, uintptr(id), 0, uintptr(unsafe.Pointer(&mii)))
	}
}

func appendSeparator(hMenu uintptr) {
	pAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
}

func openWebPage(port int) {
	url, _ := syscall.UTF16PtrFromString(fmt.Sprintf("http://localhost:%d", port))
	verb, _ := syscall.UTF16PtrFromString("open")
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(url)), 0, 0, 1)
}

func openStorageFolder(dir string) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}
	outputsDir := filepath.Join(absDir, "outputs")
	if stat, err := os.Stat(outputsDir); err == nil && stat.IsDir() {
		absDir = outputsDir
	}
	dirPtr, _ := syscall.UTF16PtrFromString(absDir)
	verb, _ := syscall.UTF16PtrFromString("open")
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(dirPtr)), 0, 0, 1)
}

func elevateServiceAction(actionFlag string, port int, dataDir string) {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	args := actionFlag + " --port=" + strconv.Itoa(port)
	if dataDir != "" {
		args += " --data-dir=" + strconv.Quote(dataDir)
	}

	verb, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exePath)
	argsPtr, _ := syscall.UTF16PtrFromString(args)

	// ShellExecute with "runas" prompts Windows native UAC dialog
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(exePtr)), uintptr(unsafe.Pointer(argsPtr)), 0, 1)
}

func removeTrayIcon() {
	if trayNID.hWnd != 0 {
		pShell_NotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&trayNID)))
		trayNID.hWnd = 0
	}
}

func startTray(port int, dataDir string, stopCh chan os.Signal) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	trayPort = port
	trayDataDir = dataDir
	trayStopCh = stopCh

	className, _ := syscall.UTF16PtrFromString("SlimBoxTrayWindowClass")
	hInst, _, _ := pGetModuleHandleW.Call(0)

	// Load branded application Logo icon
	hIcon := loadLogoIcon()

	wndClass := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     hInst,
		lpszClassName: className,
		hIcon:         hIcon,
	}

	atom, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wndClass)))
	if atom == 0 {
		return fmt.Errorf("RegisterClassExW failed: %v", err)
	}

	windowTitle, _ := syscall.UTF16PtrFromString("SlimBox Tray Monitor")
	hwnd, _, err := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		0,
		0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW failed: %v", err)
	}
	trayHWnd = hwnd

	trayNID = notifyIconDataW{
		cbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
		hWnd:             hwnd,
		uID:              1,
		uFlags:           nifMessage | nifIcon | nifTip | nifInfo,
		uCallbackMessage: wmTrayIcon,
		hIcon:            hIcon,
	}

	tipText := fmt.Sprintf("SlimBox 局域网硬件加速视频压制服务 (端口: %d)", port)
	copyUTF16(trayNID.szTip[:], tipText)

	svcActive := isServiceRunning()
	var infoTitle, infoText string
	if svcActive {
		infoTitle = "SlimBox (系统服务运行中)"
		infoText = fmt.Sprintf("Web 服务地址: http://localhost:%d\n双击托盘图标打开页面，右键菜单进行服务管理与退出。", port)
	} else {
		infoTitle = "SlimBox 视频压制服务已就绪"
		infoText = fmt.Sprintf("Web 服务地址: http://localhost:%d\n双击托盘图标打开页面，右键菜单进行服务管理与退出。", port)
	}
	copyUTF16(trayNID.szInfoTitle[:], infoTitle)
	copyUTF16(trayNID.szInfo[:], infoText)
	trayNID.dwInfoFlags = 0x00000001 // NIIF_INFO

	res, _, err := pShell_NotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&trayNID)))
	if res == 0 {
		return fmt.Errorf("Shell_NotifyIconW (NIM_ADD) failed: %v", err)
	}

	// Automatically open the Web dashboard in user's default browser
	openWebPage(port)

	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	removeTrayIcon()
	return nil
}

func copyUTF16(dst []uint16, src string) {
	chars, err := syscall.UTF16FromString(src)
	if err != nil {
		return
	}
	copy(dst, chars)
}
