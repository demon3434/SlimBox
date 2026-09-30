package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenRepository(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_tokens.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize test DB: %v", err)
	}
	defer db.Close()

	repo := NewTokenRepository(db)

	// 1. Initial count should be 0
	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("Expected 0 tokens, got %d", count)
	}

	// 2. Create Token
	token, err := repo.CreateToken("Test CLI Key", "admin")
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	if !strings.HasPrefix(token.Token, "sb_") {
		t.Fatalf("Expected token to start with sb_, got %s", token.Token)
	}
	if token.Label != "Test CLI Key" {
		t.Fatalf("Expected label 'Test CLI Key', got %s", token.Label)
	}

	// 3. Validate Token
	validToken, err := repo.ValidateToken(token.Token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if validToken == nil {
		t.Fatalf("Expected valid token, got nil")
	}
	if validToken.ID != token.ID {
		t.Fatalf("Token ID mismatch: %d vs %d", validToken.ID, token.ID)
	}

	// Validate non-existent token
	invalidToken, err := repo.ValidateToken("sb_nonexistent")
	if err != nil {
		t.Fatalf("ValidateToken with invalid string errored: %v", err)
	}
	if invalidToken != nil {
		t.Fatalf("Expected nil for invalid token, got %+v", invalidToken)
	}

	// 4. Update last used
	if err := repo.UpdateLastUsed(token.ID); err != nil {
		t.Fatalf("UpdateLastUsed failed: %v", err)
	}

	// 5. List Tokens (check masking)
	tokens, err := repo.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens failed: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("Expected 1 token in list, got %d", len(tokens))
	}
	if !strings.Contains(tokens[0].Token, "...") {
		t.Fatalf("Expected masked token with '...', got %s", tokens[0].Token)
	}
	if tokens[0].LastUsedAt == nil {
		t.Fatalf("Expected LastUsedAt to be populated after update")
	}

	// 6. Revoke Token
	if err := repo.RevokeToken(token.ID); err != nil {
		t.Fatalf("RevokeToken failed: %v", err)
	}

	// Verify revoked token no longer validates
	revokedCheck, err := repo.ValidateToken(token.Token)
	if err != nil {
		t.Fatalf("ValidateToken on revoked token errored: %v", err)
	}
	if revokedCheck != nil {
		t.Fatalf("Expected nil for revoked token, got %+v", revokedCheck)
	}
}
