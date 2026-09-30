package engine

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"slimbox/internal/domain"
)

type ProgressCallback func(prog domain.TaskProgress)

// Transcoder handles the execution of FFmpeg for a specific task.
type Transcoder struct{}

func NewTranscoder() *Transcoder {
	return &Transcoder{}
}

// BuildFFmpegArgs builds the exact FFmpeg command line options according to the user's constraints.
// Strictly adheres to ADR-0007 (default H.265) and ADR-0010 (all audio tracks to AAC, soft subtitles copy).
func (t *Transcoder) BuildFFmpegArgs(inputPath, tempOutputPath string, params domain.TranscodeParams) []string {
	codec := params.VideoCodec
	if codec == "" {
		codec = "libx265" // Default H.265 per ADR-0007
	}

	args := []string{
		"-y", // overwrite partial output
	}

	// Hardware accelerated decoding options (Apple VideoToolbox, NVIDIA NVENC, Intel QSV, AMD AMF)
	args = AppendHardwareDecoder(args, codec)

	args = append(args, "-i", inputPath)

	// 1. Video stream selection and encoding
	args = append(args, "-map", "0:v:0") // Primary video stream
	args = append(args, "-c:v", codec)

	// Video filters: combine resolution scaling and smart framerate capping
	var vfFilters []string
	if params.MaxFPS > 0 {
		// Smart capping: only cap when source fps > max_fps + 1.0 (or source fps unknown)
		if params.SourceFPS <= 0 || params.SourceFPS > float64(params.MaxFPS)+1.0 {
			vfFilters = append(vfFilters, fmt.Sprintf("fps=fps=%d", params.MaxFPS))
		}
	}
	if params.TargetWidth > 0 && params.TargetHeight > 0 {
		vfFilters = append(vfFilters, fmt.Sprintf("scale='min(%d,iw)':-2", params.TargetWidth))
	} else {
		vfFilters = append(vfFilters, "scale=trunc(iw/2)*2:trunc(ih/2)*2")
	}
	scaleFilter := strings.Join(vfFilters, ",")

	// Append encoder-specific flags (quality/CRF, rate limits, GOP, tags)
	encoderArgs := BuildVideoEncoderArgs(codec, params, scaleFilter)
	args = append(args, encoderArgs...)

	// 2. Audio stream mapping & transcoding (ADR-0010: all tracks preserved to AAC)
	audioPolicy := params.AudioTrackPolicy
	if audioPolicy == "" {
		audioPolicy = "all_aac"
	}
	audioBitrate := params.AudioBitrate
	if audioBitrate == "" {
		audioBitrate = "128k"
	}

	switch audioPolicy {
	case "copy_all":
		args = append(args, "-map", "0:a?", "-c:a", "copy")
	case "first_aac":
		args = append(args, "-map", "0:a:0?", "-c:a", "aac", "-b:a", audioBitrate)
	case "all_aac":
		fallthrough
	default:
		// Map all audio streams and re-encode to compact AAC
		args = append(args, "-map", "0:a?", "-c:a", "aac", "-b:a", audioBitrate)
	}

	// 3. Subtitle stream mapping & passthrough (ADR-0010: soft subtitles copy)
	subtitlePolicy := params.SubtitlePolicy
	if subtitlePolicy == "" {
		subtitlePolicy = "copy_all"
	}
	if subtitlePolicy == "drop" {
		args = append(args, "-sn")
	} else {
		// All soft subtitles passed through without re-encoding or burning
		args = append(args, "-map", "0:s?", "-c:s", "copy")
	}

	// 4. Custom extra args if provided by advanced user
	if strings.TrimSpace(params.ExtraArgs) != "" {
		extras := strings.Fields(params.ExtraArgs)
		args = append(args, extras...)
	}

	// 5. Container optimization (FastStart MOOV atom positioning for web streaming)
	cleanOut := strings.TrimSuffix(tempOutputPath, ".part")
	ext := strings.ToLower(filepath.Ext(cleanOut))
	if (ext == ".mp4" || ext == ".m4v") && params.ShouldFastStart() {
		args = append(args, "-movflags", "+faststart")
	}

	// 6. Machine-readable progress stream to stdout
	args = append(args, "-progress", "pipe:1")

	// 7. Explicit format specification if output filename ends with .part
	if strings.HasSuffix(strings.ToLower(tempOutputPath), ".part") {
		var format string
		switch ext {
		case ".mp4", ".m4v":
			format = "mp4"
		case ".mkv":
			format = "matroska"
		case ".webm":
			format = "webm"
		case ".mov":
			format = "mov"
		case ".ts":
			format = "mpegts"
		case ".avi":
			format = "avi"
		case ".flv":
			format = "flv"
		}
		if format != "" {
			args = append(args, "-f", format)
		}
	}

	args = append(args, tempOutputPath)

	return args
}

