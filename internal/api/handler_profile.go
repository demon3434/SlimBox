package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"slimbox/internal/domain"
	"slimbox/internal/repository"
)

type ProfileHandler struct {
	profileRepo *repository.ProfileRepository
}

func NewProfileHandler(profileRepo *repository.ProfileRepository) *ProfileHandler {
	return &ProfileHandler{profileRepo: profileRepo}
}

func (h *ProfileHandler) HandleProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profiles, err := h.profileRepo.GetAll()
		if err != nil {
			WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, profiles)

	case http.MethodPost:
		// Check if it's a reset action or save action
		// Paths: /api/v1/profiles/{name} or /api/v1/profiles/{name}/reset
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/profiles/")
		if path == "" {
			WriteJSONError(w, http.StatusBadRequest, "Profile name required")
			return
		}

		if strings.HasSuffix(path, "/reset") {
			name := strings.TrimSuffix(path, "/reset")
			if err := h.profileRepo.Reset(name); err != nil {
				WriteJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			WriteJSON(w, http.StatusOK, map[string]interface{}{
				"success": true,
				"message": fmt.Sprintf("Profile %s reset to factory default", name),
			})
			return
		}

		// Save custom profile params
		name := path
		var params domain.TranscodeParams
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			WriteJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}

		params.ProfileName = name
		if err := h.profileRepo.SaveCustom(name, params); err != nil {
			WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Custom profile %s saved", name),
			"params":  params,
		})

	default:
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
