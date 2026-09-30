//go:build darwin

package engine

import (
	"context"
	"log"
	"os/exec"
	"time"
)

// EnumeratePlatformGPUs checks for Apple Silicon / VideoToolbox hardware acceleration.
func EnumeratePlatformGPUs(ffmpegPath string) []GPUDevice {
	var list []GPUDevice

	operational := false
	if ffmpegPath != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := PrepareCmd(exec.CommandContext(ctx, ffmpegPath, "-y", "-f", "lavfi", "-i", "color=c=black:s=256x256:d=0.1", "-c:v", "hevc_videotoolbox", "-f", "null", "-"))
		operational = cmd.Run() == nil
	}

	list = append(list, GPUDevice{
		ID:          "apple_videotoolbox",
		Name:        "Apple Silicon Media Engine (VideoToolbox)",
		Vendor:      "apple",
		Type:        "videotoolbox",
		DevicePath:  "0",
		Operational: operational,
		Codecs:      []string{"hevc_videotoolbox", "h264_videotoolbox"},
	})

	log.Printf("[GPU Discovery] Enumerated %d platform accelerators on macOS", len(list))
	return list
}