// Execute runs the ffmpeg transcode process, reporting progress and honoring cancellation.
// Guarantees immediate cleanup of partial files on failure or abort (ADR-0008, ADR-0011).
func (t *Transcoder) Execute(
	ctx context.Context,
	task *domain.Task,
	onProgress ProgressCallback,
) error {
	tempOutputPath := task.OutputFilePath + ".part"

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(task.OutputFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Clean any previous partial file
	_ = os.Remove(tempOutputPath)

	// Enrich SourceBitrate and SourceFPS from MediaInfo if present
	if task.MediaInfo != nil {
		if task.Params.SourceBitrate <= 0 {
			if task.MediaInfo.Video != nil && task.MediaInfo.Video.Bitrate > 0 {
				task.Params.SourceBitrate = task.MediaInfo.Video.Bitrate
			} else if task.MediaInfo.Bitrate > 0 {
				task.Params.SourceBitrate = task.MediaInfo.Bitrate
			} else if task.MediaInfo.DurationSeconds > 0 && task.SourceFileSize > 0 {
				task.Params.SourceBitrate = int64(float64(task.SourceFileSize*8) / task.MediaInfo.DurationSeconds)
			}
		}
		if task.Params.SourceFPS <= 0 && task.MediaInfo.Video != nil {
			task.Params.SourceFPS = task.MediaInfo.Video.FPS
		}
	}

	args := t.BuildFFmpegArgs(task.SourceFilePath, tempOutputPath, task.Params)
	log.Printf("[Transcoder] Starting task %s: ffmpeg %s", task.ID, strings.Join(args, " "))

	ffmpegBin := FindFFmpeg()
	cmd := PrepareCmd(exec.CommandContext(ctx, ffmpegBin, args...))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		CleanupPartialFile(task.OutputFilePath)
		return fmt.Errorf("failed to open stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		CleanupPartialFile(task.OutputFilePath)
		return fmt.Errorf("failed to open stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		CleanupPartialFile(task.OutputFilePath)
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Read stderr in background for error diagnostics
	var stderrLog strings.Builder
	go func() {
		r := bufio.NewReader(stderr)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				break
			}
			if stderrLog.Len() < 8192 { // Keep last 8KB of logs
				stderrLog.WriteString(line)
			}
		}
	}()

	// Parse progress from stdout
	totalDuration := 0.0
	if task.MediaInfo != nil && task.MediaInfo.DurationSeconds > 0 {
		totalDuration = task.MediaInfo.DurationSeconds
	}

	prog := domain.TaskProgress{
		TotalSeconds: totalDuration,
	}

	go t.parseProgress(stdout, totalDuration, func(p domain.TaskProgress) {
		prog = p
		if onProgress != nil {
			onProgress(prog)
		}
	})

	waitErr := cmd.Wait()

	// Check if canceled or errored
	if ctx.Err() != nil {
		log.Printf("[Transcoder] Task %s was aborted. Cleaning partial artifact...", task.ID)
		CleanupPartialFile(task.OutputFilePath)
		return fmt.Errorf("transcoding aborted by user")
	}

	if waitErr != nil {
		log.Printf("[Transcoder] Task %s failed with error: %v. Cleaning partial artifact...", task.ID, waitErr)
		CleanupPartialFile(task.OutputFilePath)
		errMsg := strings.TrimSpace(stderrLog.String())
		if len(errMsg) > 1000 {
			errMsg = errMsg[len(errMsg)-1000:]
		}
		return fmt.Errorf("ffmpeg exited with error: %w (details: %s)", waitErr, errMsg)
	}

	// Success! Atomically rename .part file to final output path
	if err := os.Rename(tempOutputPath, task.OutputFilePath); err != nil {
		CleanupPartialFile(task.OutputFilePath)
		return fmt.Errorf("failed to rename partial file to final output: %w", err)
	}

	// Update final file size
	if fi, err := os.Stat(task.OutputFilePath); err == nil {
		task.OutputFileSize = fi.Size()
	}

	log.Printf("[Transcoder] Task %s completed successfully! Final size: %d bytes", task.ID, task.OutputFileSize)
	return nil
}

func (t *Transcoder) parseProgress(r io.Reader, totalDuration float64, cb ProgressCallback) {
	scanner := bufio.NewScanner(r)
	currentFPS := 0.0
	currentSeconds := 0.0
	speed := "0x"

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "fps":
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				currentFPS = f
			}
		case "out_time_us":
			if us, err := strconv.ParseInt(val, 10, 64); err == nil {
				currentSeconds = float64(us) / 1000000.0
			}
		case "out_time_ms":
			if ms, err := strconv.ParseInt(val, 10, 64); err == nil {
				currentSeconds = float64(ms) / 1000000.0
			}
		case "speed":
			speed = val
		case "progress":
			// progress=continue or progress=end
			percent := 0.0
			var etaSec int64 = 0
			if totalDuration > 0 {
				percent = (currentSeconds / totalDuration) * 100.0
				if percent > 100.0 {
					percent = 100.0
				}
				remainingSec := totalDuration - currentSeconds
				if remainingSec > 0 && currentFPS > 0 {
					// estimate based on speed
					spdVal := 1.0
					if strings.HasSuffix(speed, "x") {
						if s, err := strconv.ParseFloat(strings.TrimSuffix(speed, "x"), 64); err == nil && s > 0 {
							spdVal = s
						}
					}
					etaSec = int64(remainingSec / spdVal)
				}
			}

			cb(domain.TaskProgress{
				Percent:        percent,
				CurrentFPS:     currentFPS,
				CurrentSeconds: currentSeconds,
				TotalSeconds:   totalDuration,
				CurrentTimeStr: formatDuration(currentSeconds),
				Speed:          speed,
				ETASeconds:     etaSec,
			})
		}
	}
}

