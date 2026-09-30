package repository

import (
	"path/filepath"
	"testing"

	"slimbox/internal/domain"
)

func TestProfileRepository_DecoupledPresets(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_profiles.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize DB: %v", err)
	}
	defer db.Close()

	repo := NewProfileRepository(db)

	// 1. Verify system_profiles are seeded
	var sysCount int
	err = db.SQL.QueryRow(`SELECT COUNT(*) FROM system_profiles;`).Scan(&sysCount)
	if err != nil {
		t.Fatalf("Failed to count system_profiles: %v", err)
	}
	if sysCount != len(domain.GetFactoryPresets()) {
		t.Fatalf("Expected %d system profiles, got %d", len(domain.GetFactoryPresets()), sysCount)
	}

	// 2. GetAll initially has no customizations
	profiles, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	for _, p := range profiles {
		if p.IsCustomized {
			t.Fatalf("Profile %s should not be customized initially", p.Name)
		}
	}

	// 3. Save custom override for 1080p
	orig1080p, err := repo.GetEffectiveParams("1080p")
	if err != nil {
		t.Fatalf("Failed to get 1080p: %v", err)
	}
	if orig1080p.CRF != 24 {
		t.Fatalf("Expected factory CRF 24, got %d", orig1080p.CRF)
	}

	newParams := *orig1080p
	newParams.CRF = 20
	newParams.AudioBitrate = "192k"

	if err := repo.SaveCustom("1080p", newParams); err != nil {
		t.Fatalf("SaveCustom failed: %v", err)
	}

	// 4. Verify system_profiles remained untouched
	var sysParamsJSON string
	err = db.SQL.QueryRow(`SELECT params_json FROM system_profiles WHERE name = '1080p';`).Scan(&sysParamsJSON)
	if err != nil {
		t.Fatalf("Failed to read system_profiles row: %v", err)
	}
	// Check that user_profiles table has the override
	var userParamsJSON string
	err = db.SQL.QueryRow(`SELECT params_json FROM user_profiles WHERE profile_name = '1080p' AND token_id = 0;`).Scan(&userParamsJSON)
	if err != nil {
		t.Fatalf("Failed to read user_profiles row: %v", err)
	}

	// 5. GetEffectiveParams should return user override
	eff, err := repo.GetEffectiveParams("1080p")
	if err != nil {
		t.Fatalf("GetEffectiveParams failed: %v", err)
	}
	if eff.CRF != 20 || eff.AudioBitrate != "192k" {
		t.Fatalf("Expected effective CRF 20 & 192k, got CRF %d & %s", eff.CRF, eff.AudioBitrate)
	}

	// 6. Reset should delete user override from user_profiles
	if err := repo.Reset("1080p"); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	var count int
	_ = db.SQL.QueryRow(`SELECT COUNT(*) FROM user_profiles WHERE profile_name = '1080p';`).Scan(&count)
	if count != 0 {
		t.Fatalf("Expected 0 user_profiles after reset, got %d", count)
	}

	// 7. GetEffectiveParams after reset should revert to system default
	resetEff, err := repo.GetEffectiveParams("1080p")
	if err != nil {
		t.Fatalf("GetEffectiveParams after reset failed: %v", err)
	}
	if resetEff.CRF != 24 {
		t.Fatalf("Expected reverted CRF 24, got %d", resetEff.CRF)
	}
}
