package domain

// GetFactoryPresets returns the immutable, out-of-the-box matrix from 360p to 4K.
// Default video codec is H.265 (libx265) per ADR-0007 to maximize file size reduction.
// All audio tracks are preserved and encoded to AAC per ADR-0010.
// Subtitles are preserved as soft passthrough copies per ADR-0010.
func GetFactoryPresets() []ProfileDefinition {
	return []ProfileDefinition{
		{
			Name:             "360p",
			Label:            "360p 极限微缩",
			Description:      "适合老旧剧集或极度受限存储，超高压缩比，极速出片",
			EstimatedSavings: "~88%",
			DefaultParams: TranscodeParams{
				ProfileName:      "360p",
				VideoCodec:       "libx265",
				TargetResolution: "360p",
				TargetWidth:      640,
				TargetHeight:     360,
				CRF:              26,
				Preset:           "veryfast",
				AudioCodec:       "aac",
				AudioBitrate:     "64k",
				AudioTrackPolicy: "all_aac",
				SubtitlePolicy:   "copy_all",
			},
		},
		{
			Name:             "480p",
			Label:            "480p 标清省容",
			Description:      "标准清晰度，适合移动端小屏观影与连续剧收藏",
			EstimatedSavings: "~80%",
			DefaultParams: TranscodeParams{
				ProfileName:      "480p",
				VideoCodec:       "libx265",
				TargetResolution: "480p",
				TargetWidth:      854,
				TargetHeight:     480,
				CRF:              25,
				Preset:           "veryfast",
				AudioCodec:       "aac",
				AudioBitrate:     "96k",
				AudioTrackPolicy: "all_aac",
				SubtitlePolicy:   "copy_all",
			},
		},
		{
			Name:             "720p",
			Label:            "720p 高清均衡",
			Description:      "画质与体积的绝佳平衡点，斐讯 N1 与树莓派性能性价比首选",
			EstimatedSavings: "~72%",
			DefaultParams: TranscodeParams{
				ProfileName:      "720p",
				VideoCodec:       "libx265",
				TargetResolution: "720p",
				TargetWidth:      1280,
				TargetHeight:     720,
				CRF:              24,
				Preset:           "fast",
				AudioCodec:       "aac",
				AudioBitrate:     "128k",
				AudioTrackPolicy: "all_aac",
				SubtitlePolicy:   "copy_all",
			},
		},
		{
			Name:             "1080p",
			Label:            "1080p 全高清",
			Description:      "大屏观影保留丰富细节，适合电影主片压制",
			EstimatedSavings: "~60%",
			DefaultParams: TranscodeParams{
				ProfileName:      "1080p",
				VideoCodec:       "libx265",
				TargetResolution: "1080p",
				TargetWidth:      1920,
				TargetHeight:     1080,
				CRF:              23,
				Preset:           "fast",
				AudioCodec:       "aac",
				AudioBitrate:     "128k",
				AudioTrackPolicy: "all_aac",
				SubtitlePolicy:   "copy_all",
			},
		},
		{
			Name:             "2k",
			Label:            "2K 极清质感",
			Description:      "2560x1440 2K 屏幕高保真，紧凑码率微缩",
			EstimatedSavings: "~50%",
			DefaultParams: TranscodeParams{
				ProfileName:      "2k",
				VideoCodec:       "libx265",
				TargetResolution: "2k",
				TargetWidth:      2560,
				TargetHeight:     1440,
				CRF:              23,
				Preset:           "fast",
				AudioCodec:       "aac",
				AudioBitrate:     "160k",
				AudioTrackPolicy: "all_aac",
				SubtitlePolicy:   "copy_all",
			},
		},
		{
			Name:             "4k",
			Label:            "4K 超清微缩",
			Description:      "针对 4K UHD 蓝光原盘的高效体积缩减，耗时相对较长",
			EstimatedSavings: "~40%",
			DefaultParams: TranscodeParams{
				ProfileName:      "4k",
				VideoCodec:       "libx265",
				TargetResolution: "4k",
				TargetWidth:      3840,
				TargetHeight:     2160,
				CRF:              24,
				Preset:           "medium",
				AudioCodec:       "aac",
				AudioBitrate:     "192k",
				AudioTrackPolicy: "all_aac",
				SubtitlePolicy:   "copy_all",
			},
		},
	}
}

// GetPresetByName finds the factory profile definition by its standard enum identifier.
func GetPresetByName(name string) *ProfileDefinition {
	presets := GetFactoryPresets()
	for _, p := range presets {
		if p.Name == name {
			return &p
		}
	}
	return nil
}
