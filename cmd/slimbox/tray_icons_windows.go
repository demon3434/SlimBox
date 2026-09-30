//go:build windows

package main

import (
	"math"
	"syscall"
	"unsafe"
)

var (
	gdi32             = syscall.NewLazyDLL("gdi32.dll")
	pCreateDIBSection = gdi32.NewProc("CreateDIBSection")
	pDeleteObject     = gdi32.NewProc("DeleteObject")
)

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type bitmapInfo struct {
	bmiHeader bitmapInfoHeader
	bmiColors [1]uint32
}

// createMenuIcon generates a crisp 24x24 32-bit ARGB bitmap optimized for high-DPI Win32 menus.
func createMenuIcon(kind string) uintptr {
	const w, h = 24, 24
	bmi := bitmapInfo{
		bmiHeader: bitmapInfoHeader{
			biSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			biWidth:       w,
			biHeight:      -h, // Top-down DIB
			biPlanes:      1,
			biBitCount:    32,
			biCompression: 0, // BI_RGB
		},
	}

	var pBits uintptr
	hBitmap, _, _ := pCreateDIBSection.Call(
		0,
		uintptr(unsafe.Pointer(&bmi)),
		0,
		uintptr(unsafe.Pointer(&pBits)),
		0,
		0,
	)
	if hBitmap == 0 || pBits == 0 {
		return 0
	}

	pixels := (*[w * h]uint32)(unsafe.Pointer(pBits))
	for i := range pixels {
		pixels[i] = 0
	}

	switch kind {
	case "web":
		drawWebIcon(pixels)
	case "folder":
		drawFolderIcon(pixels)
	case "gear":
		drawGearIcon(pixels)
	case "check":
		drawCheckIcon(pixels, true)
	case "uncheck":
		drawCheckIcon(pixels, false)
	case "service":
		drawServiceIcon(pixels)
	case "exit":
		drawExitIcon(pixels)
	}

	return hBitmap
}

func deleteMenuIcon(hbmp uintptr) {
	if hbmp != 0 {
		pDeleteObject.Call(hbmp)
	}
}

func setPixel(p *[576]uint32, x, y int, argb uint32) {
	if x >= 0 && x < 24 && y >= 0 && y < 24 {
		p[y*24+x] = argb
	}
}

// drawWebIcon: Vibrant blue tech globe with clear white grid (24x24)
func drawWebIcon(p *[576]uint32) {
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			dx := float64(x) - 11.5
			dy := float64(y) - 11.5
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist <= 9.5 {
				color := uint32(0xFF2563EB) // Base blue
				if dy < 0 {
					color = 0xFF3B82F6 // Light upper gradient
				}
				// Equator, prime meridian and latitude lines
				if math.Abs(dy) < 1.0 || math.Abs(dx) < 1.0 || math.Abs(dist-6.0) < 0.9 {
					color = 0xFFE0F2FE // Crisp ice white
				}
				setPixel(p, x, y, color)
			}
		}
	}
}

// drawFolderIcon: Warm golden amber modern folder with clear tabs (24x24)
func drawFolderIcon(p *[576]uint32) {
	// Top tab
	for y := 4; y <= 7; y++ {
		for x := 3; x <= 10; x++ {
			setPixel(p, x, y, 0xFFD97706)
		}
	}
	// Main folder body
	for y := 7; y <= 20; y++ {
		for x := 2; x <= 21; x++ {
			if y == 7 {
				setPixel(p, x, y, 0xFFFCD34D) // Top highlight
			} else if y == 20 || x == 2 || x == 21 {
				setPixel(p, x, y, 0xFFD97706) // Deep amber border
			} else {
				setPixel(p, x, y, 0xFFF59E0B) // Amber yellow fill
			}
		}
	}
}

// drawGearIcon: Cyber cyan 8-tooth precision gear (24x24)
func drawGearIcon(p *[576]uint32) {
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			dx := float64(x) - 11.5
			dy := float64(y) - 11.5
			dist := math.Sqrt(dx*dx + dy*dy)
			angle := math.Atan2(dy, dx)
			teeth := math.Cos(angle * 8)
			rOuter := 9.0 + 1.5*teeth
			if dist <= rOuter && dist >= 3.6 {
				setPixel(p, x, y, 0xFF06B6D4) // Bright cyan
			}
		}
	}
}

// drawCheckIcon: Emerald green checked badge or slate outline (24x24)
func drawCheckIcon(p *[576]uint32, checked bool) {
	for y := 3; y <= 20; y++ {
		for x := 3; x <= 20; x++ {
			isBorder := (x == 3 || x == 20 || y == 3 || y == 20)
			if checked {
				setPixel(p, x, y, 0xFF10B981) // Emerald green
			} else if isBorder {
				setPixel(p, x, y, 0xFF64748B) // Slate border
			}
		}
	}
	if checked {
		// Draw white checkmark ✓
		for i := 0; i < 4; i++ {
			setPixel(p, 6+i, 11+i, 0xFFFFFFFF)
			setPixel(p, 6+i, 12+i, 0xFFFFFFFF)
		}
		for i := 0; i < 8; i++ {
			setPixel(p, 10+i, 15-i, 0xFFFFFFFF)
			setPixel(p, 10+i, 14-i, 0xFFFFFFFF)
		}
	}
}

// drawServiceIcon: Violet shield & badge (24x24)
func drawServiceIcon(p *[576]uint32) {
	for y := 3; y <= 20; y++ {
		for x := 3; x <= 20; x++ {
			dx := math.Abs(float64(x) - 11.5)
			if dx <= float64(20-y)/1.4 && y >= 6 {
				setPixel(p, x, y, 0xFF8B5CF6) // Violet fill
			} else if y < 6 && dx <= 8.0 {
				setPixel(p, x, y, 0xFF8B5CF6)
			}
		}
	}
	// Center white core
	for y := 9; y <= 13; y++ {
		for x := 10; x <= 13; x++ {
			setPixel(p, x, y, 0xFFFFFFFF)
		}
	}
}

// drawExitIcon: Coral red badge with crisp white cross (24x24)
func drawExitIcon(p *[576]uint32) {
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			dx := float64(x) - 11.5
			dy := float64(y) - 11.5
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist <= 9.5 {
				setPixel(p, x, y, 0xFFEF4444) // Coral red
			}
		}
	}
	// Crisp cross ✕
	for i := -4; i <= 4; i++ {
		cx, cy := 11+i, 11+i
		setPixel(p, cx, cy, 0xFFFFFFFF)
		setPixel(p, cx+1, cy, 0xFFFFFFFF)
		setPixel(p, 12-i, cy, 0xFFFFFFFF)
		setPixel(p, 11-i, cy, 0xFFFFFFFF)
	}
}
