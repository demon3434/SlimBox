package repository

import (
	"encoding/json"
	"fmt"

	"slimbox/internal/domain"
)

type ProfileRepository struct {
	db *DB
}

func NewProfileRepository(db *DB) *ProfileRepository {
	repo := &ProfileRepository{db: db}
	_ = repo.SeedSystemProfiles(domain.GetFactoryPresets())
	return repo
}

// SeedSystemProfiles seeds or updates the system default profiles in system_profiles.
func (r *ProfileRepository) SeedSystemProfiles(presets []domain.ProfileDefinition) error {
	tx, err := r.db.SQL.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO system_profiles (name, label, resolution, params_json, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET
			label = excluded.label,
			resolution = excluded.resolution,
			params_json = excluded.params_json,
			updated_at = CURRENT_TIMESTAMP;
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, p := range presets {
		b, err := json.Marshal(p.DefaultParams)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(p.Name, p.Label, p.DefaultParams.TargetResolution, string(b)); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetAll returns all resolution matrix profiles from system_profiles, overlaid with user customizations from user_profiles.
func (r *ProfileRepository) GetAll() ([]domain.ProfileDefinition, error) {
	// Query user overrides
	rows, err := r.db.SQL.Query(`SELECT profile_name, params_json FROM user_profiles WHERE token_id = 0;`)
	if err != nil {
		return nil, fmt.Errorf("failed to query user profiles: %w", err)
	}
	defer rows.Close()

	userOverrides := make(map[string]domain.TranscodeParams)
	for rows.Next() {
		var name, paramsJSON string
		if err := rows.Scan(&name, &paramsJSON); err == nil {
			var p domain.TranscodeParams
			if err := json.Unmarshal([]byte(paramsJSON), &p); err == nil {
				userOverrides[name] = p
			}
		}
	}

	// Query system_profiles from DB
	sysRows, err := r.db.SQL.Query(`SELECT name, label, resolution, params_json FROM system_profiles;`)
	if err != nil {
		return nil, fmt.Errorf("failed to query system profiles: %w", err)
	}
	defer sysRows.Close()

	sysMap := make(map[string]domain.TranscodeParams)
	for sysRows.Next() {
		var name, label, res, paramsJSON string
		if err := sysRows.Scan(&name, &label, &res, &paramsJSON); err == nil {
			var p domain.TranscodeParams
			if err := json.Unmarshal([]byte(paramsJSON), &p); err == nil {
				sysMap[name] = p
			}
		}
	}

	presets := domain.GetFactoryPresets()
	result := make([]domain.ProfileDefinition, len(presets))
	for i, preset := range presets {
		p := preset
		if sysParams, ok := sysMap[preset.Name]; ok {
			p.DefaultParams = sysParams
		}
		if custom, ok := userOverrides[preset.Name]; ok {
			p.EffectiveParams = custom
			p.IsCustomized = true
		} else {
			p.EffectiveParams = p.DefaultParams
			p.IsCustomized = false
		}
		result[i] = p
	}

	return result, nil
}

// SaveCustom saves or updates user custom parameters for a profile in user_profiles.
func (r *ProfileRepository) SaveCustom(name string, params domain.TranscodeParams) error {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}
	query := `
	INSERT INTO user_profiles (profile_name, token_id, params_json, updated_at)
	VALUES (?, 0, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(profile_name, token_id) DO UPDATE SET
		params_json = excluded.params_json,
		updated_at = CURRENT_TIMESTAMP;
	`
	if _, err = r.db.SQL.Exec(query, name, string(paramsJSON)); err != nil {
		return err
	}

	// Sync legacy custom_profiles for backwards compatibility
	_, _ = r.db.SQL.Exec(`
		INSERT INTO custom_profiles (name, params_json, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET
			params_json = excluded.params_json,
			updated_at = CURRENT_TIMESTAMP;
	`, name, string(paramsJSON))

	return nil
}

// Reset reverts a profile back to immutable system default by deleting user override from user_profiles.
func (r *ProfileRepository) Reset(name string) error {
	query := `DELETE FROM user_profiles WHERE profile_name = ? AND token_id = 0;`
	if _, err := r.db.SQL.Exec(query, name); err != nil {
		return err
	}
	_, _ = r.db.SQL.Exec(`DELETE FROM custom_profiles WHERE name = ?;`, name)
	return nil
}

// GetEffectiveParams returns the active parameters for a given profile name.
func (r *ProfileRepository) GetEffectiveParams(name string) (*domain.TranscodeParams, error) {
	var paramsJSON string
	// Check user override
	err := r.db.SQL.QueryRow(`SELECT params_json FROM user_profiles WHERE profile_name = ? AND token_id = 0;`, name).Scan(&paramsJSON)
	if err == nil {
		var custom domain.TranscodeParams
		if err := json.Unmarshal([]byte(paramsJSON), &custom); err == nil {
			return &custom, nil
		}
	}

	// Check system profile in DB
	err = r.db.SQL.QueryRow(`SELECT params_json FROM system_profiles WHERE name = ?;`, name).Scan(&paramsJSON)
	if err == nil {
		var sysParams domain.TranscodeParams
		if err := json.Unmarshal([]byte(paramsJSON), &sysParams); err == nil {
			return &sysParams, nil
		}
	}

	// Fallback to factory default in memory
	preset := domain.GetPresetByName(name)
	if preset != nil {
		cpy := preset.DefaultParams
		return &cpy, nil
	}

	return nil, fmt.Errorf("profile %q not found", name)
}
