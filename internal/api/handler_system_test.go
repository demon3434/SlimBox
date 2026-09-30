package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"slimbox/internal/collector"
	"slimbox/internal/engine"
	"slimbox/internal/repository"
)

func TestSystemHandler_StatsResponse(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := repository.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer db.Close()

	settingsRepo := repository.NewSettingsRepository(db)
	col := collector.NewMetricsCollector(tempDir)
	col.Start()
	defer col.Stop()

	handler := NewSystemHandler(settingsRepo, tempDir, col)

	engine.SetCachedAccelerators([]string{"nvenc", "qsv"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/stats", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	handler.HandleStats(rec, req)
	elapsed := time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Errorf("HandleStats took %v, expected < 50ms", elapsed)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	var resp SystemStatsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode stats response: %v", err)
	}

	// Verify legacy backward compatibility fields
	if resp.NumCPU <= 0 {
		t.Errorf("Expected legacy NumCPU > 0, got %d", resp.NumCPU)
	}

	// Verify new structured metrics
	if resp.Host.NumCPU != resp.NumCPU {
		t.Errorf("Host.NumCPU (%d) doesn't match legacy NumCPU (%d)", resp.Host.NumCPU, resp.NumCPU)
	}

	// Verify hardware accelerators and host IPs
	if len(resp.HardwareAccelerators) == 0 {
		t.Errorf("Expected hardware accelerators to be detected, got empty list")
	}
}
