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
