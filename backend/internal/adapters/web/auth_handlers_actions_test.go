package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bailleul.dev/budget-server/internal/core"
	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
)

func newAuthHandlerWithFakes(settings core.Settings) (*AuthHandler, *fakeUserRepo) {
	users := newFakeUserRepo()
	auth := services.NewAuthUseCase(users, fakePasswordHasher{}, newFakeClock(civil.Today()))
	if settings.JWTExpireMinutes == 0 {
		settings.JWTExpireMinutes = 60
	}
	if settings.JWTSecret == "" {
		settings.JWTSecret = "test-secret"
	}
	return NewAuthHandler(auth, settings), users
}

func TestAuthHandler_Register_Success(t *testing.T) {
	h, users := newAuthHandlerWithFakes(core.Settings{})

	body := registerRequest{Email: "new@example.com", Password: "hunter2-hunter2", DisplayName: "New User"}
	req := jsonRequest(t, "POST", "/api/auth/register", body, nil)
	rec := httptest.NewRecorder()
	h.Register(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp authResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Email == nil || *resp.Email != "new@example.com" {
		t.Errorf("Email = %v, want new@example.com", resp.Email)
	}
	if resp.Token == "" {
		t.Error("Token should not be empty")
	}
	if len(users.byID) != 1 {
		t.Fatalf("got %d users stored, want 1", len(users.byID))
	}
	if rec.Result().Cookies() == nil || len(rec.Result().Cookies()) == 0 {
		t.Error("a session cookie should have been set")
	}
}

func TestAuthHandler_Register_DuplicateEmail(t *testing.T) {
	h, users := newAuthHandlerWithFakes(core.Settings{})
	email := "existing@example.com"
	users.byID[newTestUser().ID] = &entities.User{Email: &email, DisplayName: "Existing"}

	body := registerRequest{Email: "existing@example.com", Password: "hunter2-hunter2", DisplayName: "New"}
	req := jsonRequest(t, "POST", "/api/auth/register", body, nil)
	rec := httptest.NewRecorder()
	h.Register(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
}

func TestAuthHandler_Login_Success(t *testing.T) {
	h, users := newAuthHandlerWithFakes(core.Settings{})
	hash, _ := fakePasswordHasher{}.Hash("hunter2-hunter2")
	email := "user@example.com"
	user := &entities.User{ID: newTestUser().ID, Email: &email, PasswordHash: &hash, DisplayName: "User"}
	users.byID[user.ID] = user

	body := loginRequest{Email: "user@example.com", Password: "hunter2-hunter2"}
	req := jsonRequest(t, "POST", "/api/auth/login", body, nil)
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp authResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Token == "" {
		t.Error("Token should not be empty")
	}
}

func TestAuthHandler_Login_WrongPassword(t *testing.T) {
	h, users := newAuthHandlerWithFakes(core.Settings{})
	hash, _ := fakePasswordHasher{}.Hash("hunter2-hunter2")
	email := "user@example.com"
	user := &entities.User{ID: newTestUser().ID, Email: &email, PasswordHash: &hash, DisplayName: "User"}
	users.byID[user.ID] = user

	body := loginRequest{Email: "user@example.com", Password: "wrong-password"}
	req := jsonRequest(t, "POST", "/api/auth/login", body, nil)
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body)
	}
}

func TestAuthHandler_Login_UnknownEmail(t *testing.T) {
	h, _ := newAuthHandlerWithFakes(core.Settings{})

	body := loginRequest{Email: "nobody@example.com", Password: "hunter2-hunter2"}
	req := jsonRequest(t, "POST", "/api/auth/login", body, nil)
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body)
	}
}

func TestAuthHandler_Logout_ClearsCookie(t *testing.T) {
	h, _ := newAuthHandlerWithFakes(core.Settings{})

	req := jsonRequest(t, "POST", "/api/auth/logout", nil, nil)
	rec := httptest.NewRecorder()
	h.Logout(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatalf("got cookies %+v, want one cookie with a negative MaxAge (clearing it)", cookies)
	}
}

