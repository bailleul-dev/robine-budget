package web

import (
	"net/http/httptest"
	"testing"

	"bailleul.dev/budget-server/internal/adapters/security"
	"bailleul.dev/budget-server/internal/core"
)

func TestNewRouter_HealthCheck(t *testing.T) {
	router := NewRouter(Handlers{}, newFakeUserRepo(), core.Settings{})

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
}

func TestNewRouter_ProtectedRouteRequiresAuth(t *testing.T) {
	router := NewRouter(Handlers{}, newFakeUserRepo(), core.Settings{})

	req := httptest.NewRequest("GET", "/api/accounts", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401 (no session token), body=%s", rec.Code, rec.Body)
	}
}

func TestNewRouter_ProtectedRouteWithValidSession(t *testing.T) {
	users := newFakeUserRepo()
	user := newTestUser()
	users.byID[user.ID] = user
	tokens := security.SessionTokens{Secret: "test-secret", ExpireMinutes: 60}
	token, err := tokens.Create(user.ID, user.TokenVersion)
	if err != nil {
		t.Fatalf("Create token: %v", err)
	}
	accountHandler, _, _, _ := newTestAccountHandler()
	router := NewRouter(Handlers{Accounts: accountHandler}, users, core.Settings{JWTSecret: "test-secret", JWTExpireMinutes: 60})

	req := httptest.NewRequest("GET", "/api/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (authenticated, reaches the real handler), body=%s", rec.Code, rec.Body)
	}
}

// An app checks for updates before anyone signs in, so the OTA routes must
// never sit behind AuthMiddleware (which would answer 401 here).
func TestNewRouter_OTAUpdatesArePublic(t *testing.T) {
	root := t.TempDir()
	publishOTAFixture(t, root, "1700000000")
	h, _ := newOTATestHandler(t, root, false)
	router := NewRouter(Handlers{OTAUpdates: h}, newFakeUserRepo(), core.Settings{})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, manifestRequest(nil))
	if rec.Code != 200 {
		t.Fatalf("manifest: status = %d, want 200, body=%s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/updates/assets/1.0.0/1700000000/assets/f00?ext=png", nil))
	if rec.Code != 200 {
		t.Fatalf("asset: status = %d, want 200", rec.Code)
	}
}

func TestNewRouter_OTAUpdatesDisabled(t *testing.T) {
	router := NewRouter(Handlers{}, newFakeUserRepo(), core.Settings{})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, manifestRequest(nil))
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404 when OTA isn't configured", rec.Code)
	}
}
