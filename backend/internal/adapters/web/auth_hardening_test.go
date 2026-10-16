package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/adapters/security"
	"bailleul.dev/budget-server/internal/core"
	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
)

// --- fakes ------------------------------------------------------------------------

type memTokenRepo struct {
	mu       sync.Mutex
	tokens   []*memToken
	consumed int
}

type memToken struct {
	entities.AuthToken
	used bool
}

func (r *memTokenRepo) Create(_ context.Context, t *entities.AuthToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens = append(r.tokens, &memToken{AuthToken: *t})
	return nil
}
func (r *memTokenRepo) Consume(_ context.Context, kind entities.AuthTokenKind, hash []byte, now time.Time) (uuid.UUID, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tokens {
		if t.Kind == kind && string(t.TokenHash) == string(hash) && !t.used && t.ExpiresAt.After(now) {
			t.used = true
			r.consumed++
			return t.UserID, true, nil
		}
	}
	return uuid.Nil, false, nil
}
func (r *memTokenRepo) InvalidateForUser(_ context.Context, userID uuid.UUID, kind entities.AuthTokenKind, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tokens {
		if t.UserID == userID && t.Kind == kind {
			t.used = true
		}
	}
	return nil
}

type memMailer struct {
	mu   sync.Mutex
	sent []string
}

func (m *memMailer) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, body)
	return nil
}
func (m *memMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

type emailHarness struct {
	h      *AuthHandler
	users  *fakeUserRepo
	tokens *memTokenRepo
	mailer *memMailer
}

func newEmailHarness() emailHarness {
	users := newFakeUserRepo()
	tokens := &memTokenRepo{}
	mailer := &memMailer{}
	auth := services.NewAuthUseCase(users, fakePasswordHasher{}, newFakeClock(civil.Today())).
		WithEmailFlows(services.EmailFlows{Tokens: tokens, Mailer: mailer, PublicURL: "https://lagune.test"})
	settings := core.Settings{JWTSecret: "test-secret-at-least-32-bytes-long", JWTExpireMinutes: 60}
	return emailHarness{NewAuthHandler(auth, settings), users, tokens, mailer}
}

func lastToken(t *testing.T, m *memMailer) string {
	t.Helper()
	body := m.sent[len(m.sent)-1]
	i := strings.Index(body, "?token=")
	if i < 0 {
		t.Fatalf("no token in mail: %q", body)
	}
	return strings.Fields(body[i+len("?token="):])[0]
}

func formPost(target string, values url.Values) *http.Request {
	r := httptest.NewRequest("POST", target, strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

// --- sessions ---------------------------------------------------------------------

func TestAuthMiddleware_RevokedTokenVersionIsRejected(t *testing.T) {
	users := newFakeUserRepo()
	user := newTestUser()
	users.byID[user.ID] = user
	tokens := testTokens()
	token, _ := tokens.Create(user.ID, user.TokenVersion)

	protected := AuthMiddleware(users, tokens)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	call := func() int {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/x", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		protected.ServeHTTP(rec, r)
		return rec.Code
	}
	if got := call(); got != 200 {
		t.Fatalf("before revocation: status = %d, want 200", got)
	}
	user.TokenVersion++ // logout-all / password reset
	if got := call(); got != 401 {
		t.Fatalf("after revocation: status = %d, want 401", got)
	}
}

func TestAuthMiddleware_LegacyTokenWithoutVersionStillWorksAtVersionZero(t *testing.T) {
	users := newFakeUserRepo()
	user := newTestUser()
	users.byID[user.ID] = user
	tokens := testTokens()
	legacy, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.ID.String(), "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(tokens.Secret))

	protected := AuthMiddleware(users, tokens)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/x", nil)
	r.Header.Set("Authorization", "Bearer "+legacy)
	protected.ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Fatalf("a session issued before this change must survive the deploy: status = %d", rec.Code)
	}
	user.TokenVersion = 1
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, r)
	if rec.Code != 401 {
		t.Fatalf("...but is revoked by the first bump: status = %d", rec.Code)
	}
}

func TestMCPAuthMiddleware_RevokedSessionTokenIsRejected(t *testing.T) {
	users := newFakeUserRepo()
	user := newTestUser()
	users.byID[user.ID] = user
	tokens := testTokens()
	token, _ := tokens.Create(user.ID, 0)
	user.TokenVersion = 3

	mw := MCPAuthMiddleware(nil, users, tokens)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/mcp", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	mw.ServeHTTP(rec, r)
	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401 for a revoked session token on the MCP endpoint", rec.Code)
	}
}

