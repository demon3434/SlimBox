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
	return &ProfileRepository{db: db}
}

// GetAll returns all resolution matrix profiles, merged with any user customizations saved in SQLite.
func (r *ProfileRepository) GetAll() ([]domain.ProfileDefinition, error) {
	presets := domain.GetFactoryPresets()

	// Query custom profiles
	rows, err := r.db.SQL.Query(`SELECT name, params_json FROM custom_profiles;`)
	if err != nil {
		return nil, fmt.Errorf("failed to query custom profiles: %w", err)
	}
	defer rows.Close()

	customMap := make(map[string]domain.TranscodeParams)
	for rows.Next() {
		var name, paramsJSON string
		if err := rows.Scan(&name, &paramsJSON); err == nil {
			var p domain.TranscodeParams
			if err := json.Unmarshal([]byte(paramsJSON), &p); err == nil {
				customMap[name] = p
			}
		}
	}

	result := make([]domain.ProfileDefinition, len(presets))
	for i, preset := range presets {
		p := preset
		if custom, ok := customMap[preset.Name]; ok {
			p.EffectiveParams = custom
			p.IsCustomized = true
		} else {
			p.EffectiveParams = preset.DefaultParams
			p.IsCustomized = false
		}
		result[i] = p
	}

	return result, nil
}

// SaveCustom saves or updates user custom parameters for a profile.
func (r *ProfileRepository) SaveCustom(name string, params domain.TranscodeParams) error {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}
	query := `
	INSERT INTO custom_profiles (name, params_json, updated_at)
	VALUES (?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(name) DO UPDATE SET
		params_json = excluded.params_json,
		updated_at = CURRENT_TIMESTAMP;
	`
	_, err = r.db.SQL.Exec(query, name, string(paramsJSON))
	return err
}

// Reset reverts a profile back to its immutable factory defaults by deleting custom override.
func (r *ProfileRepository) Reset(name string) error {
	query := `DELETE FROM custom_profiles WHERE name = ?;`
	_, err := r.db.SQL.Exec(query, name)
	return err
}

// GetEffectiveParams returns the active parameters for a given profile name.
func (r *ProfileRepository) GetEffectiveParams(name string) (*domain.TranscodeParams, error) {
	var paramsJSON string
	err := r.db.SQL.QueryRow(`SELECT params_json FROM custom_profiles WHERE name = ?;`, name).Scan(&paramsJSON)
	if err == nil {
		var custom domain.TranscodeParams
		if err := json.Unmarshal([]byte(paramsJSON), &custom); err == nil {
			return &custom, nil
		}
	}

	// Fallback to factory default
	preset := domain.GetPresetByName(name)
	if preset != nil {
		cpy := preset.DefaultParams
		return &cpy, nil
	}

	return nil, fmt.Errorf("profile %q not found", name)
}
