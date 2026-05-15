package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/coreline-ai/corn-agent-dashboard/internal/store"
	"net/http"
)

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid json", nil)
		return false
	}
	return true
}

func respond(w http.ResponseWriter, payload any, err error, success int) {
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if success == 0 {
		success = http.StatusOK
	}
	writeJSON(w, success, payload)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrValidation):
		writeError(w, 400, "VALIDATION_ERROR", err.Error(), nil)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, 404, "NOT_FOUND", "not found", nil)
	case errors.Is(err, store.ErrConflict):
		writeError(w, 409, "CONFLICT", err.Error(), nil)
	case errors.Is(err, store.ErrState):
		writeError(w, 409, "STATE_ERROR", err.Error(), nil)
	default:
		writeError(w, 500, "INTERNAL_ERROR", err.Error(), nil)
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string, details any) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg, "details": details}})
}
