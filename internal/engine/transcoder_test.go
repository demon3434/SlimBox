package engine

import (
	"strings"
	"testing"

	"slimbox/internal/domain"
)

func TestBuildFFmpegArgs_DefaultH265AndAudioSubs(t *testing.T) {
	transcoder := NewTranscoder()

	params := domain.TranscodeParams{
		ProfileName:      "720p",
		VideoCodec:       "libx265", // ADR-0007
		TargetWidth:      1280,
		TargetHeight:     720,
		CRF:              24,
		Preset:           "fast",
		AudioCodec:       "aac",
		AudioBitrate:     "128k",
		AudioTrackPolicy: "all_aac",  // ADR-0010
		SubtitlePolicy:   "copy_all", // ADR-0010
	}

	args := transcoder.BuildFFmpegArgs("/input/movie.mkv", "/output/movie.mkv.part", params)
	cmdLine := strings.Join(args, " ")

	// Check video codec
	if !strings.Contains(cmdLine, "-c:v libx265") {
		t.Errorf("Expected -c:v libx265 in args, got: %s", cmdLine)
	}

	// Check CRF
	if !strings.Contains(cmdLine, "-crf 24") {
		t.Errorf("Expected -crf 24, got: %s", cmdLine)
	}

	// Check scale filter
	if !strings.Contains(cmdLine, "scale='min(1280,iw)':-2") {
		t.Errorf("Expected scale filter with max 1280, got: %s", cmdLine)
	}

	// Check ADR-0010: all audio tracks mapped and encoded to AAC
	if !strings.Contains(cmdLine, "-map 0:a? -c:a aac -b:a 128k") {
		t.Errorf("Expected all audio tracks to AAC per ADR-0010, got: %s", cmdLine)
	}

	// Check ADR-0010: soft subtitles preserved via copy
	if !strings.Contains(cmdLine, "-map 0:s? -c:s copy") {
		t.Errorf("Expected soft subtitle copy per ADR-0010, got: %s", cmdLine)
	}

	// Check progress output
	if !strings.Contains(cmdLine, "-progress pipe:1") {
		t.Errorf("Expected machine-readable -progress pipe:1, got: %s", cmdLine)
	}
}

func TestBuildFFmpegArgs_H264Override(t *testing.T) {
	transcoder := NewTranscoder()

	params := domain.TranscodeParams{
		ProfileName:      "1080p",
		VideoCodec:       "libx264",
		TargetWidth:      1920,
		TargetHeight:     1080,
		CRF:              23,
		Preset:           "veryfast",
		AudioTrackPolicy: "all_aac",
		SubtitlePolicy:   "copy_all",
	}

	args := transcoder.BuildFFmpegArgs("/input/movie.mkv", "/output/movie.mkv.part", params)
	cmdLine := strings.Join(args, " ")

	if !strings.Contains(cmdLine, "-c:v libx264") {
		t.Errorf("Expected -c:v libx264, got: %s", cmdLine)
	}
	if !strings.Contains(cmdLine, "-preset veryfast") {
		t.Errorf("Expected -preset veryfast, got: %s", cmdLine)
	}
}

func TestBuildFFmpegArgs_StreamingOptimizations(t *testing.T) {
	transcoder := NewTranscoder()
	isFalse := false
	isTrue := true

	// 1. MP4 with FastStart enabled (default) & 2s keyframe
	paramsFast := domain.TranscodeParams{
		VideoCodec:       "libx265",
		FastStart:        &isTrue,
		KeyframeInterval: 2,
	}
	argsFast := transcoder.BuildFFmpegArgs("/input/movie.mkv", "/output/movie.mp4", paramsFast)
	cmdFast := strings.Join(argsFast, " ")

	if !strings.Contains(cmdFast, "-movflags +faststart") {
		t.Errorf("Expected -movflags +faststart in MP4 output, got: %s", cmdFast)
	}
	if !strings.Contains(cmdFast, "-g 60 -keyint_min 60") {
		t.Errorf("Expected -g 60 -keyint_min 60 for 2s keyframe, got: %s", cmdFast)
	}

	// 2. MP4 with FastStart explicitly disabled
	paramsNoFast := domain.TranscodeParams{
		VideoCodec:       "libx265",
		FastStart:        &isFalse,
		KeyframeInterval: 4,
	}
	argsNoFast := transcoder.BuildFFmpegArgs("/input/movie.mkv", "/output/movie.mp4", paramsNoFast)
	cmdNoFast := strings.Join(argsNoFast, " ")

	if strings.Contains(cmdNoFast, "-movflags +faststart") {
		t.Errorf("Did not expect -movflags +faststart when disabled, got: %s", cmdNoFast)
	}
	if !strings.Contains(cmdNoFast, "-g 120 -keyint_min 120") {
		t.Errorf("Expected -g 120 -keyint_min 120 for 4s keyframe, got: %s", cmdNoFast)
	}

	// 3. MKV container should not have -movflags +faststart even if enabled
	argsMkv := transcoder.BuildFFmpegArgs("/input/movie.mkv", "/output/movie.mkv", paramsFast)
	cmdMkv := strings.Join(argsMkv, " ")
	if strings.Contains(cmdMkv, "-movflags +faststart") {
		t.Errorf("MKV container should not contain -movflags +faststart, got: %s", cmdMkv)
	}
}

