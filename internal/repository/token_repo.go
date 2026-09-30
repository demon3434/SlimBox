package repository

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

type AuthToken struct {
	ID         int64      `json:"id"`
	Token      string     `json:"token,omitempty"`
	Label      string     `json:"label"`
	Role       string     `json:"role"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type TokenRepository struct {
	db *DB
}

func NewTokenRepository(db *DB) *TokenRepository {
	return &TokenRepository{db: db}
}

// GenerateSecureToken generates a random token prefixed with sb_
func GenerateSecureToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sb_" + hex.EncodeToString(b), nil
}

// MaskToken masks a token to prevent accidental disclosure while keeping it recognizable.
func MaskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	prefix := token[:5]
	suffix := token[len(token)-4:]
	return prefix + "..." + suffix
}

// CreateToken generates a new token and inserts it into auth_tokens.
func (r *TokenRepository) CreateToken(label, role string) (*AuthToken, error) {
	if role == "" {
		role = "user"
	}
	tokenStr, err := GenerateSecureToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	res, err := r.db.SQL.Exec(`
		INSERT INTO auth_tokens (token, label, role, created_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP);
	`, tokenStr, label, role)
	if err != nil {
		return nil, fmt.Errorf("failed to insert token: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &AuthToken{
		ID:        id,
		Token:     tokenStr, // Full plaintext returned only at creation
		Label:     label,
		Role:      role,
		CreatedAt: time.Now(),
	}, nil
}

// ListTokens returns all tokens with secrets masked.
func (r *TokenRepository) ListTokens() ([]AuthToken, error) {
	rows, err := r.db.SQL.Query(`
		SELECT id, token, label, role, created_at, last_used_at
		FROM auth_tokens
		ORDER BY created_at DESC;
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	var tokens []AuthToken
	for rows.Next() {
		var t AuthToken
		var rawToken string
		var lastUsed sql.NullTime
		if err := rows.Scan(&t.ID, &rawToken, &t.Label, &t.Role, &t.CreatedAt, &lastUsed); err != nil {
			return nil, err
		}
		t.Token = MaskToken(rawToken)
		if lastUsed.Valid {
			t.LastUsedAt = &lastUsed.Time
		}
		tokens = append(tokens, t)
	}

	if tokens == nil {
		tokens = []AuthToken{}
	}
	return tokens, nil
}

// RevokeToken deletes a token by ID.
func (r *TokenRepository) RevokeToken(id int64) error {
	_, err := r.db.SQL.Exec(`DELETE FROM auth_tokens WHERE id = ?;`, id)
	return err
}

// ValidateToken checks if a token string exists in the database.
func (r *TokenRepository) ValidateToken(tokenStr string) (*AuthToken, error) {
	var t AuthToken
	var lastUsed sql.NullTime
	err := r.db.SQL.QueryRow(`
		SELECT id, token, label, role, created_at, last_used_at
		FROM auth_tokens
		WHERE token = ?;
	`, tokenStr).Scan(&t.ID, &t.Token, &t.Label, &t.Role, &t.CreatedAt, &lastUsed)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastUsed.Valid {
		t.LastUsedAt = &lastUsed.Time
	}
	return &t, nil
}

// UpdateLastUsed updates the last_used_at timestamp for a token.
func (r *TokenRepository) UpdateLastUsed(id int64) error {
	_, err := r.db.SQL.Exec(`
		UPDATE auth_tokens
		SET last_used_at = CURRENT_TIMESTAMP
		WHERE id = ?;
	`, id)
	return err
}

// Count returns the number of active tokens in the database.
func (r *TokenRepository) Count() (int, error) {
	var count int
	err := r.db.SQL.QueryRow(`SELECT COUNT(*) FROM auth_tokens;`).Scan(&count)
	return count, err
}
