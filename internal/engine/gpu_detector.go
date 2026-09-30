package engine

import (
	"bufio"
	"context"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	AccelNVENC        = "nvenc"
	AccelQSV          = "qsv"
	AccelAMF          = "amf"
	AccelVideoToolbox = "videotoolbox"
	AccelVAAPI        = "vaapi"
	AccelVAAPIIntel   = "vaapi_intel"
	AccelVAAPIAMD     = "vaapi_amd"
)

type GPUDetector struct {
	ffmpegPath string
}

func NewGPUDetector(ffmpegPath string) *GPUDetector {
	if ffmpegPath == "" {
		ffmpegPath = FindFFmpeg()
	}
	return &GPUDetector{ffmpegPath: ffmpegPath}
}

var (
	accelMu        sync.RWMutex
	detectedAccels []string
	isDetected     bool
)

// DetectActiveAccelerators inspects ffmpeg and executes sub-second dry-run probes
// to confirm which hardware accelerators have functioning drivers and hardware.
func (d *GPUDetector) DetectActiveAccelerators() []string {
	if d.ffmpegPath == "" {
		return make([]string, 0)
	}

	encoders, err := d.probeSupportedEncoders()
	if err != nil {
		log.Printf("[GPU Detector] Failed to list encoders from %s: %v", d.ffmpegPath, err)
		return make([]string, 0)
	}

	verified := make([]string, 0)

	// 1. Check NVIDIA NVENC
	if encoders["hevc_nvenc"] || encoders["h264_nvenc"] {
		enc := "hevc_nvenc"
		if !encoders["hevc_nvenc"] {
			enc = "h264_nvenc"
		}
		if d.dryRunTest(enc) {
			verified = append(verified, AccelNVENC)
			log.Println("[GPU Detector] NVIDIA NVENC hardware acceleration verified.")
		}
	}

	// 2. Check Intel QuickSync QSV
	if encoders["hevc_qsv"] || encoders["h264_qsv"] {
		enc := "hevc_qsv"
		if !encoders["hevc_qsv"] {
			enc = "h264_qsv"
		}
		if d.dryRunTest(enc) {
			verified = append(verified, AccelQSV)
			log.Println("[GPU Detector] Intel QuickSync QSV hardware acceleration verified.")
		}
	}

	// 3. Check AMD AMF
	if encoders["hevc_amf"] || encoders["h264_amf"] {
		enc := "hevc_amf"
		if !encoders["hevc_amf"] {
			enc = "h264_amf"
		}
		if d.dryRunTest(enc) {
			verified = append(verified, AccelAMF)
			log.Println("[GPU Detector] AMD AMF hardware acceleration verified.")
		}
	}

	// 4. Check Apple Silicon VideoToolbox
	if encoders["hevc_videotoolbox"] || encoders["h264_videotoolbox"] {
		enc := "hevc_videotoolbox"
		if !encoders["hevc_videotoolbox"] {
			enc = "h264_videotoolbox"
		}
		if d.dryRunTest(enc) {
			verified = append(verified, AccelVideoToolbox)
			log.Println("[GPU Detector] Apple Silicon VideoToolbox hardware acceleration verified.")
		}
	}

	// 5. Check VAAPI (Linux Intel / AMD hardware acceleration)
	if encoders["hevc_vaapi"] || encoders["h264_vaapi"] {
		enc := "hevc_vaapi"
		if !encoders["hevc_vaapi"] {
			enc = "h264_vaapi"
		}
		if d.dryRunTest(enc) {
			vendorAccel := d.detectVAAPIVendor()
			verified = append(verified, vendorAccel)
			log.Printf("[GPU Detector] %s hardware acceleration verified.\n", vendorAccel)
		}
	}

	if len(verified) == 0 {
		log.Println("[GPU Detector] No operational hardware accelerators detected. Defaulting to CPU encoding.")
	}

	return verified
}

// GetCachedAccelerators returns the detected accelerators, caching the result on first invocation.
func GetCachedAccelerators() []string {
	accelMu.Lock()
	defer accelMu.Unlock()

	if !isDetected {
		detector := NewGPUDetector("")
		detectedAccels = detector.DetectActiveAccelerators()
		if detectedAccels == nil {
			detectedAccels = make([]string, 0)
		}
		isDetected = true
	}
	return detectedAccels
}

