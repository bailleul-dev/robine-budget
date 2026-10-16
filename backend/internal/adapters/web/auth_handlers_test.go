package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bailleul.dev/budget-server/internal/core"
)

func newTestAuthHandler(corsOrigin string) *AuthHandler {
	return newTestAuthHandlerWithSettings(core.Settings{CORSOrigin: corsOrigin, JWTSecret: "test-secret", JWTExpireMinutes: 60})
}

// A nil *services.AuthUseCase is safe for every test in this file: none of
// getRedirectURI/getRobineRedirectURI/isAllowedRobineCallback ever call
// through h.auth.
func newTestAuthHandlerWithSettings(settings core.Settings) *AuthHandler {
	return NewAuthHandler(nil, settings)
}

func TestIsAllowedRobineCallback(t *testing.T) {
	h := newTestAuthHandler("https://lagune.bailleul.dev")

	cases := []struct {
		name     string
		callback string
		want     bool
	}{
		{"native app scheme", "dev.bailleul.budget://auth-callback", true},
		{"the old generic scheme (interceptable by any app)", "mobile://auth-callback", false},
		{"web callback matching CORSOrigin", "https://lagune.bailleul.dev/auth-callback", true},
		{"empty", "", false},
		{"wrong path on the right origin", "https://lagune.bailleul.dev/other-page", false},
		{"right path, wrong origin", "https://evil.com/auth-callback", false},
		{"http instead of https", "http://lagune.bailleul.dev/auth-callback", false},
		{"trailing slash", "https://lagune.bailleul.dev/auth-callback/", false},
		{"a completely unrelated custom scheme", "otherapp://auth-callback", false},
		{"this server's own OAuth consent flow", "https://api.lagune.test/oauth/resume", true},
		{"another server's OAuth consent flow", "https://evil.com/oauth/resume", false},
	}
	req := httptest.NewRequest("GET", "https://api.lagune.test/api/auth/oidc/robine/login", nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.isAllowedRobineCallback(req, tc.callback); got != tc.want {
				t.Errorf("isAllowedRobineCallback(%q) = %v, want %v", tc.callback, got, tc.want)
			}
		})
	}
}

// The allowlist is derived from this handler's own CORSOrigin - a
// differently configured deployment (different domain) must not accept a
// callback built from the *other* one's origin.
func TestIsAllowedRobineCallback_DifferentCORSOrigin(t *testing.T) {
	h := newTestAuthHandler("https://staging.example.com")
	req := httptest.NewRequest("GET", "/", nil)
	if h.isAllowedRobineCallback(req, "https://lagune.bailleul.dev/auth-callback") {
		t.Error("a callback matching a different deployment's origin must not be allowed")
	}
	if !h.isAllowedRobineCallback(req, "https://staging.example.com/auth-callback") {
		t.Error("a callback matching this handler's own CORSOrigin should be allowed")
	}
}

func TestGetRedirectURI_PrefersConfiguredValue(t *testing.T) {
	h := newTestAuthHandlerWithSettings(core.Settings{GoogleRedirectURI: "https://lagune.bailleul.dev/api/auth/google/callback"})
	req := httptest.NewRequest("GET", "/api/auth/google/login", nil)
	if got := h.getRedirectURI(req); got != "https://lagune.bailleul.dev/api/auth/google/callback" {
		t.Errorf("got %q, want the explicitly configured GoogleRedirectURI", got)
	}
}

func TestGetRedirectURI_FallsBackToRequestHost(t *testing.T) {
	h := newTestAuthHandlerWithSettings(core.Settings{})
	req := httptest.NewRequest("GET", "/api/auth/google/login", nil)
	req.Host = "localhost:8000"
	if got := h.getRedirectURI(req); got != "http://localhost:8000/api/auth/google/callback" {
		t.Errorf("got %q, want a plain-http fallback built from the request host", got)
	}
}

