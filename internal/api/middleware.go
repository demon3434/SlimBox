package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"slimbox/internal/repository"
)

type contextKey string

const (
	TokenContextKey contextKey = "auth_token"
)

type failureRecord struct {
	count        int
	blockedUntil time.Time
}

type AuthMiddleware struct {
	authEnabled bool
	tokenRepo   *repository.TokenRepository
	adminRepo   *repository.AdminRepository
	failMu      sync.Mutex
	failures    map[string]*failureRecord
}

func NewAuthMiddleware(authEnabled bool, tokenRepo *repository.TokenRepository, adminRepo *repository.AdminRepository) *AuthMiddleware {
	return &AuthMiddleware{
		authEnabled: authEnabled,
		tokenRepo:   tokenRepo,
		adminRepo:   adminRepo,
		failures:    make(map[string]*failureRecord),
	}
}

// ExtractToken retrieves the bearer or api key from headers or query parameters.
func ExtractToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	}
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return strings.TrimSpace(apiKey)
	}
	if queryToken := r.URL.Query().Get("token"); queryToken != "" {
		return strings.TrimSpace(queryToken)
	}
	if queryKey := r.URL.Query().Get("key"); queryKey != "" {
		return strings.TrimSpace(queryKey)
	}
	if queryApiKey := r.URL.Query().Get("api_key"); queryApiKey != "" {
		return strings.TrimSpace(queryApiKey)
	}
	return ""
}

// GetClientIP extracts the real remote IP address of the client.
func GetClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

// RecordAuthFailure registers a failed verification, increments count, and enforces 10min block on 5 failures.
func (m *AuthMiddleware) RecordAuthFailure(ip string) {
	m.failMu.Lock()
	defer m.failMu.Unlock()

	rec, exists := m.failures[ip]
	now := time.Now()
	if !exists || now.After(rec.blockedUntil) {
		rec = &failureRecord{count: 0}
		m.failures[ip] = rec
	}
	rec.count++
	if rec.count >= 5 {
		rec.blockedUntil = now.Add(10 * time.Minute)
	}
}

// ClearAuthFailure resets the failure counter for an IP upon successful auth.
func (m *AuthMiddleware) ClearAuthFailure(ip string) {
	m.failMu.Lock()
	defer m.failMu.Unlock()
	delete(m.failures, ip)
}

// IsIPBlocked checks if an IP is currently in the 10-minute ban window.
func (m *AuthMiddleware) IsIPBlocked(ip string) bool {
	m.failMu.Lock()
	defer m.failMu.Unlock()

	rec, exists := m.failures[ip]
	if !exists {
		return false
	}
	if time.Now().Before(rec.blockedUntil) {
		return true
	}
	if time.Now().After(rec.blockedUntil) && !rec.blockedUntil.IsZero() {
		delete(m.failures, ip)
	}
	return false
}

// IsAuthRequired returns true when environment enables security mode.
func (m *AuthMiddleware) IsAuthRequired() bool {
	return m.authEnabled
}

// ValidateToken verifies a token string against the token repository.
func (m *AuthMiddleware) ValidateToken(token string) (*repository.AuthToken, bool) {
	if token == "" {
		return nil, false
	}

	if m.tokenRepo != nil {
		tok, err := m.tokenRepo.ValidateToken(token)
		if err == nil && tok != nil {
			_ = m.tokenRepo.UpdateLastUsed(tok.ID)
			return tok, true
		}
	}

	return nil, false
}

func (m *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Enable CORS headers for API calls
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Don't protect static assets (html, css, js) so UI can load the setup/login modal
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Public whitelist endpoints (auth status, login, initial setup, cli binaries)
		if r.URL.Path == "/api/v1/auth/status" ||
			r.URL.Path == "/api/v1/auth/login" ||
			r.URL.Path == "/api/v1/auth/setup" ||
			r.URL.Path == "/api/v1/auth/verify" ||
			r.URL.Path == "/api/v1/cli/download" ||
			strings.HasPrefix(r.URL.Path, "/api/v1/cli/download/") {
			next.ServeHTTP(w, r)
			return
		}

		// If no authentication is configured in .env, run in completely open mode
		if !m.IsAuthRequired() {
			next.ServeHTTP(w, r)
			return
		}

		clientIP := GetClientIP(r)
		if m.IsIPBlocked(clientIP) {
			WriteJSONError(w, http.StatusTooManyRequests, "Too many failed attempts. Temporary 10-minute block active.")
			return
		}

		tokenStr := ExtractToken(r)
		tok, valid := m.ValidateToken(tokenStr)
		if !valid {
			WriteJSONError(w, http.StatusUnauthorized, "Unauthorized: valid API key or token required")
			return
		}

		ctx := context.WithValue(r.Context(), TokenContextKey, tok)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin is a decorator middleware ensuring the authenticated caller has the admin role.
func (m *AuthMiddleware) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !m.IsAuthRequired() {
			next(w, r)
			return
		}

		val := r.Context().Value(TokenContextKey)
		if val == nil {
			WriteJSONError(w, http.StatusUnauthorized, "Unauthorized: authentication required")
			return
		}

		tok, ok := val.(*repository.AuthToken)
		if !ok || tok.Role != "admin" {
			WriteJSONError(w, http.StatusForbidden, "Forbidden: administrative privileges required")
			return
		}

		next(w, r)
	}
}

func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func WriteJSONError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]interface{}{
		"error":   true,
		"message": message,
	})
}
