package domain

import (
	"time"
)

type TaskStatus string

const (
	StatusPending     TaskStatus = "pending"     // Uploaded, media probed, awaiting user profile selection
	StatusProbing     TaskStatus = "probing"     // Analyzing media streams via ffprobe
	StatusQueued      TaskStatus = "queued"      // Submitted with parameters, waiting in serial queue
	StatusPaused      TaskStatus = "paused"      // Paused by user in queue
	StatusTranscoding TaskStatus = "transcoding" // Actively transcoding
	StatusCompleted   TaskStatus = "completed"   // Compression finished successfully
	StatusFailed      TaskStatus = "failed"      // Transcoding failed, error recorded
	StatusAborted     TaskStatus = "aborted"     // Aborted by user at runtime
)

// MediaVideoStream holds video track metadata probed by ffprobe
type MediaVideoStream struct {
	CodecName    string  `json:"codec_name"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	DisplayRatio string  `json:"display_ratio"`
	FPS          float64 `json:"fps"`
	Bitrate      int64   `json:"bitrate"`
	PixelFormat  string  `json:"pixel_format"`
}

// MediaAudioStream holds audio track metadata
type MediaAudioStream struct {
	Index         int    `json:"index"`
	CodecName     string `json:"codec_name"`
	Channels      int    `json:"channels"`
	ChannelLayout string `json:"channel_layout"`
	SampleRate    int    `json:"sample_rate"`
	Bitrate       int64  `json:"bitrate"`
	Language      string `json:"language"`
	Title         string `json:"title"`
}

// MediaSubtitleStream holds subtitle track metadata
type MediaSubtitleStream struct {
	Index     int    `json:"index"`
	CodecName string `json:"codec_name"`
	Language  string `json:"language"`
	Title     string `json:"title"`
}

// MediaInfo encapsulates all probed streams for a source video
type MediaInfo struct {
	FormatName      string                `json:"format_name"`
	DurationSeconds float64               `json:"duration_seconds"`
	FileSizeBytes   int64                 `json:"file_size_bytes"`
	Bitrate         int64                 `json:"bitrate"`
	Video           *MediaVideoStream     `json:"video,omitempty"`
	AudioTracks     []MediaAudioStream    `json:"audio_tracks"`
	SubtitleTracks  []MediaSubtitleStream `json:"subtitle_tracks"`
}

// TranscodeParams defines all parameters used to invoke ffmpeg
type TranscodeParams struct {
	ProfileName      string `json:"profile_name"`       // "360p", "480p", "720p", "1080p", "2k", "4k", "custom"
	VideoCodec       string `json:"video_codec"`        // "libx265" (default) or "libx264"
	TargetResolution string `json:"target_resolution"`  // "360p", "480p", "720p", "1080p", "2k", "4k", "original"
	TargetWidth      int    `json:"target_width"`       // Max width bound (e.g. 1280 for 720p)
	TargetHeight     int    `json:"target_height"`      // Max height bound (e.g. 720 for 720p)
	CRF              int    `json:"crf"`                // Constant Rate Factor (e.g. 24, 26, 28)
	Preset           string `json:"preset"`             // "ultrafast", "superfast", "veryfast", "faster", "fast", "medium"
	AudioCodec       string `json:"audio_codec"`        // "aac" (default) or "copy"
	AudioBitrate     string `json:"audio_bitrate"`      // "64k", "96k", "128k", "160k", "192k"
	AudioTrackPolicy string `json:"audio_track_policy"` // "all_aac" (default, ADR-0010) or "first_aac" or "copy_all"
	SubtitlePolicy   string `json:"subtitle_policy"`    // "copy_all" (default, ADR-0010) or "drop"
	FastStart        *bool   `json:"faststart,omitempty"`        // Put MOOV atom at front for instant streaming playback (default: true)
	KeyframeInterval int     `json:"keyframe_interval"`          // Keyframe interval in seconds (default: 2s for fast seek)
	MaxFPS           int     `json:"max_fps,omitempty"`          // Frame rate cap (e.g. 30, 24; 0 = preserve source)
	SourceBitrate    int64   `json:"source_bitrate,omitempty"`   // Source video bitrate in bps for ceiling calculation
	SourceFPS        float64 `json:"source_fps,omitempty"`       // Source video frame rate for smart capping
	ExtraArgs        string  `json:"extra_args"`                 // Optional custom ffmpeg args
}

// ShouldFastStart returns whether FastStart (MOOV atom front) is enabled, defaulting to true.
func (p TranscodeParams) ShouldFastStart() bool {
	if p.FastStart != nil {
		return *p.FastStart
	}
	return true
}

// EffectiveMaxFPS returns the configured max FPS ceiling, 0 means unconstrained
func (p TranscodeParams) EffectiveMaxFPS() int {
	return p.MaxFPS
}

// EffectiveKeyframeInterval returns the keyframe interval in seconds, defaulting to 2.
func (p TranscodeParams) EffectiveKeyframeInterval() int {
	if p.KeyframeInterval > 0 {
		return p.KeyframeInterval
	}
	return 2
}

// TaskProgress represents real-time transcoding metrics
type TaskProgress struct {
	Percent        float64 `json:"percent"`
	CurrentFPS     float64 `json:"current_fps"`
	CurrentTimeStr string  `json:"current_time_str"`
	CurrentSeconds float64 `json:"current_seconds"`
	TotalSeconds   float64 `json:"total_seconds"`
	Speed          string  `json:"speed"`
	ETASeconds     int64   `json:"eta_seconds"`
}

// Task is the central entity tracking the lifecycle of an uploaded video compression job
type Task struct {
	ID             string          `json:"id"`
	SourceFileName string          `json:"source_file_name"`
	SourceFilePath string          `json:"source_file_path"`
	SourceFileSize int64           `json:"source_file_size"`
	OutputFileName string          `json:"output_file_name"`
	OutputFilePath string          `json:"output_file_path"`
	OutputFileSize int64           `json:"output_file_size"`
	Status         TaskStatus      `json:"status"`
	MediaInfo      *MediaInfo      `json:"media_info,omitempty"`
	Params         TranscodeParams `json:"params"`
	Progress       TaskProgress    `json:"progress"`
	ErrorMsg       string          `json:"error_msg,omitempty"`
	DownloadCount  int             `json:"download_count"`
	Priority       int             `json:"priority"`
	CreatedAt      time.Time       `json:"created_at"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
}

// StorageSettings defines configurable storage lifecycle rules (ADR-0004)
type StorageSettings struct {
	DeleteSourceAfterTranscode bool `json:"delete_source_after_transcode"`
	RetentionHours             int  `json:"retention_hours"`      // 0 = disabled
	ChunkThresholdMB           int  `json:"chunk_threshold_mb"`   // Default 200MB, <=0 fallback to 200
}

// ProfileDefinition represents a named preset in the resolution matrix (ADR-0003, ADR-0007)
type ProfileDefinition struct {
	Name             string          `json:"name"`              // "360p", "480p", "720p", "1080p", "2k", "4k"
	Label            string          `json:"label"`             // Friendly display name
	Description      string          `json:"description"`       // Usage hints and expected size reduction
	DefaultParams    TranscodeParams `json:"default_params"`    // Factory default parameters
	EffectiveParams  TranscodeParams `json:"effective_params"`  // Current parameters (customized or factory)
	IsCustomized     bool            `json:"is_customized"`     // True if user altered this preset
	EstimatedSavings string          `json:"estimated_savings"` // e.g. "~85%"
}