func TestBuildFFmpegArgs_PartOutputFormatAndFastStart(t *testing.T) {
	transcoder := NewTranscoder()
	isTrue := true

	params := domain.TranscodeParams{
		VideoCodec: "libx265",
		FastStart:  &isTrue,
	}

	// 1. MP4 temporary .part output
	argsMP4Part := transcoder.BuildFFmpegArgs("/input/movie.mp4", "/output/movie.mp4.part", params)
	cmdMP4Part := strings.Join(argsMP4Part, " ")

	if !strings.Contains(cmdMP4Part, "-movflags +faststart") {
		t.Errorf("Expected -movflags +faststart in MP4.part output, got: %s", cmdMP4Part)
	}
	if !strings.Contains(cmdMP4Part, "-f mp4 /output/movie.mp4.part") {
		t.Errorf("Expected -f mp4 before /output/movie.mp4.part, got: %s", cmdMP4Part)
	}

	// 2. MKV temporary .part output
	argsMkvPart := transcoder.BuildFFmpegArgs("/input/movie.mkv", "/output/movie.mkv.part", params)
	cmdMkvPart := strings.Join(argsMkvPart, " ")

	if strings.Contains(cmdMkvPart, "-movflags +faststart") {
		t.Errorf("MKV.part container should not contain -movflags +faststart, got: %s", cmdMkvPart)
	}
	if !strings.Contains(cmdMkvPart, "-f matroska /output/movie.mkv.part") {
		t.Errorf("Expected -f matroska before /output/movie.mkv.part, got: %s", cmdMkvPart)
	}

	// 3. Non-MP4 source (.avi) transcoding to MP4 .part output with FastStart
	argsAviToMP4 := transcoder.BuildFFmpegArgs("/input/lubinghua.avi", "/output/lubinghua_480p_0fec02.mp4.part", params)
	cmdAviToMP4 := strings.Join(argsAviToMP4, " ")

	if !strings.Contains(cmdAviToMP4, "-movflags +faststart") {
		t.Errorf("Expected -movflags +faststart when transcoding AVI to MP4, got: %s", cmdAviToMP4)
	}
	if !strings.Contains(cmdAviToMP4, "-f mp4 /output/lubinghua_480p_0fec02.mp4.part") {
		t.Errorf("Expected -f mp4 before output file, got: %s", cmdAviToMP4)
	}
}

func TestBuildFFmpegArgs_VideoToolbox(t *testing.T) {
	transcoder := NewTranscoder()

	// 1. hevc_videotoolbox
	paramsHEVC := domain.TranscodeParams{
		VideoCodec:   "hevc_videotoolbox",
		TargetWidth:  1920,
		TargetHeight: 1080,
		CRF:          24,
	}
	argsHEVC := transcoder.BuildFFmpegArgs("/input/movie.mkv", "/output/movie.mp4.part", paramsHEVC)
	cmdHEVC := strings.Join(argsHEVC, " ")

	if !strings.Contains(cmdHEVC, "-hwaccel videotoolbox -i /input/movie.mkv") {
		t.Errorf("Expected -hwaccel videotoolbox before -i, got: %s", cmdHEVC)
	}
	if !strings.Contains(cmdHEVC, "-c:v hevc_videotoolbox") {
		t.Errorf("Expected -c:v hevc_videotoolbox, got: %s", cmdHEVC)
	}
	if !strings.Contains(cmdHEVC, "-q:v 50") {
		t.Errorf("Expected -q:v 50 for default CRF 24 mapping, got: %s", cmdHEVC)
	}
	if !strings.Contains(cmdHEVC, "-b:v 2500000 -maxrate 3500000") {
		t.Errorf("Expected default 1080p bitrate cap in args, got: %s", cmdHEVC)
	}
	if !strings.Contains(cmdHEVC, "-tag:v hvc1") {
		t.Errorf("Expected -tag:v hvc1 for Apple HEVC compatibility, got: %s", cmdHEVC)
	}

	// 2. h264_videotoolbox with direct quality factor
	paramsH264 := domain.TranscodeParams{
		VideoCodec: "h264_videotoolbox",
		CRF:        60, // direct q factor
	}
	argsH264 := transcoder.BuildFFmpegArgs("/input/movie.mkv", "/output/movie.mp4.part", paramsH264)
	cmdH264 := strings.Join(argsH264, " ")

	if !strings.Contains(cmdH264, "-c:v h264_videotoolbox") {
		t.Errorf("Expected -c:v h264_videotoolbox, got: %s", cmdH264)
	}
	if !strings.Contains(cmdH264, "-q:v 60") {
		t.Errorf("Expected -q:v 60 for direct q factor, got: %s", cmdH264)
	}
}

