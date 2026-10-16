package web

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"bailleul.dev/budget-server/internal/adapters/security"
	"bailleul.dev/budget-server/internal/core"
	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
)

type fakeOAuthClientRepo struct {
	byID map[string]*entities.OAuthClient
}

func (f *fakeOAuthClientRepo) GetByID(ctx context.Context, id string) (*entities.OAuthClient, error) {
	return f.byID[id], nil
}
func (f *fakeOAuthClientRepo) Add(ctx context.Context, c *entities.OAuthClient) (*entities.OAuthClient, error) {
	f.byID[c.ID] = c
	return c, nil
}

type fakeOAuthCodeRepo struct {
	byHash map[string]*entities.OAuthAuthorizationCode
}

func (f *fakeOAuthCodeRepo) Add(ctx context.Context, c *entities.OAuthAuthorizationCode) error {
	f.byHash[c.CodeHash] = c
	return nil
}
func (f *fakeOAuthCodeRepo) Consume(ctx context.Context, codeHash string) (*entities.OAuthAuthorizationCode, error) {
	c := f.byHash[codeHash]
	delete(f.byHash, codeHash)
	return c, nil
}

type fakeOAuthGrantRepo struct {
	byID    map[uuid.UUID]*entities.OAuthGrant
	clients *fakeOAuthClientRepo
}

func (f *fakeOAuthGrantRepo) find(match func(*entities.OAuthGrant) bool) *entities.OAuthGrant {
	for _, g := range f.byID {
		if match(g) {
			out := *g
			return &out
		}
	}
	return nil
}
func (f *fakeOAuthGrantRepo) Add(ctx context.Context, g *entities.OAuthGrant) (*entities.OAuthGrant, error) {
	g.ClientName = f.clients.byID[g.ClientID].Name
	f.byID[g.ID] = g
	return g, nil
}
func (f *fakeOAuthGrantRepo) GetByAccessTokenHash(ctx context.Context, hash string) (*entities.OAuthGrant, error) {
	return f.find(func(g *entities.OAuthGrant) bool { return g.AccessTokenHash == hash }), nil
}
func (f *fakeOAuthGrantRepo) GetByRefreshTokenHash(ctx context.Context, hash string) (*entities.OAuthGrant, error) {
	return f.find(func(g *entities.OAuthGrant) bool { return g.RefreshTokenHash == hash }), nil
}
func (f *fakeOAuthGrantRepo) Rotate(ctx context.Context, id uuid.UUID, oldRefreshHash string, g *entities.OAuthGrant) (bool, error) {
	existing := f.byID[id]
	if existing == nil || existing.RefreshTokenHash != oldRefreshHash {
		return false, nil
	}
	existing.AccessTokenHash, existing.AccessExpiresAt = g.AccessTokenHash, g.AccessExpiresAt
	existing.RefreshTokenHash, existing.RefreshExpiresAt = g.RefreshTokenHash, g.RefreshExpiresAt
	return true, nil
}
func (f *fakeOAuthGrantRepo) ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.OAuthGrant, error) {
	var out []entities.OAuthGrant
	for _, g := range f.byID {
		if g.UserID == userID {
			out = append(out, *g)
		}
	}
	return out, nil
}
func (f *fakeOAuthGrantRepo) Delete(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	if g := f.byID[id]; g != nil && g.UserID == userID {
		delete(f.byID, id)
		return true, nil
	}
	return false, nil
}

const (
	oauthTestPassword    = "correct horse battery"
	oauthTestRedirectURI = "http://localhost:9999/callback"
	oauthTestVerifier    = "a-sufficiently-long-pkce-code-verifier-0123456789"
)

type oauthE2E struct {
	server  *httptest.Server
	browser *http.Client // keeps cookies, never follows redirects
	user    *entities.User
	account *entities.Account
}