func TestAuthHandler_LogoutAll(t *testing.T) {
	e := newEmailHarness()
	user := newTestUser()
	e.users.byID[user.ID] = user

	rec := httptest.NewRecorder()
	req := jsonRequest(t, "POST", "/api/auth/logout-all", nil, nil)
	e.h.LogoutAll(rec, req.WithContext(withUser(req.Context(), user)))
	if rec.Code != 200 || user.TokenVersion != 1 {
		t.Fatalf("status = %d, token version = %d, want 200 and 1", rec.Code, user.TokenVersion)
	}
	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == security.SessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("the session cookie should be cleared")
	}
}

// --- registration / login -----------------------------------------------------------

func TestAuthHandler_Register_WeakPasswordIs400WithReason(t *testing.T) {
	e := newEmailHarness()
	rec := httptest.NewRecorder()
	e.h.Register(rec, jsonRequest(t, "POST", "/api/auth/register", registerRequest{Email: "a@example.com", Password: "short", DisplayName: "A"}, nil))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "at least 10 characters") {
		t.Fatalf("status = %d body = %s, want 400 with the policy reason", rec.Code, rec.Body)
	}
	if len(e.users.byID) != 0 {
		t.Error("no user should have been created")
	}
}

func TestAuthHandler_Login_ThrottlesRepeatedFailuresPerAddress(t *testing.T) {
	e := newEmailHarness()
	email := "victim@example.com"
	e.users.byID[uuid.New()] = &entities.User{ID: uuid.New(), Email: &email, PasswordHash: strPtr("hashed:right-password-123"), DisplayName: "V"}

	attempt := func(password, ip string) int {
		rec := httptest.NewRecorder()
		req := jsonRequest(t, "POST", "/api/auth/login", loginRequest{Email: email, Password: password}, nil)
		req.RemoteAddr = ip + ":1000"
		e.h.Login(rec, req)
		return rec.Code
	}
	for i := 0; i < 5; i++ {
		if got := attempt("wrong-password-1", "198.51.100.1"); got != 401 {
			t.Fatalf("failure %d: status = %d, want 401", i+1, got)
		}
	}
	if got := attempt("right-password-123", "198.51.100.1"); got != 429 {
		t.Errorf("after 5 failures even the right password is throttled: status = %d, want 429", got)
	}
	if got := attempt("right-password-123", "203.0.113.9"); got != 200 {
		t.Errorf("the real owner from another address must not be locked out: status = %d, want 200", got)
	}
}

func TestAuthHandler_Login_SuccessDoesNotConsumeTheBudget(t *testing.T) {
	e := newEmailHarness()
	email := "me@example.com"
	e.users.byID[uuid.New()] = &entities.User{ID: uuid.New(), Email: &email, PasswordHash: strPtr("hashed:right-password-123"), DisplayName: "Me"}
	for i := 0; i < 20; i++ {
		rec := httptest.NewRecorder()
		req := jsonRequest(t, "POST", "/api/auth/login", loginRequest{Email: email, Password: "right-password-123"}, nil)
		req.RemoteAddr = "198.51.100.5:1"
		e.h.Login(rec, req)
		if rec.Code != 200 {
			t.Fatalf("login %d: status = %d, want 200", i+1, rec.Code)
		}
	}
}