func TestGetRedirectURI_FallsBackHTTPSBehindProxy(t *testing.T) {
	h := newTestAuthHandlerWithSettings(core.Settings{})
	req := httptest.NewRequest("GET", "/api/auth/google/login", nil)
	req.Host = "lagune.bailleul.dev"
	req.Header.Set("X-Forwarded-Proto", "https")
	if got := h.getRedirectURI(req); got != "https://lagune.bailleul.dev/api/auth/google/callback" {
		t.Errorf("got %q, want an https fallback when X-Forwarded-Proto says so", got)
	}
}

func TestGetRobineRedirectURI_PrefersConfiguredValue(t *testing.T) {
	h := newTestAuthHandlerWithSettings(core.Settings{RobineRedirectURI: "https://lagune.bailleul.dev/api/auth/oidc/robine/callback"})
	req := httptest.NewRequest("GET", "/api/auth/oidc/robine/login", nil)
	if got := h.getRobineRedirectURI(req); got != "https://lagune.bailleul.dev/api/auth/oidc/robine/callback" {
		t.Errorf("got %q, want the explicitly configured RobineRedirectURI", got)
	}
}

func TestGetRobineRedirectURI_FallsBackToRequestHost(t *testing.T) {
	h := newTestAuthHandlerWithSettings(core.Settings{})
	req := httptest.NewRequest("GET", "/api/auth/oidc/robine/login", nil)
	req.Host = "localhost:8000"
	if got := h.getRobineRedirectURI(req); got != "http://localhost:8000/api/auth/oidc/robine/callback" {
		t.Errorf("got %q, want a plain-http fallback built from the request host", got)
	}
}

func TestAuthHandler_Config(t *testing.T) {
	cases := []struct {
		name       string
		settings   core.Settings
		wantGoogle bool
		wantRobine bool
		wantApple  bool
	}{
		{"nothing configured", core.Settings{}, false, false, false},
		{"google only", core.Settings{GoogleClientID: "web-client-id"}, true, false, false},
		{"robine needs both issuer and client id", core.Settings{RobineClientID: "budget"}, false, false, false},
		{"robine fully configured", core.Settings{RobineClientID: "budget", RobineIssuer: "https://id.bailleul.dev/default"}, false, true, false},
		{"apple bundle id", core.Settings{AppleBundleIDs: []string{"dev.bailleul.budget"}}, false, false, true},
		{"all configured", core.Settings{GoogleClientID: "web-client-id", RobineClientID: "budget", RobineIssuer: "https://id.bailleul.dev/default", AppleBundleIDs: []string{"dev.bailleul.budget"}}, true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestAuthHandlerWithSettings(tc.settings)
			rec := httptest.NewRecorder()
			h.Config(rec, httptest.NewRequest(http.MethodGet, "/api/auth/config", nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			var body authConfigResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.GoogleEnabled != tc.wantGoogle {
				t.Errorf("GoogleEnabled = %v, want %v", body.GoogleEnabled, tc.wantGoogle)
			}
			if body.RobineEnabled != tc.wantRobine {
				t.Errorf("RobineEnabled = %v, want %v", body.RobineEnabled, tc.wantRobine)
			}
			if body.AppleEnabled != tc.wantApple {
				t.Errorf("AppleEnabled = %v, want %v", body.AppleEnabled, tc.wantApple)
			}
		})
	}
}

func TestAuthHandler_AppleMobileLogin_Refuses(t *testing.T) {
	post := func(h *AuthHandler, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/apple/mobile", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.AppleMobileLogin(rec, req)
		return rec
	}

	unconfigured := newTestAuthHandlerWithSettings(core.Settings{JWTSecret: "test-secret"})
	if rec := post(unconfigured, `{"identity_token":"whatever"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unconfigured: status = %d, want 400", rec.Code)
	}

	configured := newTestAuthHandlerWithSettings(core.Settings{JWTSecret: "test-secret", AppleBundleIDs: []string{"dev.bailleul.budget"}})
	if rec := post(configured, `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("no identity_token: status = %d, want 400", rec.Code)
	}
	// Not a JWT at all: refused without ever reaching Apple.
	if rec := post(configured, `{"identity_token":"not-a-jwt"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("malformed token: status = %d, want 401", rec.Code)
	}
}
