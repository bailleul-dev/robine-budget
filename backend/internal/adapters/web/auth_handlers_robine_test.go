package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"bailleul.dev/budget-server/internal/core"
)

// robineOIDCFixture spins up a fake OIDC provider (discovery + token +
// userinfo endpoints) so RobineLogin/RobineCallback/discoverRobine can be
// exercised end to end: unlike GoogleLogin/GoogleCallback, every URL this
// handler hits (RobineIssuer, and the token/userinfo endpoints returned by
// the discovery document) is config-driven rather than hardcoded, so a real
// httptest.Server plugged in via settings.RobineIssuer needs no client
// injection at all.
type robineOIDCFixture struct {
	server      *httptest.Server
	tokenErr    bool
	userInfoSub string
	handler     *AuthHandler
}

func newRobineOIDCFixture(t *testing.T) *robineOIDCFixture {
	t.Helper()
	f := &robineOIDCFixture{userInfoSub: "robine-user-1"}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": f.server.URL + "/authorize",
			"token_endpoint":         f.server.URL + "/token",
			"userinfo_endpoint":      f.server.URL + "/userinfo",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if f.tokenErr {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "fake-access-token", "token_type": "Bearer"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"sub": f.userInfoSub, "email": "robine@example.com", "name": "Robine User"})
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)

	f.handler, _ = newAuthHandlerWithFakes(core.Settings{
		RobineClientID: "budget", RobineIssuer: f.server.URL, CORSOrigin: "https://budget.example.com",
	})
	return f
}

func TestAuthHandler_RobineLogin_Success(t *testing.T) {
	f := newRobineOIDCFixture(t)

	req := httptest.NewRequest("GET", "/api/auth/oidc/robine/login?callback="+url.QueryEscape(robineNativeCallbackURL), nil)
	rec := httptest.NewRecorder()
	f.handler.RobineLogin(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302, body=%s", rec.Code, rec.Body)
	}
	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatal("Location header should be set")
	}
	var foundState, foundCallback bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == "robine_oauth_state" && c.Value != "" {
			foundState = true
		}
		if c.Name == "robine_oauth_callback" && c.Value == robineNativeCallbackURL {
			foundCallback = true
		}
	}
	if !foundState {
		t.Error("a robine_oauth_state cookie should have been set")
	}
	if !foundCallback {
		t.Error("a robine_oauth_callback cookie carrying the requested callback should have been set")
	}
}

// runRobineCallback drives RobineLogin then RobineCallback back to back,
// carrying the state/callback cookies RobineLogin set and the query params
// a real Robine redirect would append - the only way to reach
// RobineCallback's state-matching logic without duplicating it.
func runRobineCallback(t *testing.T, f *robineOIDCFixture, code string) *httptest.ResponseRecorder {
	t.Helper()
	loginReq := httptest.NewRequest("GET", "/api/auth/oidc/robine/login?callback="+url.QueryEscape(robineNativeCallbackURL), nil)
	loginRec := httptest.NewRecorder()
	f.handler.RobineLogin(loginRec, loginReq)
	cookies := loginRec.Result().Cookies()

	callbackReq := httptest.NewRequest("GET", "/api/auth/oidc/robine/callback?code="+code+"&state="+cookieValue(cookies, "robine_oauth_state"), nil)
	for _, c := range cookies {
		callbackReq.AddCookie(c)
	}
	callbackRec := httptest.NewRecorder()
	f.handler.RobineCallback(callbackRec, callbackReq)
	return callbackRec
}

func cookieValue(cookies []*http.Cookie, name string) string {
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func TestAuthHandler_RobineCallback_Success(t *testing.T) {
	f := newRobineOIDCFixture(t)

	rec := runRobineCallback(t, f, "auth-code")

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302, body=%s", rec.Code, rec.Body)
	}
	location := rec.Header().Get("Location")
	if location == "" || !strings.Contains(location, "token=") {
		t.Fatalf("Location = %q, want it to carry a token= query param", location)
	}
}

func TestAuthHandler_RobineCallback_StateMismatch(t *testing.T) {
	f := newRobineOIDCFixture(t)

	loginReq := httptest.NewRequest("GET", "/api/auth/oidc/robine/login?callback="+url.QueryEscape(robineNativeCallbackURL), nil)
	loginRec := httptest.NewRecorder()
	f.handler.RobineLogin(loginRec, loginReq)

	callbackReq := httptest.NewRequest("GET", "/api/auth/oidc/robine/callback?code=auth-code&state=wrong-state", nil)
	for _, c := range loginRec.Result().Cookies() {
		callbackReq.AddCookie(c)
	}
	callbackRec := httptest.NewRecorder()
	f.handler.RobineCallback(callbackRec, callbackReq)

	if callbackRec.Code != 400 {
		t.Fatalf("status = %d, want 400 (state mismatch), body=%s", callbackRec.Code, callbackRec.Body)
	}
}

func TestAuthHandler_RobineCallback_TokenExchangeFailure(t *testing.T) {
	f := newRobineOIDCFixture(t)
	f.tokenErr = true

	rec := runRobineCallback(t, f, "auth-code")

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (token exchange failed), body=%s", rec.Code, rec.Body)
	}
}

func TestAuthHandler_RobineCallback_MissingCode(t *testing.T) {
	f := newRobineOIDCFixture(t)

	loginReq := httptest.NewRequest("GET", "/api/auth/oidc/robine/login?callback="+url.QueryEscape(robineNativeCallbackURL), nil)
	loginRec := httptest.NewRecorder()
	f.handler.RobineLogin(loginRec, loginReq)
	cookies := loginRec.Result().Cookies()

	callbackReq := httptest.NewRequest("GET", "/api/auth/oidc/robine/callback?state="+cookieValue(cookies, "robine_oauth_state"), nil)
	for _, c := range cookies {
		callbackReq.AddCookie(c)
	}
	callbackRec := httptest.NewRecorder()
	f.handler.RobineCallback(callbackRec, callbackReq)

	if callbackRec.Code != 400 {
		t.Fatalf("status = %d, want 400 (missing code), body=%s", callbackRec.Code, callbackRec.Body)
	}
}

func TestAuthHandler_RobineCallback_MissingStateCookie(t *testing.T) {
	f := newRobineOIDCFixture(t)

	loginReq := httptest.NewRequest("GET", "/api/auth/oidc/robine/login?callback="+url.QueryEscape(robineNativeCallbackURL), nil)
	loginRec := httptest.NewRecorder()
	f.handler.RobineLogin(loginRec, loginReq)

	callbackReq := httptest.NewRequest("GET", "/api/auth/oidc/robine/callback?code=auth-code&state=abc", nil)
	// Only carry the callback cookie, not the state one.
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "robine_oauth_callback" {
			callbackReq.AddCookie(c)
		}
	}
	callbackRec := httptest.NewRecorder()
	f.handler.RobineCallback(callbackRec, callbackReq)

	if callbackRec.Code != 400 {
		t.Fatalf("status = %d, want 400 (missing state cookie), body=%s", callbackRec.Code, callbackRec.Body)
	}
}