func TestRouter_AuthEndpointsAreRateLimitedPerIP(t *testing.T) {
	e := newEmailHarness()
	router := NewRouter(Handlers{Auth: e.h}, e.users, core.Settings{})
	post := func(ip, xff string) int {
		rec := httptest.NewRecorder()
		req := jsonRequest(t, "POST", "/api/auth/forgot-password", forgotPasswordRequest{Email: "x@example.com"}, nil)
		req.RemoteAddr = ip + ":9"
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		router.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < 10; i++ {
		if got := post("198.51.100.7", ""); got != 200 {
			t.Fatalf("request %d: status = %d, want 200", i+1, got)
		}
	}
	if got := post("198.51.100.7", ""); got != 429 {
		t.Errorf("11th request from one IP: status = %d, want 429", got)
	}
	// With no trusted proxy configured, a spoofed X-Forwarded-For must not
	// buy a fresh budget.
	if got := post("198.51.100.7", "1.2.3.4"); got != 429 {
		t.Errorf("spoofed X-Forwarded-For: status = %d, want 429", got)
	}
	if got := post("203.0.113.50", ""); got != 200 {
		t.Errorf("another IP: status = %d, want 200", got)
	}
}

// --- forgot password -------------------------------------------------------------------

func TestForgotPassword_SameAnswerForKnownAndUnknownAddress(t *testing.T) {
	e := newEmailHarness()
	email := "known@example.com"
	e.users.byID[uuid.New()] = &entities.User{ID: uuid.New(), Email: &email, PasswordHash: strPtr("hashed:x-password-123"), DisplayName: "K"}

	ask := func(addr string) (int, string) {
		rec := httptest.NewRecorder()
		e.h.ForgotPassword(rec, jsonRequest(t, "POST", "/api/auth/forgot-password", forgotPasswordRequest{Email: addr}, nil))
		return rec.Code, rec.Body.String()
	}
	c1, b1 := ask("known@example.com")
	c2, b2 := ask("ghost@example.com")
	if c1 != 200 || c2 != 200 || b1 != b2 {
		t.Fatalf("answers differ: (%d %q) vs (%d %q)", c1, b1, c2, b2)
	}
	deadline := time.Now().Add(2 * time.Second)
	for e.mailer.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if e.mailer.count() != 1 {
		t.Errorf("sent %d mails, want exactly 1 (only the known address)", e.mailer.count())
	}
}

func TestForgotPassword_CapsMailsPerAddress(t *testing.T) {
	e := newEmailHarness()
	email := "known@example.com"
	e.users.byID[uuid.New()] = &entities.User{ID: uuid.New(), Email: &email, PasswordHash: strPtr("hashed:x-password-123"), DisplayName: "K"}
	for i := 0; i < 10; i++ {
		rec := httptest.NewRecorder()
		e.h.ForgotPassword(rec, jsonRequest(t, "POST", "/api/auth/forgot-password", forgotPasswordRequest{Email: email}, nil))
		if rec.Code != 200 {
			t.Fatalf("status = %d: the cap must not be observable", rec.Code)
		}
	}
	time.Sleep(200 * time.Millisecond)
	if n := e.mailer.count(); n > 3 {
		t.Errorf("sent %d reset mails to one address, want at most 3", n)
	}
}

// --- pages -----------------------------------------------------------------------------------

func TestPages_GETNeverConsumesTheToken(t *testing.T) {
	e := newEmailHarness()
	rec := httptest.NewRecorder()
	e.h.Register(rec, jsonRequest(t, "POST", "/api/auth/register", registerRequest{Email: "new@example.com", Password: "long-enough-password", DisplayName: "N"}, nil))
	token := lastToken(t, e.mailer)

	// A mail scanner pre-fetching the link:
	rec = httptest.NewRecorder()
	e.h.VerifyEmailPage(rec, httptest.NewRequest("GET", "/verify-email?token="+url.QueryEscape(token), nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `method="post"`) {
		t.Fatalf("GET: status = %d, want the confirmation form", rec.Code)
	}
	if e.tokens.consumed != 0 {
		t.Fatal("a GET consumed the token; a link scanner would burn it")
	}

	rec = httptest.NewRecorder()
	e.h.VerifyEmailSubmit(rec, formPost("/verify-email", url.Values{"token": {token}}))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Adresse confirmée") {
		t.Fatalf("POST: status = %d body = %s", rec.Code, rec.Body)
	}
	for _, u := range e.users.byID {
		if !u.EmailVerified() {
			t.Error("user should be verified after the POST")
		}
	}

	rec = httptest.NewRecorder()
	e.h.VerifyEmailSubmit(rec, formPost("/verify-email", url.Values{"token": {token}}))
	if rec.Code != 400 {
		t.Errorf("replaying the link: status = %d, want 400", rec.Code)
	}
}

func TestResetPage_MismatchDoesNotConsumeAndSuccessRevokesSessions(t *testing.T) {
	e := newEmailHarness()
	email := "me@example.com"
	user := &entities.User{ID: uuid.New(), Email: &email, PasswordHash: strPtr("hashed:old-password-123"), DisplayName: "Me"}
	e.users.byID[user.ID] = user
	rec := httptest.NewRecorder()
	e.h.ForgotPassword(rec, jsonRequest(t, "POST", "/x", forgotPasswordRequest{Email: email}, nil))
	deadline := time.Now().Add(2 * time.Second)
	for e.mailer.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	token := lastToken(t, e.mailer)

	rec = httptest.NewRecorder()
	e.h.ResetPasswordSubmit(rec, formPost("/reset-password", url.Values{"token": {token}, "password": {"new-password-123"}, "confirm": {"different-123456"}}))
	if rec.Code != 400 || e.tokens.consumed != 0 {
		t.Fatalf("mismatch: status = %d consumed = %d, want 400 and an untouched token", rec.Code, e.tokens.consumed)
	}

	rec = httptest.NewRecorder()
	e.h.ResetPasswordSubmit(rec, formPost("/reset-password", url.Values{"token": {token}, "password": {"new-password-123"}, "confirm": {"new-password-123"}}))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Mot de passe modifié") {
		t.Fatalf("reset: status = %d body = %s", rec.Code, rec.Body)
	}
	if user.TokenVersion != 1 {
		t.Errorf("token version = %d, want 1 (all sessions revoked)", user.TokenVersion)
	}
	if user.PasswordHash == nil || *user.PasswordHash != "hashed:new-password-123" {
		t.Errorf("password not updated: %v", user.PasswordHash)
	}
}

