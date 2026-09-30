package engine

import (
	"testing"

	"slimbox/internal/domain"
)

func TestDetermineOutputContainer(t *testing.T) {
	tests := []struct {
		name           string
		sourceFileName string
		mediaInfo      *domain.MediaInfo
		subtitlePolicy string
		expectedExt    string
	}{
		{
			name:           "AVI video without subtitles defaults to .mp4",
			sourceFileName: "鲁冰花.avi",
			mediaInfo: &domain.MediaInfo{
				FormatName:     "avi",
				SubtitleTracks: []domain.MediaSubtitleStream{},
			},
			subtitlePolicy: "copy_all",
			expectedExt:    ".mp4",
		},
		{
			name:           "MKV video without subtitles defaults to .mp4",
			sourceFileName: "movie.mkv",
			mediaInfo: &domain.MediaInfo{
				FormatName:     "matroska",
				SubtitleTracks: []domain.MediaSubtitleStream{},
			},
			subtitlePolicy: "copy_all",
			expectedExt:    ".mp4",
		},
		{
			name:           "WMV video without subtitles defaults to .mp4",
			sourceFileName: "clip.wmv",
			mediaInfo:      nil,
			subtitlePolicy: "copy_all",
			expectedExt:    ".mp4",
		},
		{
			name:           "FLV video without subtitles defaults to .mp4",
			sourceFileName: "stream.flv",
			mediaInfo:      nil,
			subtitlePolicy: "copy_all",
			expectedExt:    ".mp4",
		},
		{
			name:           "TS transport stream defaults to .mp4",
			sourceFileName: "broadcast.ts",
			mediaInfo:      nil,
			subtitlePolicy: "",
			expectedExt:    ".mp4",
		},
		{
			name:           "MP4 source without subtitles remains .mp4",
			sourceFileName: "sample.mp4",
			mediaInfo: &domain.MediaInfo{
				FormatName:     "mov,mp4,m4a,3gp,3g2,mj2",
				SubtitleTracks: []domain.MediaSubtitleStream{},
			},
			subtitlePolicy: "copy_all",
			expectedExt:    ".mp4",
		},
		{
			name:           "Source with ASS styled subtitles and copy_all falls back to .mkv",
			sourceFileName: "anime.mkv",
			mediaInfo: &domain.MediaInfo{
				FormatName: "matroska",
				SubtitleTracks: []domain.MediaSubtitleStream{
					{Index: 2, CodecName: "ass", Language: "chi"},
				},
			},
			subtitlePolicy: "copy_all",
			expectedExt:    ".mkv",
		},
		{
			name:           "Source with PGS bitmap subtitles and copy_all falls back to .mkv",
			sourceFileName: "bluray.mkv",
			mediaInfo: &domain.MediaInfo{
				FormatName: "matroska",
				SubtitleTracks: []domain.MediaSubtitleStream{
					{Index: 2, CodecName: "hdmv_pgs_subtitle", Language: "eng"},
				},
			},
			subtitlePolicy: "copy_all",
			expectedExt:    ".mkv",
		},
		{
			name:           "Source with SubRip subtitles and copy_all falls back to .mkv",
			sourceFileName: "show.mkv",
			mediaInfo: &domain.MediaInfo{
				FormatName: "matroska",
				SubtitleTracks: []domain.MediaSubtitleStream{
					{Index: 2, CodecName: "subrip", Language: "eng"},
				},
			},
			subtitlePolicy: "copy_all",
			expectedExt:    ".mkv",
		},
		{
			name:           "Source with ASS subtitles but subtitle policy is drop outputs .mp4",
			sourceFileName: "anime.mkv",
			mediaInfo: &domain.MediaInfo{
				FormatName: "matroska",
				SubtitleTracks: []domain.MediaSubtitleStream{
					{Index: 2, CodecName: "ass", Language: "chi"},
				},
			},
			subtitlePolicy: "drop",
			expectedExt:    ".mp4",
		},
		{
			name:           "Source with native mov_text subtitle and copy_all keeps .mp4",
			sourceFileName: "itunes.mp4",
			mediaInfo: &domain.MediaInfo{
				FormatName: "mp4",
				SubtitleTracks: []domain.MediaSubtitleStream{
					{Index: 2, CodecName: "mov_text", Language: "eng"},
				},
			},
			subtitlePolicy: "copy_all",
			expectedExt:    ".mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := DetermineOutputContainer(tt.sourceFileName, tt.mediaInfo, tt.subtitlePolicy)
			if ext != tt.expectedExt {
				t.Errorf("DetermineOutputContainer(%s) = %s, expected %s", tt.sourceFileName, ext, tt.expectedExt)
			}
		})
	}
}

func TestGenerateOutputFileName(t *testing.T) {
	name := GenerateOutputFileName("鲁冰花.avi", "480p", "0fec02ef-7a88-4cdd", nil, "copy_all")
	expected := "鲁冰花_480p_0fec02.mp4"
	if name != expected {
		t.Errorf("GenerateOutputFileName() = %s, expected %s", name, expected)
	}

	mkvName := GenerateOutputFileName("anime.mkv", "1080p", "abc12345-6789", &domain.MediaInfo{
		SubtitleTracks: []domain.MediaSubtitleStream{
			{Index: 2, CodecName: "ass"},
		},
	}, "copy_all")
	expectedMkv := "anime_1080p_abc123.mkv"
	if mkvName != expectedMkv {
		t.Errorf("GenerateOutputFileName() = %s, expected %s", mkvName, expectedMkv)
	}
}
