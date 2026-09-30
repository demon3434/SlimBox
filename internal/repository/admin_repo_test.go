package repository

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAdminRepository(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "slimbox_admin_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "admin_test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}
	defer db.Close()

	repo := NewAdminRepository(db)

	// 1. Initial check - should not have password
	hasPass, err := repo.HasAdminPassword()
	if err != nil {
		t.Fatalf("HasAdminPassword failed: %v", err)
	}
	if hasPass {
		t.Fatalf("Expected hasPass to be false initially")
	}

	// 2. PIN generation and verification
	pin, err := repo.GenerateSetupPIN()
	if err != nil {
		t.Fatalf("GenerateSetupPIN failed: %v", err)
	}
	if len(pin) != 6 {
		t.Fatalf("Expected 6-digit PIN, got %s", pin)
	}
	if !repo.VerifySetupPIN(pin) {
		t.Fatalf("VerifySetupPIN failed for valid PIN")
	}
	if repo.VerifySetupPIN("000000") {
		t.Fatalf("VerifySetupPIN succeeded for wrong PIN")
	}

	// 3. Set Admin Password
	if err := repo.SetAdminPassword("my_master_secret"); err != nil {
		t.Fatalf("SetAdminPassword failed: %v", err)
	}

	// Setting password should automatically clear the active PIN
	if repo.VerifySetupPIN(pin) {
		t.Fatalf("Active PIN should be cleared after password is set")
	}

	// 4. Verify password
	hasPass, err = repo.HasAdminPassword()
	if err != nil || !hasPass {
		t.Fatalf("Expected HasAdminPassword to be true after set")
	}

	valid, err := repo.VerifyAdminPassword("my_master_secret")
	if err != nil || !valid {
		t.Fatalf("VerifyAdminPassword failed for correct password")
	}

	invalid, err := repo.VerifyAdminPassword("wrong_password")
	if err != nil {
		t.Fatalf("VerifyAdminPassword errored on wrong pass: %v", err)
	}
	if invalid {
		t.Fatalf("VerifyAdminPassword succeeded for wrong password")
	}

	// 5. Update password
	if err := repo.SetAdminPassword("new_master_secret"); err != nil {
		t.Fatalf("SetAdminPassword failed on update: %v", err)
	}
	validNew, _ := repo.VerifyAdminPassword("new_master_secret")
	if !validNew {
		t.Fatalf("VerifyAdminPassword failed on updated password")
	}
	oldCheck, _ := repo.VerifyAdminPassword("my_master_secret")
	if oldCheck {
		t.Fatalf("Old password still valid after update")
	}
}