// newOAuthE2E serves the real router — OAuth server, MCP endpoint and all —
// over HTTP, with one password user who owns one account.
func newOAuthE2E(t *testing.T) *oauthE2E {
	t.Helper()
	f := newOperationHandlerFixture()
	users := newFakeUserRepo()
	hasher := security.BcryptPasswordHasher{}
	hash, _ := hasher.Hash(oauthTestPassword)
	user := newTestUser()
	user.PasswordHash = &hash
	users.byID[user.ID] = user
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Compte courant", Currency: "EUR"}
	f.accounts.byID[account.ID] = account

	clients := &fakeOAuthClientRepo{byID: map[string]*entities.OAuthClient{}}
	oauth := services.NewOAuthUseCase(clients, &fakeOAuthCodeRepo{byHash: map[string]*entities.OAuthAuthorizationCode{}},
		&fakeOAuthGrantRepo{byID: map[uuid.UUID]*entities.OAuthGrant{}, clients: clients}, core.SystemClock{})
	settings := core.Settings{JWTSecret: "test-secret", JWTExpireMinutes: 60}
	bankSync, _, _, _ := newTestBankSyncHandler(&fakeBankConnector{})
	router := NewRouter(Handlers{
		Accounts: f.accountHandler,
		MCP:      NewMCPHandler(f.accountHandler, NewCategoryHandler(f.categories, f.accountHandler), f.handler, bankSync),
		OAuth:    NewOAuthHandler(oauth, services.NewAuthUseCase(users, hasher, core.SystemClock{}), users, settings),
	}, users, settings)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &oauthE2E{server: server, browser: browser, user: user, account: account}
}

