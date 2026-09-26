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
	// Defaults: delete source after transcode = false, delete output after download = false, retention = 0
	def := domain.StorageSettings{
		DeleteSourceAfterTranscode: false,
		DeleteOutputAfterDownload:  false,
		RetentionHours:             0,
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