func TestBuildFFmpegArgs_SourceBitrateCeiling(t *testing.T) {
	transcoder := NewTranscoder()

	// Source bitrate 3.1 Mbps (3128000 bps)
	params := domain.TranscodeParams{
		VideoCodec:    "hevc_videotoolbox",
		TargetWidth:   1920,
		TargetHeight:  1080,
		SourceBitrate: 3128000,
	}

	args := transcoder.BuildFFmpegArgs("/input/source.mp4", "/output/out.mp4.part", params)
	cmdLine := strings.Join(args, " ")

	// Expected target: ~48% of 3128000 = ~1501440
	// Expected maxrate: ~65% of 3128000 = ~2033200
	if !strings.Contains(cmdLine, "-b:v 1501440 -maxrate 2033200") {
		t.Errorf("Expected halving bitrate cap for 3.1Mbps source, got: %s", cmdLine)
	}
}

func TestBuildFFmpegArgs_SmartFPSCapping(t *testing.T) {
	transcoder := NewTranscoder()

	// Case 1: 59.94 fps source with MaxFPS 30 -> should inject fps=fps=30
	paramsHighFPS := domain.TranscodeParams{
		VideoCodec:   "hevc_videotoolbox",
		TargetWidth:  1920,
		TargetHeight: 1080,
		MaxFPS:       30,
		SourceFPS:    59.94,
	}
	argsHigh := transcoder.BuildFFmpegArgs("/input/source.mp4", "/output/out.mp4.part", paramsHighFPS)
	cmdHigh := strings.Join(argsHigh, " ")
	if !strings.Contains(cmdHigh, "fps=fps=30") {
		t.Errorf("Expected fps=fps=30 for 59.94fps source, got: %s", cmdHigh)
	}

	// Case 2: 24 fps cinema source with MaxFPS 30 -> should NOT inject fps filter
	paramsCinema := domain.TranscodeParams{
		VideoCodec:   "hevc_videotoolbox",
		TargetWidth:  1920,
		TargetHeight: 1080,
		MaxFPS:       30,
		SourceFPS:    24.0,
	}
	argsCinema := transcoder.BuildFFmpegArgs("/input/cinema.mp4", "/output/out.mp4.part", paramsCinema)
	cmdCinema := strings.Join(argsCinema, " ")
	if strings.Contains(cmdCinema, "fps=fps=30") {
		t.Errorf("Did not expect fps filter for native 24fps source, got: %s", cmdCinema)
	}
}

func TestBuildFFmpegArgs_MultiGPU(t *testing.T) {
	transcoder := NewTranscoder()

	// 1. NVIDIA NVENC
	paramsNVENC := domain.TranscodeParams{
		VideoCodec: "hevc_nvenc",
		CRF:        22,
		Preset:     "p6",
	}
	argsNVENC := transcoder.BuildFFmpegArgs("/input/test.mp4", "/output/test.mp4.part", paramsNVENC)
	cmdNVENC := strings.Join(argsNVENC, " ")
	if !strings.Contains(cmdNVENC, "-hwaccel cuda -i /input/test.mp4") {
		t.Errorf("Expected -hwaccel cuda, got: %s", cmdNVENC)
	}
	if !strings.Contains(cmdNVENC, "-c:v hevc_nvenc -rc vbr -cq:v 22 -preset p6") {
		t.Errorf("Expected NVENC parameters, got: %s", cmdNVENC)
	}
	if !strings.Contains(cmdNVENC, "-tag:v hvc1") {
		t.Errorf("Expected -tag:v hvc1 for hevc_nvenc, got: %s", cmdNVENC)
	}

	// 2. Intel QuickSync QSV
	paramsQSV := domain.TranscodeParams{
		VideoCodec: "hevc_qsv",
		CRF:        25,
		Preset:     "medium",
	}
	argsQSV := transcoder.BuildFFmpegArgs("/input/test.mp4", "/output/test.mp4.part", paramsQSV)
	cmdQSV := strings.Join(argsQSV, " ")
	if !strings.Contains(cmdQSV, "-hwaccel qsv -i /input/test.mp4") {
		t.Errorf("Expected -hwaccel qsv, got: %s", cmdQSV)
	}
	if !strings.Contains(cmdQSV, "-c:v hevc_qsv -global_quality 25 -preset medium") {
		t.Errorf("Expected QSV parameters, got: %s", cmdQSV)
	}

	// 3. AMD AMF
	paramsAMF := domain.TranscodeParams{
		VideoCodec: "hevc_amf",
		CRF:        23,
	}
	argsAMF := transcoder.BuildFFmpegArgs("/input/test.mp4", "/output/test.mp4.part", paramsAMF)
	cmdAMF := strings.Join(argsAMF, " ")
	if !strings.Contains(cmdAMF, "-hwaccel d3d11va -i /input/test.mp4") {
		t.Errorf("Expected -hwaccel d3d11va, got: %s", cmdAMF)
	}
	if !strings.Contains(cmdAMF, "-c:v hevc_amf -rc vbr_latency -quality quality -qp_i 23 -qp_p 23") {
		t.Errorf("Expected AMF parameters, got: %s", cmdAMF)
	}
}


