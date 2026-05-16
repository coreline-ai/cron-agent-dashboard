package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

const maxJSONBodyBytes = 2 << 20

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeDecodeError(w, err)
		return false
	}
	return true
}

func decodeOptional(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		writeDecodeError(w, err)
		return false
	}
	return true
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body too large", nil)
		return
	}
	writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid json", nil)
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
		slog.Error("internal store error", "err", err)
		writeError(w, 500, "INTERNAL_ERROR", "internal server error", nil)
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string, details any) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg, "details": details}})
}