// SetCachedAccelerators allows overriding detected accelerators for tests.
func SetCachedAccelerators(accels []string) {
	accelMu.Lock()
	defer accelMu.Unlock()
	if accels == nil {
		detectedAccels = make([]string, 0)
	} else {
		detectedAccels = accels
	}
	isDetected = true
}

// IsAcceleratorSupported checks if the specified video codec has operational hardware acceleration support.
func IsAcceleratorSupported(codec string) bool {
	active := GetCachedAccelerators()

	var requiredAccel string
	switch codec {
	case "hevc_nvenc", "h264_nvenc":
		requiredAccel = AccelNVENC
	case "hevc_qsv", "h264_qsv":
		requiredAccel = AccelQSV
	case "hevc_amf", "h264_amf":
		requiredAccel = AccelAMF
	case "hevc_videotoolbox", "h264_videotoolbox":
		requiredAccel = AccelVideoToolbox
	case "hevc_vaapi", "h264_vaapi":
		for _, a := range active {
			if a == AccelVAAPI || a == AccelVAAPIIntel || a == AccelVAAPIAMD {
				return true
			}
		}
		return false
	default:
		return true // CPU codecs (libx265, libx264, etc.) are always supported
	}

	for _, a := range active {
		if a == requiredAccel {
			return true
		}
	}
	return false
}

// SanitizeVideoCodec validates if a codec has operational hardware acceleration, falling back to CPU software encoder if not.
func SanitizeVideoCodec(codec string) string {
	if codec == "" || codec == "hevc" {
		return "libx265"
	}
	if codec == "h264" {
		return "libx264"
	}
	cfg := GetCurrentGPUConfig()
	if cfg.PreferredGPU == "cpu_only" {
		if strings.HasPrefix(codec, "h264_") {
			return "libx264"
		}
		return "libx265"
	}
	if !IsAcceleratorSupported(codec) {
		log.Printf("[Transcoder] Hardware codec %s not supported on this host, falling back to CPU software encoder", codec)
		if strings.HasPrefix(codec, "h264_") {
			return "libx264"
		}
		return "libx265"
	}
	return codec
}

func (d *GPUDetector) probeSupportedEncoders() (map[string]bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := PrepareCmd(exec.CommandContext(ctx, d.ffmpegPath, "-encoders"))
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	supported := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.HasPrefix(parts[0], "V") {
			encoderName := parts[1]
			supported[encoderName] = true
		}
	}

	return supported, nil
}

func (d *GPUDetector) dryRunTest(encoder string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var args []string
	if strings.HasSuffix(encoder, "_vaapi") {
		args = []string{
			"-y",
			"-init_hw_device", "vaapi=va:/dev/dri/renderD128",
			"-f", "lavfi",
			"-i", "color=c=black:s=256x256:d=0.1",
			"-vf", "format=nv12,hwupload",
			"-c:v", encoder,
			"-f", "null",
			"-",
		}
	} else {
		// Dry run with a 256x256 test pattern for 0.1 seconds to test driver availability (AMD AMF requires >= 256x256)
		args = []string{
			"-y",
			"-f", "lavfi",
			"-i", "color=c=black:s=256x256:d=0.1",
			"-c:v", encoder,
			"-f", "null",
			"-",
		}
	}

	cmd := PrepareCmd(exec.CommandContext(ctx, d.ffmpegPath, args...))
	err := cmd.Run()
	return err == nil
}

func (d *GPUDetector) detectVAAPIVendor() string {
	paths := []string{
		"/sys/class/drm/renderD128/device/vendor",
		"/sys/class/drm/card0/device/vendor",
	}
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			v := strings.ToLower(strings.TrimSpace(string(data)))
			if strings.Contains(v, "0x8086") {
				return AccelVAAPIIntel
			}
			if strings.Contains(v, "0x1002") {
				return AccelVAAPIAMD
			}
		}
	}

	// Fallback to querying ffmpeg driver info
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := PrepareCmd(exec.CommandContext(ctx, d.ffmpegPath, "-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-v", "verbose", "-f", "null", "-"))
	if out, err := cmd.CombinedOutput(); err == nil {
		s := strings.ToLower(string(out))
		if strings.Contains(s, "intel") {
			return AccelVAAPIIntel
		}
		if strings.Contains(s, "amd") || strings.Contains(s, "radeon") {
			return AccelVAAPIAMD
		}
	}

	return AccelVAAPI
}