func TestAuthHandler_Me(t *testing.T) {
	h, _ := newAuthHandlerWithFakes(core.Settings{})
	email := "user@example.com"
	user := &entities.User{ID: newTestUser().ID, Email: &email, DisplayName: "User"}

	req := asUser(jsonRequest(t, "GET", "/api/auth/me", nil, nil), user)
	rec := httptest.NewRecorder()
	h.Me(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp userResponse
	decodeJSONBody(t, rec, &resp)
	if resp.DisplayName != "User" {
		t.Errorf("DisplayName = %q, want User", resp.DisplayName)
	}
}

func TestAuthHandler_GoogleLogin_NotConfigured(t *testing.T) {
	h, _ := newAuthHandlerWithFakes(core.Settings{})

	req := jsonRequest(t, "GET", "/api/auth/google/login", nil, nil)
	rec := httptest.NewRecorder()
	h.GoogleLogin(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
}

func TestAuthHandler_GoogleLogin_RedirectsToGoogle(t *testing.T) {
	h, _ := newAuthHandlerWithFakes(core.Settings{GoogleClientID: "g-client"})

	req := httptest.NewRequest("GET", "/api/auth/google/login", nil)
	rec := httptest.NewRecorder()
	h.GoogleLogin(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302, body=%s", rec.Code, rec.Body)
	}
	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatal("Location header should be set")
	}
	foundStateCookie := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "google_oauth_state" && c.Value != "" {
			foundStateCookie = true
		}
	}
	if !foundStateCookie {
		t.Error("a google_oauth_state cookie should have been set")
	}
}

func TestAuthHandler_GoogleLogin_IgnoresUnsafeRedirect(t *testing.T) {
	h, _ := newAuthHandlerWithFakes(core.Settings{GoogleClientID: "g-client"})

	req := httptest.NewRequest("GET", "/api/auth/google/login?redirect=https://evil.example.com", nil)
	rec := httptest.NewRecorder()
	h.GoogleLogin(rec, req)

	for _, c := range rec.Result().Cookies() {
		if c.Name == "google_oauth_redirect" {
			t.Errorf("an unsafe (absolute) redirect should never be stashed in a cookie, got %q", c.Value)
		}
	}
}

func TestAuthHandler_RobineLogin_NotConfigured(t *testing.T) {
	h, _ := newAuthHandlerWithFakes(core.Settings{})

	req := jsonRequest(t, "GET", "/api/auth/oidc/robine/login?callback=dev.bailleul.budget://auth-callback", nil, nil)
	rec := httptest.NewRecorder()
	h.RobineLogin(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
}

func TestAuthHandler_RobineLogin_RejectsUnknownCallback(t *testing.T) {
	h, _ := newAuthHandlerWithFakes(core.Settings{RobineClientID: "r-client", RobineIssuer: "https://id.example.com"})

	req := jsonRequest(t, "GET", "/api/auth/oidc/robine/login?callback=https://evil.example.com/steal-token", nil, nil)
	rec := httptest.NewRecorder()
	h.RobineLogin(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (callback not on the allowlist), body=%s", rec.Code, rec.Body)
	}
}

func TestAuthHandler_RobineCallback_MissingCallbackCookie(t *testing.T) {
	h, _ := newAuthHandlerWithFakes(core.Settings{RobineClientID: "r-client", RobineIssuer: "https://id.example.com"})

	req := jsonRequest(t, "GET", "/api/auth/oidc/robine/callback?code=abc&state=xyz", nil, nil)
	rec := httptest.NewRecorder()
	h.RobineCallback(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (missing callback cookie), body=%s", rec.Code, rec.Body)
	}
}

func TestAuthHandler_DeleteMe(t *testing.T) {
	h, users := newAuthHandlerWithFakes(core.Settings{})
	user := newTestUser()
	users.byID[user.ID] = user

	req := asUser(jsonRequest(t, "DELETE", "/api/auth/me", nil, nil), user)
	rec := httptest.NewRecorder()
	h.DeleteMe(rec, req)

	if rec.Code != 204 {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body)
	}
	if _, ok := users.byID[user.ID]; ok {
		t.Fatal("user should have been deleted")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatalf("got cookies %+v, want the session cookie cleared", cookies)
	}
}
