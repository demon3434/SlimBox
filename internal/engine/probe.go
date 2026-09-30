package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
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

// ProbeMedia executes ffprobe on the given filePath, or seamlessly falls back
// to ffmpeg -i if ffprobe is absent.
func ProbeMedia(ctx context.Context, filePath string) (*domain.MediaInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ffprobePath := FindFFprobe()
	hasFFprobe := false
	if _, err := exec.LookPath(ffprobePath); err == nil {
		hasFFprobe = true
	} else if fi, err := os.Stat(ffprobePath); err == nil && !fi.IsDir() {
		hasFFprobe = true
	}

	if !hasFFprobe {
		// Fallback to ffmpeg -hide_banner -i (ADR: Eliminate redundant 227MB ffprobe.exe)
		return probeViaFFmpeg(ctx, FindFFmpeg(), filePath)
	}

	cmd := PrepareCmd(exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	))

	out, err := cmd.Output()
	if err != nil {
		// If ffprobe fails, try ffmpeg fallback before giving up
		if fbInfo, fbErr := probeViaFFmpeg(ctx, FindFFmpeg(), filePath); fbErr == nil {
			return fbInfo, nil
		}
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

var (
	reDuration   = regexp.MustCompile(`Duration:\s*(\d{2}):(\d{2}):(\d{2}(?:\.\d+)?)`)
	reBitrate    = regexp.MustCompile(`bitrate:\s*(\d+)\s*kb/s`)
	reVideo      = regexp.MustCompile(`Stream #\d+:\d+.*?: Video:\s*([a-zA-Z0-9_\-]+)`)
	reRes        = regexp.MustCompile(`(\d{2,5})x(\d{2,5})`)
	reFPS        = regexp.MustCompile(`([\d\.]+)\s*fps`)
	rePixFmt     = regexp.MustCompile(`(yuv[a-zA-Z0-9_]+)`)
	reAudio      = regexp.MustCompile(`Stream #\d+:\d+.*?: Audio:\s*([a-zA-Z0-9_\-]+)`)
	reSampleRate = regexp.MustCompile(`(\d+)\s*Hz`)
	reChannels   = regexp.MustCompile(`,\s*(stereo|mono|\d+\.\d+)`)
)

func probeViaFFmpeg(ctx context.Context, ffmpegPath, filePath string) (*domain.MediaInfo, error) {
	cmd := PrepareCmd(exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-i", filePath))
	out, _ := cmd.CombinedOutput()
	output := string(out)

	info := &domain.MediaInfo{
		AudioTracks:    make([]domain.MediaAudioStream, 0),
		SubtitleTracks: make([]domain.MediaSubtitleStream, 0),
	}

	if fi, err := os.Stat(filePath); err == nil {
		info.FileSizeBytes = fi.Size()
	}

	if m := reDuration.FindStringSubmatch(output); len(m) == 4 {
		h, _ := strconv.ParseFloat(m[1], 64)
		min, _ := strconv.ParseFloat(m[2], 64)
		sec, _ := strconv.ParseFloat(m[3], 64)
		info.DurationSeconds = h*3600 + min*60 + sec
	}

	if m := reBitrate.FindStringSubmatch(output); len(m) == 2 {
		kb, _ := strconv.ParseInt(m[1], 10, 64)
		info.Bitrate = kb * 1000
	}

	lines := strings.Split(output, "\n")
	for idx, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Stream #") {
			continue
		}

		if strings.Contains(line, ": Video:") && info.Video == nil {
			v := &domain.MediaVideoStream{}
			if m := reVideo.FindStringSubmatch(line); len(m) == 2 {
				v.CodecName = m[1]
			}
			if m := reRes.FindStringSubmatch(line); len(m) == 3 {
				w, _ := strconv.Atoi(m[1])
				h, _ := strconv.Atoi(m[2])
				v.Width = w
				v.Height = h
			}
			if m := reFPS.FindStringSubmatch(line); len(m) == 2 {
				fps, _ := strconv.ParseFloat(m[1], 64)
				v.FPS = fps
			}
			if m := rePixFmt.FindStringSubmatch(line); len(m) == 2 {
				v.PixelFormat = m[1]
			}
			info.Video = v
		} else if strings.Contains(line, ": Audio:") {
			a := domain.MediaAudioStream{
				Index: idx,
			}
			if m := reAudio.FindStringSubmatch(line); len(m) == 2 {
				a.CodecName = m[1]
			}
			if m := reSampleRate.FindStringSubmatch(line); len(m) == 2 {
				sr, _ := strconv.Atoi(m[1])
				a.SampleRate = sr
			}
			if m := reChannels.FindStringSubmatch(line); len(m) == 2 {
				a.ChannelLayout = m[1]
				if m[1] == "stereo" {
					a.Channels = 2
				} else if m[1] == "mono" {
					a.Channels = 1
				}
			}
			info.AudioTracks = append(info.AudioTracks, a)
		}
	}

	if info.DurationSeconds == 0 && info.Video == nil {
		return nil, fmt.Errorf("ffmpeg probe failed to parse media information for %s", filePath)
	}

	return info, nil
}

