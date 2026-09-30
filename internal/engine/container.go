package engine

import (
	"path/filepath"
	"strings"

	"slimbox/internal/domain"
)

// IncompatibleMP4SubtitleCodecs lists subtitle formats that cannot be muxed
// into an MP4 container using passthrough (-c:s copy) without causing FFmpeg errors.
var IncompatibleMP4SubtitleCodecs = map[string]bool{
	"ass":               true,
	"ssa":               true,
	"subrip":            true,
	"srt":               true,
	"hdmv_pgs_subtitle": true,
	"pgs":               true,
	"dvd_subtitle":      true,
	"vobsub":            true,
	"dvb_subtitle":      true,
	"dvb_teletext":      true,
	"text":              true,
	"xsub":              true,
}

// DetermineOutputContainer determines whether the output file should be ".mp4" or ".mkv".
// It defaults to ".mp4" for universal streaming compatibility (with FastStart moov atom)
// across common source video containers (.avi, .mkv, .wmv, .flv, .mov, .ts, .mp4, etc.).
// It safely falls back to ".mkv" only when incompatible soft subtitles must be preserved.
func DetermineOutputContainer(sourceFileName string, mediaInfo *domain.MediaInfo, subtitlePolicy string) string {
	// Normalize subtitle policy (default is copy_all per ADR-0010)
	policy := strings.ToLower(strings.TrimSpace(subtitlePolicy))
	if policy == "" {
		policy = "copy_all"
	}

	// 1. If subtitles are present and the user wants to retain them, check codec compatibility
	if mediaInfo != nil && len(mediaInfo.SubtitleTracks) > 0 && policy != "drop" {
		for _, sub := range mediaInfo.SubtitleTracks {
			codec := strings.ToLower(strings.TrimSpace(sub.CodecName))
			if IncompatibleMP4SubtitleCodecs[codec] || codec == "" {
				// Incompatible subtitle stream detected, must use MKV container
				return ".mkv"
			}
			// Only mov_text is natively stream-copyable into MP4
			if codec != "mov_text" && codec != "tx3g" {
				return ".mkv"
			}
		}
	}

	// 2. Default to universal .mp4 for all video sources (with FastStart optimization)
	return ".mp4"
}

// GenerateOutputFileName produces a standardized output file name incorporating the base name,
// profile name, short task ID, and determined container extension.
func GenerateOutputFileName(sourceFileName string, profileName string, taskID string, mediaInfo *domain.MediaInfo, subtitlePolicy string) string {
	ext := filepath.Ext(sourceFileName)
	baseName := strings.TrimSuffix(sourceFileName, ext)
	if baseName == "" {
		baseName = "video"
	}

	containerExt := DetermineOutputContainer(sourceFileName, mediaInfo, subtitlePolicy)

	shortID := taskID
	if len(shortID) > 6 {
		shortID = shortID[:6]
	}

	profile := strings.TrimSpace(profileName)
	if profile == "" {
		profile = "standard"
	}

	return baseName + "_" + profile + "_" + shortID + containerExt
}
