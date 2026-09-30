package engine

import (
	"fmt"
	"strconv"

	"slimbox/internal/domain"
)

// AppendHardwareDecoder adds input hwaccel options based on codec and active GPU.
func AppendHardwareDecoder(args []string, codec string) []string {
	cfg := GetCurrentGPUConfig()
	if cfg.PreferredGPU == "cpu_only" {
		return args
	}

	switch codec {
	case "hevc_videotoolbox", "h264_videotoolbox":
		return append(args, "-hwaccel", "videotoolbox")
	case "hevc_nvenc", "h264_nvenc":
		return append(args, "-hwaccel", "cuda")
	case "hevc_qsv", "h264_qsv":
		return append(args, "-hwaccel", "qsv")
	case "hevc_amf", "h264_amf":
		return append(args, "-hwaccel", "d3d11va")
	case "hevc_vaapi", "h264_vaapi":
		vaapiDev := "/dev/dri/renderD128"
		if cfg.ActiveGPU != nil && cfg.ActiveGPU.DevicePath != "" {
			vaapiDev = cfg.ActiveGPU.DevicePath
		}
		return append(args, "-init_hw_device", fmt.Sprintf("vaapi=va:%s", vaapiDev))
	default:
		return args
	}
}

// BuildVideoEncoderArgs builds the encoder-specific flags (quality, bitrate, GOP, tags).
func BuildVideoEncoderArgs(codec string, params domain.TranscodeParams, scaleFilter string) []string {
	targetBps, maxBps := calcBitrateConstraints(params)
	var args []string
	cfg := GetCurrentGPUConfig()

	switch codec {
	case "hevc_nvenc", "h264_nvenc":
		cq := 24
		if params.CRF > 0 {
			cq = params.CRF
		}
		args = append(args, "-rc", "vbr", "-cq:v", strconv.Itoa(cq))
		preset := params.Preset
		if preset == "" {
			preset = "p5"
		}
		args = append(args, "-preset", preset)
		if cfg.ActiveGPU != nil && cfg.ActiveGPU.Type == "nvenc" && cfg.ActiveGPU.DevicePath != "" && cfg.ActiveGPU.DevicePath != "0" {
			args = append(args, "-gpu", cfg.ActiveGPU.DevicePath)
		}
		args = append(args, "-b:v", strconv.FormatInt(targetBps, 10), "-maxrate", strconv.FormatInt(maxBps, 10))
		args = append(args, "-vf", scaleFilter, "-pix_fmt", "yuv420p")
		if codec == "hevc_nvenc" {
			args = append(args, "-tag:v", "hvc1")
		}
		args = appendGOPArgs(args, params)

	case "hevc_qsv", "h264_qsv":
		q := 24
		if params.CRF > 0 {
			q = params.CRF
		}
		args = append(args, "-global_quality", strconv.Itoa(q))
		preset := params.Preset
		if preset == "" {
			preset = "medium"
		}
		args = append(args, "-preset", preset)
		if cfg.ActiveGPU != nil && cfg.ActiveGPU.Type == "qsv" && cfg.ActiveGPU.DevicePath != "" && cfg.ActiveGPU.DevicePath != "0" {
			args = append(args, "-qsv_device", cfg.ActiveGPU.DevicePath)
		}
		args = append(args, "-b:v", strconv.FormatInt(targetBps, 10), "-maxrate", strconv.FormatInt(maxBps, 10))
		args = append(args, "-vf", scaleFilter, "-pix_fmt", "yuv420p")
		if codec == "hevc_qsv" {
			args = append(args, "-tag:v", "hvc1")
		}
		args = appendGOPArgs(args, params)

	case "hevc_amf", "h264_amf":
		args = append(args, "-rc", "vbr_latency", "-quality", "quality")
		if params.CRF > 0 {
			args = append(args, "-qp_i", strconv.Itoa(params.CRF), "-qp_p", strconv.Itoa(params.CRF))
		}
		args = append(args, "-b:v", strconv.FormatInt(targetBps, 10), "-maxrate", strconv.FormatInt(maxBps, 10))
		args = append(args, "-vf", scaleFilter, "-pix_fmt", "yuv420p")
		if codec == "hevc_amf" {
			args = append(args, "-tag:v", "hvc1")
		}
		args = appendGOPArgs(args, params)

	case "hevc_vaapi", "h264_vaapi":
		q := 25
		if params.CRF > 0 {
			q = params.CRF
		}
		args = append(args, "-qp", strconv.Itoa(q))
		args = append(args, "-b:v", strconv.FormatInt(targetBps, 10), "-maxrate", strconv.FormatInt(maxBps, 10))
		vaapiFilter := "format=nv12,hwupload"
		if scaleFilter != "" {
			vaapiFilter = scaleFilter + ",format=nv12,hwupload"
		}
		args = append(args, "-vf", vaapiFilter)
		if codec == "hevc_vaapi" {
			args = append(args, "-tag:v", "hvc1")
		}
		args = appendGOPArgs(args, params)

	case "hevc_videotoolbox", "h264_videotoolbox":
		q := 50
		if params.CRF > 0 {
			if params.CRF > 30 {
				q = params.CRF
			} else {
				q = 50 - (params.CRF-24)*2
			}
			if q < 20 {
				q = 20
			}
			if q > 95 {
				q = 95
			}
		}
		args = append(args, "-q:v", strconv.Itoa(q))
		args = append(args, "-b:v", strconv.FormatInt(targetBps, 10), "-maxrate", strconv.FormatInt(maxBps, 10))
		args = append(args, "-vf", scaleFilter, "-pix_fmt", "yuv420p")
		if codec == "hevc_videotoolbox" {
			args = append(args, "-tag:v", "hvc1")
		}
		args = appendGOPArgs(args, params)

	default: // libx265, libx264, or any software fallback
		crf := params.CRF
		if crf <= 0 {
			if codec == "libx265" {
				crf = 24
			} else {
				crf = 23
			}
		}
		args = append(args, "-crf", strconv.Itoa(crf))

		preset := params.Preset
		if preset == "" {
			preset = "fast"
		}
		args = append(args, "-preset", preset)

		if params.SourceBitrate > 0 {
			args = append(args, "-maxrate", strconv.FormatInt(maxBps, 10), "-bufsize", strconv.FormatInt(maxBps*2, 10))
		}

		args = append(args, "-vf", scaleFilter, "-pix_fmt", "yuv420p")
		if codec == "libx265" {
			args = append(args, "-tag:v", "hvc1")
		}
		args = appendGOPArgs(args, params)
	}

	return args
}

func appendGOPArgs(args []string, params domain.TranscodeParams) []string {
	keyInterval := params.EffectiveKeyframeInterval()
	if keyInterval > 0 {
		gopFrames := keyInterval * 30
		args = append(args, "-g", fmt.Sprintf("%d", gopFrames), "-keyint_min", fmt.Sprintf("%d", gopFrames))
	}
	return args
}
