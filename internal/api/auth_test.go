package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"slimbox/internal/collector"
	"slimbox/internal/engine"
	"slimbox/internal/repository"
	"slimbox/internal/scheduler"
)


func setupTestServer(t *testing.T, authEnabled bool) (*Server, *repository.TokenRepository, *repository.AdminRepository) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_api.db")

	db, err := repository.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	taskRepo := repository.NewTaskRepository(db)
	profileRepo := repository.NewProfileRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)
	tokenRepo := repository.NewTokenRepository(db)
	adminRepo := repository.NewAdminRepository(db)

	col := collector.NewMetricsCollector(tempDir)
	transcoder := engine.NewTranscoder()
	queue := scheduler.NewSerialQueue(taskRepo, settingsRepo, transcoder)
	lifecycle := scheduler.NewLifecycleManager(taskRepo, settingsRepo)
	server := NewServer(
		taskRepo,
		profileRepo,
		settingsRepo,
		tokenRepo,
		adminRepo,
		authEnabled,
		queue,
		lifecycle,
		tempDir,
		tempDir,
		tempDir,
		nil,
		col,
	)

	return server, tokenRepo, adminRepo

}

func TestAuth_DualModeAndRBAC(t *testing.T) {
	// --- Phase 1: Open Mode (SLIMBOX_AUTH_ENABLED = false) ---
	openServer, _, _ := setupTestServer(t, false)
	openHandler := openServer.Handler()

	// 1. Status indicates auth_enabled = false
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	rec := httptest.NewRecorder()
	openHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 from status, got %d", rec.Code)
	}

	var statusResp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&statusResp)
	if statusResp["auth_enabled"] != false {
		t.Fatalf("Expected auth_enabled false in open mode, got %v", statusResp["auth_enabled"])
	}

	// 2. Accessing protected endpoint succeeds without credentials in open mode
	reqProfiles := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
	recProfiles := httptest.NewRecorder()
	openHandler.ServeHTTP(recProfiles, reqProfiles)
	if recProfiles.Code != http.StatusOK {
		t.Fatalf("Expected 200 in open mode, got %d", recProfiles.Code)
	}

	// --- Phase 2: Strict Security Mode (SLIMBOX_AUTH_ENABLED = true) ---
	strictServer, tokenRepo, adminRepo := setupTestServer(t, true)
	strictHandler := strictServer.Handler()

	// 3. Status indicates auth_enabled = true, initialized = false
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	rec = httptest.NewRecorder()
	strictHandler.ServeHTTP(rec, req)
	json.NewDecoder(rec.Body).Decode(&statusResp)
	if statusResp["auth_enabled"] != true || statusResp["initialized"] != false {
		t.Fatalf("Expected auth_enabled=true, initialized=false initially, got: %v", statusResp)
	}

	// 4. Requesting API without credentials returns 401
	reqUnauth := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	recUnauth := httptest.NewRecorder()
	strictHandler.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 in strict mode without token, got %d", recUnauth.Code)
	}

	// 5. Setup admin password via PIN
	pin, err := adminRepo.GenerateSetupPIN()
	if err != nil {
		t.Fatalf("Failed to generate PIN: %v", err)
	}

	// 5a. Attempt setup with invalid PIN
	badSetupPayload, _ := json.Marshal(map[string]string{"pin": "000000", "password": "masterpassword123"})
	reqBadSetup := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup", bytes.NewReader(badSetupPayload))
	recBadSetup := httptest.NewRecorder()
	strictHandler.ServeHTTP(recBadSetup, reqBadSetup)
	if recBadSetup.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for wrong setup PIN, got %d", recBadSetup.Code)
	}

	// 5b. Valid setup with matching PIN
	setupPayload, _ := json.Marshal(map[string]string{"pin": pin, "password": "masterpassword123"})
	reqSetup := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup", bytes.NewReader(setupPayload))
	recSetup := httptest.NewRecorder()
	strictHandler.ServeHTTP(recSetup, reqSetup)
	if recSetup.Code != http.StatusOK {
		t.Fatalf("Expected 200 from valid setup, got %d: %s", recSetup.Code, recSetup.Body.String())
	}

	var setupResp map[string]interface{}
	json.NewDecoder(recSetup.Body).Decode(&setupResp)
	adminToken := setupResp["token"].(string)
	if adminToken == "" {
		t.Fatalf("Expected setup to return admin token")
	}

	// 6. Test Admin Login with master password
	loginPayload, _ := json.Marshal(map[string]string{"password": "masterpassword123"})
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginPayload))
	recLogin := httptest.NewRecorder()
	strictHandler.ServeHTTP(recLogin, reqLogin)
	if recLogin.Code != http.StatusOK {
		t.Fatalf("Expected 200 from login, got %d: %s", recLogin.Code, recLogin.Body.String())
	}

	// 7. Admin manages tokens: Create a user token
	createTokenPayload, _ := json.Marshal(map[string]string{"label": "CLI Worker", "role": "user"})
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/auth/tokens", bytes.NewReader(createTokenPayload))
	reqCreate.Header.Set("Authorization", "Bearer "+adminToken)
	recCreate := httptest.NewRecorder()
	strictHandler.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("Expected 201 from token creation by admin, got %d", recCreate.Code)
	}

	var createResp struct {
		Token repository.AuthToken `json:"token"`
	}
	json.NewDecoder(recCreate.Body).Decode(&createResp)
	userToken := createResp.Token.Token

	// 8. Test RBAC: User Token calling regular endpoint succeeds
	reqUserTask := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	reqUserTask.Header.Set("Authorization", "Bearer "+userToken)
	recUserTask := httptest.NewRecorder()
	strictHandler.ServeHTTP(recUserTask, reqUserTask)
	if recUserTask.Code != http.StatusOK {
		t.Fatalf("Expected 200 for user token on /tasks, got %d", recUserTask.Code)
	}

	// 9. Test RBAC: User Token calling admin endpoint is BLOCKED with 403
	reqUserForbidden := httptest.NewRequest(http.MethodGet, "/api/v1/auth/tokens", nil)
	reqUserForbidden.Header.Set("Authorization", "Bearer "+userToken)
	recUserForbidden := httptest.NewRecorder()
	strictHandler.ServeHTTP(recUserForbidden, reqUserForbidden)
	if recUserForbidden.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when user token accesses /auth/tokens, got %d", recUserForbidden.Code)
	}

	// 10. Test Fail-Closed: Admin revokes the user token
	reqRevoke := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/tokens/2", nil)
	reqRevoke.Header.Set("Authorization", "Bearer "+adminToken)
	recRevoke := httptest.NewRecorder()
	strictHandler.ServeHTTP(recRevoke, reqRevoke)
	if recRevoke.Code != http.StatusOK {
		t.Fatalf("Expected 200 revoking token, got %d", recRevoke.Code)
	}

	// After revoking user token, system DOES NOT fall back to open mode (remains strict)
	reqCheckClosed := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	recCheckClosed := httptest.NewRecorder()
	strictHandler.ServeHTTP(recCheckClosed, reqCheckClosed)
	if recCheckClosed.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401: system must remain strictly protected, got %d", recCheckClosed.Code)
	}

	_ = tokenRepo
}
