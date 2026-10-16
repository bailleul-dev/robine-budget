package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorConstructors(t *testing.T) {
	cases := []struct {
		name       string
		err        *APIError
		wantStatus int
	}{
		{"NotFound", NotFound("nope"), http.StatusNotFound},
		{"BadRequest", BadRequest("bad"), http.StatusBadRequest},
		{"Unauthorized", Unauthorized("nope"), http.StatusUnauthorized},
		{"Forbidden", Forbidden("nope"), http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Status != tc.wantStatus {
				t.Errorf("Status = %d, want %d", tc.err.Status, tc.wantStatus)
			}
			if tc.err.Error() != tc.err.Detail {
				t.Errorf("Error() = %q, want %q", tc.err.Error(), tc.err.Detail)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusCreated, map[string]string{"hello": "world"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["hello"] != "world" {
		t.Fatalf("body = %v, want {hello: world}", body)
	}
}

func TestWriteJSON_NilBodyWritesNoContent(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusNoContent, nil)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
}

func TestWriteError_APIError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, NotFound("account not found"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["detail"] != "account not found" {
		t.Fatalf("detail = %q, want %q", body["detail"], "account not found")
	}
}

// A plain (non-*APIError) error — e.g. a raw DB failure — must never leak
// its message to the client: opaque 500, generic detail. Anything more
// specific has to be deliberately wrapped in an *APIError by the caller.
func TestWriteError_OpaqueForUnknownErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, errors.New("pq: connection refused"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["detail"] != "internal server error" {
		t.Fatalf("detail = %q, want the generic message (no DB internals leaked)", body["detail"])
	}
}

// WriteError only recognizes an *APIError by a direct type assertion, not
// errors.As — an *APIError wrapped with fmt.Errorf("...: %w", ...) is
// treated as an opaque error, not unwrapped to recover its status/detail.
// Documenting this as the actual current behavior, since it's an easy trap
// for a future caller who wraps an APIError expecting the status to survive.
func TestWriteError_WrappedAPIErrorIsNotUnwrapped(t *testing.T) {
	wrapped := errWrap("fetching account: ", NotFound("account not found"))
	rec := httptest.NewRecorder()
	WriteError(rec, wrapped)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (wrapped APIError is not unwrapped)", rec.Code, http.StatusInternalServerError)
	}
}

func errWrap(prefix string, err error) error {
	return &wrappedError{prefix: prefix, err: err}
}

type wrappedError struct {
	prefix string
	err    error
}

func (w *wrappedError) Error() string { return w.prefix + w.err.Error() }
func (w *wrappedError) Unwrap() error { return w.err }

func TestDecodeJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"budget"}`))
	var dest struct {
		Name string `json:"name"`
	}
	if err := DecodeJSON(req, &dest); err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if dest.Name != "budget" {
		t.Fatalf("Name = %q, want budget", dest.Name)
	}
}

func TestDecodeJSON_InvalidBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`not json`))
	var dest map[string]string
	err := DecodeJSON(req, &dest)
	if err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Status != http.StatusBadRequest {
		t.Fatalf("Status = %d, want %d", apiErr.Status, http.StatusBadRequest)
	}
}