func (e *oauthE2E) do(t *testing.T, method, path string, form url.Values) (*http.Response, string) {
	t.Helper()
	target := path
	if strings.HasPrefix(path, "/") {
		target = e.server.URL + path
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, _ := http.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := e.browser.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, string(raw)
}

func (e *oauthE2E) postJSON(t *testing.T, path string, body any, dest any) *http.Response {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(e.server.URL+path, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	if dest != nil {
		_ = json.NewDecoder(resp.Body).Decode(dest)
	}
	return resp
}

var csrfPattern = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

func csrfFrom(t *testing.T, page string) string {
	t.Helper()
	m := csrfPattern.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no csrf field in page:\n%s", page)
	}
	return m[1]
}

func (e *oauthE2E) register(t *testing.T) string {
	t.Helper()
	var reg registerClientResponse
	resp := e.postJSON(t, "/oauth/register", map[string]any{"client_name": "Claude Code", "redirect_uris": []string{oauthTestRedirectURI}}, &reg)
	if resp.StatusCode != http.StatusCreated || reg.ClientID == "" {
		t.Fatalf("register: status %d, %+v", resp.StatusCode, reg)
	}
	return reg.ClientID
}

func authorizePath(clientID string) string {
	sum := sha256.Sum256([]byte(oauthTestVerifier))
	return "/oauth/authorize?" + url.Values{
		"client_id": {clientID}, "redirect_uri": {oauthTestRedirectURI}, "response_type": {"code"}, "state": {"xyz"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
	}.Encode()
}

// signInAndApprove walks the browser side: login page, password form, back
// to consent, approve — and returns the redirect to the client's callback.
func (e *oauthE2E) signInAndApprove(t *testing.T, clientID, decision string) *url.URL {
	t.Helper()
	resp, page := e.do(t, "GET", authorizePath(clientID), nil)
	if resp.StatusCode != 200 || !strings.Contains(page, "Connectez-vous") {
		t.Fatalf("authorize: status %d, want the login page:\n%s", resp.StatusCode, page)
	}
	resp, _ = e.do(t, "POST", "/oauth/login", url.Values{"csrf": {csrfFrom(t, page)}, "email": {*e.user.Email}, "password": {oauthTestPassword}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: status %d, want 303", resp.StatusCode)
	}
	resp, _ = e.do(t, "GET", resp.Header.Get("Location"), nil) // /oauth/resume
	resp, page = e.do(t, "GET", resp.Header.Get("Location"), nil)
	if resp.StatusCode != 200 || !strings.Contains(page, "Autoriser Claude Code") {
		t.Fatalf("after login: status %d, want the consent page:\n%s", resp.StatusCode, page)
	}
	resp, _ = e.do(t, "POST", "/oauth/authorize", url.Values{"csrf": {csrfFrom(t, page)}, "decision": {decision}})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("decision: status %d, want 302", resp.StatusCode)
	}
	callback, _ := url.Parse(resp.Header.Get("Location"))
	if !strings.HasPrefix(callback.String(), oauthTestRedirectURI) || callback.Query().Get("state") != "xyz" {
		t.Fatalf("redirected to %s, want the client's callback with its state", callback)
	}
	return callback
}

func (e *oauthE2E) token(t *testing.T, form url.Values) (int, map[string]any) {
	t.Helper()
	resp, err := http.PostForm(e.server.URL+"/oauth/token", form)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func (e *oauthE2E) mcpStatus(t *testing.T, token string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", e.server.URL+"/api/mcp", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestOAuthE2E_ClaudeConnects(t *testing.T) {
	e := newOAuthE2E(t)

	// 1. An unauthenticated MCP call points at the resource metadata…
	resp, err := http.Post(e.server.URL+"/api/mcp", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	challenge := resp.Header.Get("WWW-Authenticate")
	if resp.StatusCode != 401 || !strings.Contains(challenge, `resource_metadata="`+e.server.URL+`/.well-known/oauth-protected-resource/api/mcp"`) {
		t.Fatalf("status %d, WWW-Authenticate %q", resp.StatusCode, challenge)
	}
	// …which names this server as the authorization server.
	var prm map[string]any
	r2, _ := http.Get(e.server.URL + "/.well-known/oauth-protected-resource/api/mcp")
	_ = json.NewDecoder(r2.Body).Decode(&prm)
	r2.Body.Close()
	if prm["resource"] != e.server.URL+"/api/mcp" || prm["authorization_servers"].([]any)[0] != e.server.URL {
		t.Fatalf("protected resource metadata = %v", prm)
	}
	var asm map[string]any
	r3, _ := http.Get(e.server.URL + "/.well-known/oauth-authorization-server")
	_ = json.NewDecoder(r3.Body).Decode(&asm)
	r3.Body.Close()
	if asm["token_endpoint"] != e.server.URL+"/oauth/token" || asm["registration_endpoint"] != e.server.URL+"/oauth/register" {
		t.Fatalf("authorization server metadata = %v", asm)
	}

	// 2. Register, sign in, approve, exchange the code.
	clientID := e.register(t)
	callback := e.signInAndApprove(t, clientID, "approve")
	if callback.Query().Get("iss") != e.server.URL {
		t.Errorf("iss = %q, want %q", callback.Query().Get("iss"), e.server.URL)
	}
	status, tokens := e.token(t, url.Values{
		"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {callback.Query().Get("code")},
		"redirect_uri": {oauthTestRedirectURI}, "code_verifier": {oauthTestVerifier},
	})
	if status != 200 || tokens["token_type"] != "Bearer" {
		t.Fatalf("token exchange: status %d, %v", status, tokens)
	}
	access := tokens["access_token"].(string)

	// 3. The access token works on the MCP endpoint, through the real SDK client.
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: e.server.URL + "/api/mcp", HTTPClient: &http.Client{Transport: bearerTransport{access}},
	}, nil)
	if err != nil {
		t.Fatalf("MCP connect with access token: %v", err)
	}
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_accounts", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("list_accounts: %v, %+v", err, res)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(raw), e.account.ID.String()) {
		t.Fatalf("list_accounts = %s, want the user's account", raw)
	}
	_ = session.Close()

	// …but not on the rest of the API: it's scoped to MCP.
	apiReq, _ := http.NewRequest("GET", e.server.URL+"/api/accounts", nil)
	apiReq.Header.Set("Authorization", "Bearer "+access)
	apiResp, _ := http.DefaultClient.Do(apiReq)
	apiResp.Body.Close()
	if apiResp.StatusCode != 401 {
		t.Errorf("OAuth access token on /api/accounts: status %d, want 401", apiResp.StatusCode)
	}

	// 4. Refresh rotates the pair.
	status, refreshed := e.token(t, url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {tokens["refresh_token"].(string)}})
	if status != 200 {
		t.Fatalf("refresh: status %d, %v", status, refreshed)
	}
	if got := e.mcpStatus(t, access); got != 401 {
		t.Errorf("old access token after refresh: status %d, want 401", got)
	}
	newAccess := refreshed["access_token"].(string)
	if got := e.mcpStatus(t, newAccess); got == 401 {
		t.Error("refreshed access token rejected")
	}

	// 5. The user sees the app in their connected apps and revokes it.
	resp, body := e.do(t, "GET", "/api/oauth/grants", nil)
	var grants []oauthGrantResponse
	_ = json.Unmarshal([]byte(body), &grants)
	if resp.StatusCode != 200 || len(grants) != 1 || grants[0].ClientName != "Claude Code" {
		t.Fatalf("grants: status %d, %s", resp.StatusCode, body)
	}
	if resp, _ := e.do(t, "DELETE", "/api/oauth/grants/"+grants[0].ID, nil); resp.StatusCode != 204 {
		t.Fatalf("revoke: status %d", resp.StatusCode)
	}
	if got := e.mcpStatus(t, newAccess); got != 401 {
		t.Errorf("access token after revoke: status %d, want 401", got)
	}
}

