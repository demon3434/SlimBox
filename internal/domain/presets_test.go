package domain

import (
	"testing"
)

func TestFactoryPresets_ResolutionMatrix(t *testing.T) {
	presets := GetFactoryPresets()
	expected := []string{"360p", "480p", "720p", "1080p", "2k", "4k"}

	if len(presets) != len(expected) {
		t.Fatalf("Expected %d presets, got %d", len(expected), len(presets))
	}

	for i, name := range expected {
		p := presets[i]
		if p.Name != name {
			t.Errorf("Preset %d name mismatch: expected %s, got %s", i, name, p.Name)
		}

		// ADR-0007: Default codec is H.265
		if p.DefaultParams.VideoCodec != "libx265" {
			t.Errorf("Preset %s default codec must be libx265, got %s", name, p.DefaultParams.VideoCodec)
		}

		// ADR-0010: Default audio policy is all_aac
		if p.DefaultParams.AudioTrackPolicy != "all_aac" {
			t.Errorf("Preset %s default audio policy must be all_aac, got %s", name, p.DefaultParams.AudioTrackPolicy)
		}

		// ADR-0010: Default subtitle policy is copy_all
		if p.DefaultParams.SubtitlePolicy != "copy_all" {
			t.Errorf("Preset %s default subtitle policy must be copy_all, got %s", name, p.DefaultParams.SubtitlePolicy)
		}
	}
}

func TestGetPresetByName(t *testing.T) {
	p := GetPresetByName("720p")
	if p == nil {
		t.Fatal("Expected 720p preset, got nil")
	}
	if p.DefaultParams.TargetWidth != 1280 || p.DefaultParams.TargetHeight != 720 {
		t.Errorf("Expected 1280x720, got %dx%d", p.DefaultParams.TargetWidth, p.DefaultParams.TargetHeight)
	}

	nonExistent := GetPresetByName("8k")
	if nonExistent != nil {
		t.Errorf("Expected nil for 8k, got %+v", nonExistent)
	}
}
