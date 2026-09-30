//go:build !windows && !darwin

package engine

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// EnumeratePlatformGPUs scans Linux /sys/class/drm/renderD* and /dev/dri for operational GPU nodes.
func EnumeratePlatformGPUs(ffmpegPath string) []GPUDevice {
	var list []GPUDevice

	// 1. Scan Linux DRM render nodes (/dev/dri/renderD*)
	matches, _ := filepath.Glob("/sys/class/drm/renderD*")
	for _, sysPath := range matches {
		nodeID := filepath.Base(sysPath)
		devPath := filepath.Join("/dev/dri", nodeID)

		vendor := "unknown"
		name := fmt.Sprintf("Linux DRM Device (%s)", nodeID)

		vendorData, err := os.ReadFile(filepath.Join(sysPath, "device/vendor"))
		if err == nil {
			v := strings.ToLower(strings.TrimSpace(string(vendorData)))
			if strings.Contains(v, "0x8086") {
				vendor = "intel"
				name = fmt.Sprintf("Intel Graphics (%s)", nodeID)
			} else if strings.Contains(v, "0x1002") {
				vendor = "amd"
				name = fmt.Sprintf("AMD Radeon Graphics (%s)", nodeID)
			} else if strings.Contains(v, "0x10de") {
				vendor = "nvidia"
				name = fmt.Sprintf("NVIDIA GPU (%s)", nodeID)
			}
		}

		// Perform dry-run test on this specific render node
		operational := false
		var codecs []string
		if ffmpegPath != "" {
			if dryRunVAAPINode(ffmpegPath, devPath) {
				operational = true
				codecs = []string{"hevc_vaapi", "h264_vaapi"}
			}
		}

		list = append(list, GPUDevice{
			ID:          nodeID,
			Name:        name,
			Vendor:      vendor,
			Type:        "vaapi",
			DevicePath:  devPath,
			Operational: operational,
			Codecs:      codecs,
		})
	}

	// 2. Check NVIDIA NVENC via container passthrough
	if ffmpegPath != "" && probeNVENCOperational(ffmpegPath) {
		list = append([]GPUDevice{{
			ID:          "nvidia_nvenc",
			Name:        "NVIDIA GPU (NVENC Passthrough)",
			Vendor:      "nvidia",
			Type:        "nvenc",
			DevicePath:  "0",
			Operational: true,
			Codecs:      []string{"hevc_nvenc", "h264_nvenc"},
		}}, list...)
	}

	log.Printf("[GPU Discovery] Enumerated %d platform GPU accelerators on Linux", len(list))
	return list
}

func dryRunVAAPINode(ffmpegPath, devPath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	args := []string{
		"-y",
		"-init_hw_device", fmt.Sprintf("vaapi=va:%s", devPath),
		"-f", "lavfi",
		"-i", "color=c=black:s=256x256:d=0.1",
		"-vf", "format=nv12,hwupload",
		"-c:v", "hevc_vaapi",
		"-f", "null",
		"-",
	}

	cmd := PrepareCmd(exec.CommandContext(ctx, ffmpegPath, args...))
	return cmd.Run() == nil
}

func probeNVENCOperational(ffmpegPath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	args := []string{
		"-y",
		"-f", "lavfi",
		"-i", "color=c=black:s=256x256:d=0.1",
		"-c:v", "hevc_nvenc",
		"-f", "null",
		"-",
	}
	cmd := PrepareCmd(exec.CommandContext(ctx, ffmpegPath, args...))
	return cmd.Run() == nil
}