func TestOAuthE2E_Deny(t *testing.T) {
	e := newOAuthE2E(t)
	callback := e.signInAndApprove(t, e.register(t), "deny")
	if callback.Query().Get("error") != "access_denied" || callback.Query().Get("code") != "" {
		t.Fatalf("callback = %s, want error=access_denied and no code", callback)
	}
}

func TestOAuthE2E_WrongPassword(t *testing.T) {
	e := newOAuthE2E(t)
	_, page := e.do(t, "GET", authorizePath(e.register(t)), nil)
	resp, page := e.do(t, "POST", "/oauth/login", url.Values{"csrf": {csrfFrom(t, page)}, "email": {*e.user.Email}, "password": {"nope"}})
	if resp.StatusCode != 401 || !strings.Contains(page, "E-mail ou mot de passe incorrect") {
		t.Fatalf("status %d, want the login page with an error:\n%s", resp.StatusCode, page)
	}
}

func TestOAuthE2E_RejectsMissingCSRF(t *testing.T) {
	e := newOAuthE2E(t)
	e.do(t, "GET", authorizePath(e.register(t)), nil)
	resp, _ := e.do(t, "POST", "/oauth/login", url.Values{"email": {*e.user.Email}, "password": {oauthTestPassword}})
	if resp.StatusCode != 403 {
		t.Fatalf("login without csrf: status %d, want 403", resp.StatusCode)
	}
}

// An unknown client or an unregistered redirect_uri must never be redirected
// to — that would make /oauth/authorize an open redirect.
func TestOAuthE2E_UntrustedRedirectShowsErrorPage(t *testing.T) {
	e := newOAuthE2E(t)
	clientID := e.register(t)
	path := strings.Replace(authorizePath(clientID), url.QueryEscape(oauthTestRedirectURI), url.QueryEscape("https://evil.example.com/cb"), 1)
	resp, page := e.do(t, "GET", path, nil)
	if resp.StatusCode != 400 || resp.Header.Get("Location") != "" || !strings.Contains(page, "Impossible de continuer") {
		t.Fatalf("status %d, Location %q, want the error page", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestOAuthE2E_RegisterRejectsRemoteHTTPRedirect(t *testing.T) {
	e := newOAuthE2E(t)
	var out map[string]string
	resp := e.postJSON(t, "/oauth/register", map[string]any{"redirect_uris": []string{"http://evil.example.com/cb"}}, &out)
	if resp.StatusCode != 400 || out["error"] != "invalid_redirect_uri" {
		t.Fatalf("status %d, %v", resp.StatusCode, out)
	}
}

func TestOAuthE2E_MetadataOpenToAnyOrigin(t *testing.T) {
	e := newOAuthE2E(t)
	req, _ := http.NewRequest("OPTIONS", e.server.URL+"/oauth/token", nil)
	req.Header.Set("Origin", "https://inspector.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("preflight: status %d, ACAO %q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
}
