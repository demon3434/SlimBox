package repository

import (
	"database/sql"
	"encoding/json"
	"slimbox/internal/domain"
)

type SettingsRepository struct {
	db *DB
}

func NewSettingsRepository(db *DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) GetStorageSettings() (domain.StorageSettings, error) {
	// Defaults: delete source after transcode = false, retention = 0, threshold = 200MB
	def := domain.StorageSettings{
		DeleteSourceAfterTranscode: false,
		RetentionHours:             0,
		ChunkThresholdMB:           200,
	}

	var val string
	err := r.db.SQL.QueryRow(`SELECT value FROM system_settings WHERE key = 'storage_settings';`).Scan(&val)
	if err == sql.ErrNoRows {
		return def, nil
	}
	if err != nil {
		return def, err
	}

	var s domain.StorageSettings
	if err := json.Unmarshal([]byte(val), &s); err != nil {
		return def, nil
	}
	if s.ChunkThresholdMB <= 0 {
		s.ChunkThresholdMB = 200
	}
	return s, nil
}

func (r *SettingsRepository) SaveStorageSettings(s domain.StorageSettings) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	query := `
	INSERT INTO system_settings (key, value)
	VALUES ('storage_settings', ?)
	ON CONFLICT(key) DO UPDATE SET value = excluded.value;
	`
	_, err = r.db.SQL.Exec(query, string(data))
	return err
}

func (r *SettingsRepository) GetGPUSettings() (string, error) {
	var val string
	err := r.db.SQL.QueryRow(`SELECT value FROM system_settings WHERE key = 'preferred_gpu';`).Scan(&val)
	if err == sql.ErrNoRows {
		return "auto", nil
	}
	if err != nil {
		return "auto", err
	}
	if val == "" {
		return "auto", nil
	}
	return val, nil
}

func (r *SettingsRepository) SaveGPUSettings(pref string) error {
	if pref == "" {
		pref = "auto"
	}
	query := `
	INSERT INTO system_settings (key, value)
	VALUES ('preferred_gpu', ?)
	ON CONFLICT(key) DO UPDATE SET value = excluded.value;
	`
	_, err := r.db.SQL.Exec(query, pref)
	return err
}
