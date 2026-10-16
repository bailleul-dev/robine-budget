package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
)

// asUser attaches user to the request context the same way AuthMiddleware
// would have, for handler tests that call handler methods directly (thereby
// skipping the router/middleware chain entirely).
func asUser(r *http.Request, user *entities.User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userCtxKey, user))
}

func newTestUser() *entities.User {
	email := "user-" + uuid.NewString() + "@example.com"
	return &entities.User{ID: uuid.New(), Email: &email, DisplayName: "Test User"}
}

// jsonRequest builds an httptest request with a JSON body and sets path
// values (net/http's ServeMux path parameters, e.g. {accountID}) directly,
// since these tests call handler methods without going through a mux.
func jsonRequest(t *testing.T, method, target string, body any, pathValues map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request body: %v", err)
		}
	}
	req := httptest.NewRequest(method, target, &buf)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	return req
}

func decodeJSONBody(t *testing.T, rec *httptest.ResponseRecorder, dest any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(dest); err != nil {
		t.Fatalf("decode response body (status %d, body %q): %v", rec.Code, rec.Body.String(), err)
	}
}
