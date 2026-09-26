package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// OptionalAuthMiddleware enforces token/password authentication if configured (ADR-0009).
// If SLIMBOX_API_KEY or SLIMBOX_PASSWORD is empty, all requests are permitted anonymously.
func OptionalAuthMiddleware(next http.Handler) http.Handler {
	requiredKey := os.Getenv("SLIMBOX_API_KEY")
	if requiredKey == "" {
		requiredKey = os.Getenv("SLIMBOX_PASSWORD")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Enable CORS headers for API calls
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// If no auth key is configured, pass through
		if requiredKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Don't protect static assets (html, css, js) so UI can load the login dialog
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Check Authorization Header
		authHeader := r.Header.Get("Authorization")
		token := ""
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
			token = apiKey
		} else if queryToken := r.URL.Query().Get("token"); queryToken != "" {
			token = queryToken
		}

		if token != requiredKey {
			WriteJSONError(w, http.StatusUnauthorized, "Unauthorized: valid API key or password required")
			return
		}

		next.ServeHTTP(w, r)
	})
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
