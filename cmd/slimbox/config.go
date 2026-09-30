package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type AppConfig struct {
	Port    int    `json:"port,omitempty"`
	DataDir string `json:"data_dir,omitempty"`
}

func getConfigPath() string {
	exePath, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exePath), "config.json")
	}
	return "config.json"
}

func loadAppConfig() AppConfig {
	var cfg AppConfig
	data, err := os.ReadFile(getConfigPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func saveAppConfig(cfg AppConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getConfigPath(), data, 0644)
}
