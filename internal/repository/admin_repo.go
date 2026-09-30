package repository

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

type AdminRepository struct {
	db        *DB
	activePIN string
	mu        sync.RWMutex
}

func NewAdminRepository(db *DB) *AdminRepository {
	return &AdminRepository{db: db}
}

// HasAdminPassword checks if a master administrator password has already been set.
func (r *AdminRepository) HasAdminPassword() (bool, error) {
	var count int
	err := r.db.SQL.QueryRow("SELECT COUNT(*) FROM admin_credentials WHERE id = 1;").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check admin credentials: %w", err)
	}
	return count > 0, nil
}

// SetAdminPassword computes a Bcrypt hash and updates the master password.
func (r *AdminRepository) SetAdminPassword(plainPassword string) error {
	if plainPassword == "" {
		return fmt.Errorf("admin password cannot be empty")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = r.db.SQL.Exec(`
		INSERT INTO admin_credentials (id, password_hash, updated_at)
		VALUES (1, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET password_hash = excluded.password_hash, updated_at = CURRENT_TIMESTAMP;
	`, string(hash))
	if err != nil {
		return fmt.Errorf("failed to save admin credentials: %w", err)
	}

	r.mu.Lock()
	r.activePIN = "" // Clear PIN once password is set
	r.mu.Unlock()

	return nil
}

// VerifyAdminPassword verifies a plaintext password against the stored bcrypt hash.
func (r *AdminRepository) VerifyAdminPassword(plainPassword string) (bool, error) {
	if plainPassword == "" {
		return false, nil
	}

	var hash string
	err := r.db.SQL.QueryRow("SELECT password_hash FROM admin_credentials WHERE id = 1;").Scan(&hash)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read admin hash: %w", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(plainPassword))
	if err != nil {
		return false, nil
	}
	return true, nil
}

// GenerateSetupPIN generates a 6-digit random PIN string and caches it in memory.
func (r *AdminRepository) GenerateSetupPIN() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "", fmt.Errorf("failed to generate random PIN: %w", err)
	}
	pin := fmt.Sprintf("%06d", n.Int64()+100000)

	r.mu.Lock()
	r.activePIN = pin
	r.mu.Unlock()

	return pin, nil
}

// VerifySetupPIN checks if the given PIN matches the active setup PIN.
func (r *AdminRepository) VerifySetupPIN(pin string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.activePIN == "" || pin == "" {
		return false
	}
	return r.activePIN == pin
}

// ClearSetupPIN explicitly wipes the memory-cached PIN.
func (r *AdminRepository) ClearSetupPIN() {
	r.mu.Lock()
	r.activePIN = ""
	r.mu.Unlock()
}
