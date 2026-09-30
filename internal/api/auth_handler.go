package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"slimbox/internal/repository"
)

type AuthHandler struct {
	tokenRepo *repository.TokenRepository
	adminRepo *repository.AdminRepository
	authMid   *AuthMiddleware
}

func NewAuthHandler(
	tokenRepo *repository.TokenRepository,
	adminRepo *repository.AdminRepository,
	authMid *AuthMiddleware,
) *AuthHandler {
	return &AuthHandler{
		tokenRepo: tokenRepo,
		adminRepo: adminRepo,
		authMid:   authMid,
	}
}

// HandleStatus reports auth mode, initialization status, and current credentials.
func (h *AuthHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	required := h.authMid.IsAuthRequired()
	hasPass := true
	if h.adminRepo != nil {
		p, err := h.adminRepo.HasAdminPassword()
		if err == nil {
			hasPass = p
		}
	}

	authenticated := false
	var label string
	var role string

	tokenStr := ExtractToken(r)
	if tok, valid := h.authMid.ValidateToken(tokenStr); valid {
		authenticated = true
		label = tok.Label
		role = tok.Role
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"auth_enabled":  required,
		"initialized":   hasPass,
		"authenticated": authenticated,
		"label":         label,
		"role":          role,
	})
}

// HandleLogin verifies the administrator master password and exchanges it for a persistent admin token.
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	clientIP := GetClientIP(r)
	if h.authMid.IsIPBlocked(clientIP) {
		WriteJSONError(w, http.StatusTooManyRequests, "Too many failed attempts. Temporary 10-minute block active.")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Password) == "" {
		WriteJSONError(w, http.StatusBadRequest, "Password is required")
		return
	}

	if h.adminRepo == nil {
		WriteJSONError(w, http.StatusInternalServerError, "Admin repository not initialized")
		return
	}

	valid, err := h.adminRepo.VerifyAdminPassword(req.Password)
	if err != nil || !valid {
		time.Sleep(500 * time.Millisecond) // Timing defense
		h.authMid.RecordAuthFailure(clientIP)
		WriteJSONError(w, http.StatusUnauthorized, "Invalid administrator password")
		return
	}

	h.authMid.ClearAuthFailure(clientIP)

	// Create an admin Bearer token for the web session
	tok, err := h.tokenRepo.CreateToken("Web Master Session", "admin")
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, "Failed to create session token: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"valid": true,
		"token": tok.Token,
		"label": tok.Label,
		"role":  tok.Role,
	})
}

// HandleSetup performs initial password setup using the 6-digit terminal PIN.
func (h *AuthHandler) HandleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	clientIP := GetClientIP(r)
	if h.authMid.IsIPBlocked(clientIP) {
		WriteJSONError(w, http.StatusTooManyRequests, "Too many failed attempts. Temporary 10-minute block active.")
		return
	}

	if h.adminRepo == nil {
		WriteJSONError(w, http.StatusInternalServerError, "Admin repository not initialized")
		return
	}

	alreadyInitialized, err := h.adminRepo.HasAdminPassword()
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, "Failed to check setup state: "+err.Error())
		return
	}
	if alreadyInitialized {
		WriteJSONError(w, http.StatusBadRequest, "System already initialized with master password")
		return
	}

	var req struct {
		PIN      string `json:"pin"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, http.StatusBadRequest, "Invalid setup payload")
		return
	}

	req.PIN = strings.TrimSpace(req.PIN)
	req.Password = strings.TrimSpace(req.Password)

	if req.PIN == "" || req.Password == "" {
		WriteJSONError(w, http.StatusBadRequest, "Both setup PIN and new password are required")
		return
	}

	if len(req.Password) < 6 {
		WriteJSONError(w, http.StatusBadRequest, "Password must be at least 6 characters long")
		return
	}

	if !h.adminRepo.VerifySetupPIN(req.PIN) {
		time.Sleep(500 * time.Millisecond) // Timing defense
		h.authMid.RecordAuthFailure(clientIP)
		WriteJSONError(w, http.StatusUnauthorized, "Invalid setup PIN code. Check terminal stdout log.")
		return
	}

	if err := h.adminRepo.SetAdminPassword(req.Password); err != nil {
		WriteJSONError(w, http.StatusInternalServerError, "Failed to store master password: "+err.Error())
		return
	}

	h.authMid.ClearAuthFailure(clientIP)

	// Automatically issue the initial admin session token
	tok, err := h.tokenRepo.CreateToken("Web Master Session", "admin")
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, "Failed to issue admin token: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"token":   tok.Token,
		"label":   tok.Label,
		"role":    tok.Role,
	})
}

// HandleVerify verifies a token or password payload {"token": "..."}
func (h *AuthHandler) HandleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	clientIP := GetClientIP(r)
	if h.authMid.IsIPBlocked(clientIP) {
		WriteJSONError(w, http.StatusTooManyRequests, "Too many failed attempts. Temporary 10-minute block active.")
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	tok, valid := h.authMid.ValidateToken(req.Token)
	if !valid {
		time.Sleep(500 * time.Millisecond) // Timing defense
		h.authMid.RecordAuthFailure(clientIP)
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   false,
			"message": "Invalid token or password",
		})
		return
	}

	h.authMid.ClearAuthFailure(clientIP)
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"valid": true,
		"label": tok.Label,
		"role":  tok.Role,
	})
}

// HandleTokens handles GET (list) and POST (create) for /api/v1/auth/tokens (Admin only)
func (h *AuthHandler) HandleTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tokens, err := h.tokenRepo.ListTokens()
		if err != nil {
			WriteJSONError(w, http.StatusInternalServerError, "Failed to list tokens: "+err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"tokens": tokens,
		})

	case http.MethodPost:
		var req struct {
			Label string `json:"label"`
			Role  string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Label) == "" {
			WriteJSONError(w, http.StatusBadRequest, "Token label is required")
			return
		}

		tok, err := h.tokenRepo.CreateToken(req.Label, req.Role)
		if err != nil {
			WriteJSONError(w, http.StatusInternalServerError, "Failed to create token: "+err.Error())
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]interface{}{
			"token": tok,
		})

	default:
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// HandleTokenSubroute handles DELETE /api/v1/auth/tokens/{id} (Admin only)
func (h *AuthHandler) HandleTokenSubroute(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/auth/tokens/")
	if idStr == "" {
		h.HandleTokens(w, r)
		return
	}

	if r.Method == http.MethodDelete {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			WriteJSONError(w, http.StatusBadRequest, "Invalid token ID")
			return
		}

		if err := h.tokenRepo.RevokeToken(id); err != nil {
			WriteJSONError(w, http.StatusInternalServerError, "Failed to revoke token: "+err.Error())
			return
		}

		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "Token revoked successfully",
		})
		return
	}

	WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
}
