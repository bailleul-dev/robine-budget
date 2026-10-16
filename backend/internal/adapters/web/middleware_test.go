package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bailleul.dev/budget-server/internal/adapters/security"
	"bailleul.dev/budget-server/internal/core"
)

func testTokens() security.SessionTokens {
	return security.SessionTokens{Secret: "test-secret-at-least-32-bytes-long", ExpireMinutes: 60}
}

func TestAuthMiddleware_ValidBearerToken(t *testing.T) {
	users := newFakeUserRepo()
	user := newTestUser()
	users.byID[user.ID] = user
	tokens := testTokens()
	token, err := tokens.Create(user.ID, user.TokenVersion)
	if err != nil {
		t.Fatalf("Create token: %v", err)
	}

	var gotUserID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = CurrentUser(r).ID.String()
		w.WriteHeader(http.StatusOK)
	})
	handler := AuthMiddleware(users, tokens)(next)

	req := httptest.NewRequest("GET", "/api/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	if gotUserID != user.ID.String() {
		t.Errorf("CurrentUser().ID = %q, want %s", gotUserID, user.ID)
	}
}

func TestAuthMiddleware_ValidCookie(t *testing.T) {
	users := newFakeUserRepo()
	user := newTestUser()
	users.byID[user.ID] = user
	tokens := testTokens()
	token, err := tokens.Create(user.ID, user.TokenVersion)
	if err != nil {
		t.Fatalf("Create token: %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := AuthMiddleware(users, tokens)(next)

	req := httptest.NewRequest("GET", "/api/accounts", nil)
	req.AddCookie(&http.Cookie{Name: security.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	users := newFakeUserRepo()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := AuthMiddleware(users, testTokens())(next)

	req := httptest.NewRequest("GET", "/api/accounts", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	users := newFakeUserRepo()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := AuthMiddleware(users, testTokens())(next)

	req := httptest.NewRequest("GET", "/api/accounts", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body)
	}
}

func TestAuthMiddleware_TokenForUnknownUser(t *testing.T) {
	users := newFakeUserRepo()
	tokens := testTokens()
	user := newTestUser() // never added to the repo
	token, err := tokens.Create(user.ID, user.TokenVersion)
	if err != nil {
		t.Fatalf("Create token: %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := AuthMiddleware(users, tokens)(next)

	req := httptest.NewRequest("GET", "/api/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401 (the user behind the token no longer exists), body=%s", rec.Code, rec.Body)
	}
}

func TestCORSMiddleware_AllowedOrigin(t *testing.T) {
	settings := core.Settings{CORSOrigin: "https://budget.example.com"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := CORSMiddleware(settings)(next)

	req := httptest.NewRequest("GET", "/api/accounts", nil)
	req.Header.Set("Origin", "https://budget.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://budget.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want https://budget.example.com", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want true", got)
	}
}

func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	settings := core.Settings{CORSOrigin: "https://budget.example.com"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := CORSMiddleware(settings)(next)

	req := httptest.NewRequest("GET", "/api/accounts", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for a disallowed origin", got)
	}
}

func TestCORSMiddleware_PreflightRequest(t *testing.T) {
	settings := core.Settings{CORSOrigin: "https://budget.example.com"}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	handler := CORSMiddleware(settings)(next)

	req := httptest.NewRequest("OPTIONS", "/api/accounts", nil)
	req.Header.Set("Origin", "https://budget.example.com")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if called {
		t.Error("a preflight OPTIONS request should not reach the wrapped handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization" {
		t.Errorf("Access-Control-Allow-Headers = %q, want echoed request headers", got)
	}
}