func TestPages_EscapeTheTokenAndSetStrictHeaders(t *testing.T) {
	e := newEmailHarness()
	rec := httptest.NewRecorder()
	e.h.ResetPasswordPage(rec, httptest.NewRequest("GET", "/reset-password?token="+url.QueryEscape(`"><script>alert(1)</script>`), nil))
	body := rec.Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("the token was reflected unescaped")
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "script-src") {
		t.Errorf("CSP = %q, want default-src 'none' and no scripts allowed", csp)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("recovery pages must not be cached")
	}
}

func TestPages_MissingTokenIs400(t *testing.T) {
	e := newEmailHarness()
	for name, fn := range map[string]http.HandlerFunc{"verify": e.h.VerifyEmailPage, "reset": e.h.ResetPasswordPage} {
		rec := httptest.NewRecorder()
		fn(rec, httptest.NewRequest("GET", "/x", nil))
		if rec.Code != 400 {
			t.Errorf("%s page without a token: status = %d, want 400", name, rec.Code)
		}
	}
}

// --- private beta ---------------------------------------------------------------------------

func TestAuthHandler_Register_ClosedIs403WithAFrenchMessage(t *testing.T) {
	users := newFakeUserRepo()
	auth := services.NewAuthUseCase(users, fakePasswordHasher{}, newFakeClock(civil.Today())).
		WithSignupPolicy(func(email string) bool { return email == "invited@example.com" })
	h := NewAuthHandler(auth, core.Settings{JWTSecret: "test-secret-at-least-32-bytes-long", JWTExpireMinutes: 60})

	rec := httptest.NewRecorder()
	h.Register(rec, jsonRequest(t, "POST", "/api/auth/register", registerRequest{Email: "stranger@example.com", Password: "long-enough-password", DisplayName: "S"}, nil))
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "bêta privée") {
		t.Fatalf("status = %d body = %s, want 403 with the private-beta message", rec.Code, rec.Body)
	}
	if len(users.byID) != 0 {
		t.Error("an account was created")
	}

	rec = httptest.NewRecorder()
	h.Register(rec, jsonRequest(t, "POST", "/api/auth/register", registerRequest{Email: "invited@example.com", Password: "long-enough-password", DisplayName: "I"}, nil))
	if rec.Code != 200 {
		t.Fatalf("an invited address: status = %d body = %s, want 200", rec.Code, rec.Body)
	}
}
