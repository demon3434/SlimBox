package engine

import (
	"strings"
	"testing"
)

func TestParseEncoders(t *testing.T) {
	sampleOutput := `
Encoders:
 V..... = Video
 A..... = Audio
 S..... = Subtitle
 ------
 V..... hevc_nvenc           NVIDIA NVENC hevc encoder (codec hevc)
 V..... h264_nvenc           NVIDIA NVENC H.264 encoder (codec h264)
 V..... hevc_qsv             HEVC (Intel Quick Sync Video acceleration) (codec hevc)
 V..... libx264              libx264 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (codec h264)
 V..... libx265              x265 H.265 / HEVC encoder (codec hevc)
 A..... aac                  AAC (Advanced Audio Coding)
`
	supported := make(map[string]bool)
	lines := strings.Split(sampleOutput, "\n")
	for _, line := range lines {
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) >= 2 && strings.HasPrefix(parts[0], "V") {
			supported[parts[1]] = true
		}
	}

	if !supported["hevc_nvenc"] {
		t.Errorf("Expected hevc_nvenc to be parsed")
	}
	if !supported["hevc_qsv"] {
		t.Errorf("Expected hevc_qsv to be parsed")
	}
	if !supported["libx265"] {
		t.Errorf("Expected libx265 to be parsed")
	}
	if supported["aac"] {
		t.Errorf("Did not expect audio encoder aac in video encoders")
	}
}

func TestCachedAcceleratorsOverride(t *testing.T) {
	SetCachedAccelerators([]string{AccelNVENC, AccelQSV})
	accels := GetCachedAccelerators()
	if len(accels) != 2 || accels[0] != AccelNVENC || accels[1] != AccelQSV {
		t.Errorf("Unexpected cached accelerators: %v", accels)
	}
}

func TestRealHardwareDetection(t *testing.T) {
	d := NewGPUDetector("")
	accels := d.DetectActiveAccelerators()
	t.Logf("Detected accelerators on this host: %v", accels)
}
