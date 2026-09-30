//go:build windows

package engine

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// EnumeratePlatformGPUs scans Windows display controllers and tests operational encoders.
func EnumeratePlatformGPUs(ffmpegPath string) []GPUDevice {
	var list []GPUDevice

	// 1. Query display controllers via PowerShell WMI
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		"Get-CimInstance Win32_VideoController | Select-Object -ExpandProperty Name")
	out, err := cmd.Output()

	var adapterNames []string
	if err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			name := strings.TrimSpace(scanner.Text())
			if name != "" {
				adapterNames = append(adapterNames, name)
			}
		}
	}

	// Fallback if WMI fails or yields empty list
	if len(adapterNames) == 0 {
		adapterNames = append(adapterNames, "Primary Graphics Adapter")
	}

	for idx, rawName := range adapterNames {
		lower := strings.ToLower(rawName)
		vendor := "unknown"
		accelType := "software"
		var codecs []string

		if strings.Contains(lower, "nvidia") || strings.Contains(lower, "geforce") || strings.Contains(lower, "rtx") || strings.Contains(lower, "gtx") {
			vendor = "nvidia"
			accelType = "nvenc"
			codecs = []string{"hevc_nvenc", "h264_nvenc"}
		} else if strings.Contains(lower, "intel") || strings.Contains(lower, "uhd") || strings.Contains(lower, "iris") || strings.Contains(lower, "arc") {
			vendor = "intel"
			accelType = "qsv"
			codecs = []string{"hevc_qsv", "h264_qsv"}
		} else if strings.Contains(lower, "amd") || strings.Contains(lower, "radeon") {
			vendor = "amd"
			accelType = "amf"
			codecs = []string{"hevc_amf", "h264_amf"}
		}

		operational := false
		if ffmpegPath != "" && len(codecs) > 0 {
			operational = dryRunWindowsCodec(ffmpegPath, codecs[0], idx)
		}

		list = append(list, GPUDevice{
			ID:          fmt.Sprintf("win_gpu_%d", idx),
			Name:        rawName,
			Vendor:      vendor,
			Type:        accelType,
			DevicePath:  fmt.Sprintf("%d", idx),
			Operational: operational,
			Codecs:      codecs,
		})
	}

	log.Printf("[GPU Discovery] Enumerated %d display adapters on Windows", len(list))
	return list
}

func dryRunWindowsCodec(ffmpegPath, codec string, adapterIdx int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var args []string
	if strings.Contains(codec, "nvenc") {
		args = []string{"-y", "-f", "lavfi", "-i", "color=c=black:s=256x256:d=0.1", "-gpu", fmt.Sprintf("%d", adapterIdx), "-c:v", codec, "-f", "null", "-"}
	} else if strings.Contains(codec, "qsv") {
		args = []string{"-y", "-f", "lavfi", "-i", "color=c=black:s=256x256:d=0.1", "-c:v", codec, "-f", "null", "-"}
	} else {
		args = []string{"-y", "-f", "lavfi", "-i", "color=c=black:s=256x256:d=0.1", "-c:v", codec, "-f", "null", "-"}
	}

	cmd := PrepareCmd(exec.CommandContext(ctx, ffmpegPath, args...))
	return cmd.Run() == nil
}