func formatDuration(sec float64) string {
	s := int(sec)
	h := s / 3600
	m := (s % 3600) / 60
	seconds := s % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, seconds)
}

func calcBitrateConstraints(params domain.TranscodeParams) (int64, int64) {
	var defaultTarget, defaultMax int64
	w := params.TargetWidth
	switch {
	case w >= 3840: // 4K
		defaultTarget, defaultMax = 8500000, 12000000
	case w >= 2560: // 2K
		defaultTarget, defaultMax = 4500000, 6000000
	case w >= 1920: // 1080p
		defaultTarget, defaultMax = 2500000, 3500000
	case w >= 1280: // 720p
		defaultTarget, defaultMax = 1500000, 2200000
	case w >= 854: // 480p
		defaultTarget, defaultMax = 700000, 1000000
	default: // 360p or default
		defaultTarget, defaultMax = 400000, 600000
	}

	targetBps := defaultTarget
	maxBps := defaultMax

	if params.SourceBitrate > 0 {
		// Enforce halving / safe ceiling: target ~48% of source, max ~65% of source
		safeTarget := int64(float64(params.SourceBitrate) * 0.48)
		safeMax := int64(float64(params.SourceBitrate) * 0.65)
		if safeTarget < 450000 {
			safeTarget = 450000
		}
		if safeMax < 600000 {
			safeMax = 600000
		}
		if safeTarget < targetBps {
			targetBps = safeTarget
		}
		if safeMax < maxBps {
			maxBps = safeMax
		}
	}
	return targetBps, maxBps
}
