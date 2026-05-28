package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
)

var (
	ErrBadRequest        = errors.New("bad_request")
	ErrInvalidFileKey    = errors.New("invalid_file_key")
	ErrNotFound          = errors.New("not_found")
	ErrInputTooLarge     = errors.New("input_too_large")
	ErrUnsupportedFormat = errors.New("unsupported_format")
	ErrNoEmbeddedPreview = errors.New("no_embedded_preview")
	ErrAtCapacity        = errors.New("at_capacity")
	ErrInternal          = errors.New("internal_error")
)

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrBadRequest), errors.Is(err, ErrInvalidFileKey):
		return http.StatusBadRequest
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrInputTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, ErrUnsupportedFormat):
		return http.StatusUnsupportedMediaType
	case errors.Is(err, ErrNoEmbeddedPreview):
		return http.StatusUnprocessableEntity
	case errors.Is(err, ErrAtCapacity):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func writeError(w http.ResponseWriter, err error, message string) (int, string) {
	status := statusFor(err)
	code := err.Error()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: message, Code: code})
	return status, code
}
