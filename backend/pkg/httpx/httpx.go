// Package httpx holds small JSON request/response helpers shared by every
// handler package — the HTTP-layer equivalent of pkg/money and pkg/civil:
// generic enough to not belong to any one resource's handler file.
package httpx

import (
	"encoding/json"
	"mime"
	"net/http"
	"strings"
)

// APIError is the {"detail": "..."} shape the existing FastAPI backend (and
// the clients built against it — Flutter, web) already expect on failure.
type APIError struct {
	Status int
	Detail string
}

func (e *APIError) Error() string { return e.Detail }

func NotFound(detail string) *APIError { return &APIError{Status: http.StatusNotFound, Detail: detail} }
func BadRequest(detail string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Detail: detail}
}
func Unauthorized(detail string) *APIError {
	return &APIError{Status: http.StatusUnauthorized, Detail: detail}
}
func Forbidden(detail string) *APIError {
	return &APIError{Status: http.StatusForbidden, Detail: detail}
}

func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func WriteError(w http.ResponseWriter, err error) {
	if apiErr, ok := err.(*APIError); ok {
		WriteJSON(w, apiErr.Status, map[string]string{"detail": apiErr.Detail})
		return
	}
	WriteJSON(w, http.StatusInternalServerError, map[string]string{"detail": "internal server error"})
}

func DecodeJSON(r *http.Request, dest any) error {
	if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
		return BadRequest("invalid request body: " + err.Error())
	}
	return nil
}

var contentTypesByExtension = map[string]string{
	"bundle": "application/javascript",
	"hbc":    "application/javascript",
	"js":     "application/javascript",
	"ttf":    "font/ttf",
	"otf":    "font/otf",
	"woff":   "font/woff",
	"woff2":  "font/woff2",
	"png":    "image/png",
	"jpg":    "image/jpeg",
	"jpeg":   "image/jpeg",
	"gif":    "image/gif",
	"webp":   "image/webp",
	"svg":    "image/svg+xml",
	"xml":    "application/xml",
	"json":   "application/json",
}

// ContentTypeForExtension maps a file extension (with or without the dot)
// to its MIME type, independent of the host's mime.types (the runtime image
// may not ship one).
func ContentTypeForExtension(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	if t, ok := contentTypesByExtension[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension("." + ext); t != "" {
		return t
	}
	return "application/octet-stream"
}
