package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"slimbox/internal/domain"
)

type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	Index          int               `json:"index"`
	CodecType      string            `json:"codec_type"`
	CodecName      string            `json:"codec_name"`
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	DisplayRatio   string            `json:"display_aspect_ratio"`
	RFrameRate     string            `json:"r_frame_rate"`
	AvgFrameRate   string            `json:"avg_frame_rate"`
	BitRate        string            `json:"bit_rate"`
	PixFmt         string            `json:"pix_fmt"`
	Channels       int               `json:"channels"`
	ChannelLayout  string            `json:"channel_layout"`
	SampleRate     string            `json:"sample_rate"`
	Tags           map[string]string `json:"tags"`
}

type ffprobeFormat struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
	Size       string `json:"size"`
	BitRate    string `json:"bit_rate"`
}

// ProbeMedia executes ffprobe on the given filePath and returns structured MediaInfo.
func ProbeMedia(ctx context.Context, filePath string) (*domain.MediaInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe execution failed on %s: %w", filePath, err)
	}

	var raw ffprobeOutput
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe json output: %w", err)
	}

	info := &domain.MediaInfo{
		FormatName:     raw.Format.FormatName,
		AudioTracks:    make([]domain.MediaAudioStream, 0),
		SubtitleTracks: make([]domain.MediaSubtitleStream, 0),
	}

	if d, err := strconv.ParseFloat(raw.Format.Duration, 64); err == nil {
		info.DurationSeconds = d
	}
	if s, err := strconv.ParseInt(raw.Format.Size, 10, 64); err == nil {
		info.FileSizeBytes = s
	}
	if b, err := strconv.ParseInt(raw.Format.BitRate, 10, 64); err == nil {
		info.Bitrate = b
	}

	for _, stream := range raw.Streams {
		switch stream.CodecType {
		case "video":
			if info.Video == nil { // primary video stream
				fps := parseFraction(stream.AvgFrameRate)
				if fps <= 0 {
					fps = parseFraction(stream.RFrameRate)
				}
				br, _ := strconv.ParseInt(stream.BitRate, 10, 64)
				info.Video = &domain.MediaVideoStream{
					CodecName:    stream.CodecName,
					Width:        stream.Width,
					Height:       stream.Height,
					DisplayRatio: stream.DisplayRatio,
					FPS:          fps,
					Bitrate:      br,
					PixelFormat:  stream.PixFmt,
				}
			}
		case "audio":
			br, _ := strconv.ParseInt(stream.BitRate, 10, 64)
			sr, _ := strconv.Atoi(stream.SampleRate)
			lang := getTagValue(stream.Tags, "language", "lang", "LANGUAGE")
			title := getTagValue(stream.Tags, "title", "TITLE")
			info.AudioTracks = append(info.AudioTracks, domain.MediaAudioStream{
				Index:         stream.Index,
				CodecName:     stream.CodecName,
				Channels:      stream.Channels,
				ChannelLayout: stream.ChannelLayout,
				SampleRate:    sr,
				Bitrate:       br,
				Language:      lang,
				Title:         title,
			})
		case "subtitle":
			lang := getTagValue(stream.Tags, "language", "lang", "LANGUAGE")
			title := getTagValue(stream.Tags, "title", "TITLE")
			info.SubtitleTracks = append(info.SubtitleTracks, domain.MediaSubtitleStream{
				Index:     stream.Index,
				CodecName: stream.CodecName,
				Language:  lang,
				Title:     title,
			})
		}
	}

	return info, nil
}

func parseFraction(s string) float64 {
	parts := strings.Split(s, "/")
	if len(parts) == 1 {
		val, _ := strconv.ParseFloat(parts[0], 64)
		return val
	}
	if len(parts) == 2 {
		num, _ := strconv.ParseFloat(parts[0], 64)
		den, _ := strconv.ParseFloat(parts[1], 64)
		if den > 0 {
			return num / den
		}
	}
	return 0
}

func getTagValue(tags map[string]string, keys ...string) string {
	if tags == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := tags[k]; ok && v != "" {
			return v
		}
	}
	return ""
}
