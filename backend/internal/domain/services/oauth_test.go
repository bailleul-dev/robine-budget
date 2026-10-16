package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
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
	byID map[uuid.UUID]*entities.OAuthGrant
}

func (f *fakeOAuthGrantRepo) Add(ctx context.Context, g *entities.OAuthGrant) (*entities.OAuthGrant, error) {
	f.byID[g.ID] = g
	return g, nil
}
func (f *fakeOAuthGrantRepo) GetByAccessTokenHash(ctx context.Context, hash string) (*entities.OAuthGrant, error) {
	for _, g := range f.byID {
		if g.AccessTokenHash == hash {
			return g, nil
		}
	}
	return nil, nil
}
func (f *fakeOAuthGrantRepo) GetByRefreshTokenHash(ctx context.Context, hash string) (*entities.OAuthGrant, error) {
	for _, g := range f.byID {
		if g.RefreshTokenHash == hash {
			return g, nil
		}
	}
	return nil, nil
}
func (f *fakeOAuthGrantRepo) Rotate(ctx context.Context, id uuid.UUID, oldRefreshHash string, g *entities.OAuthGrant) (bool, error) {
	existing := f.byID[id]
	if existing == nil || existing.RefreshTokenHash != oldRefreshHash {
		return false, nil
	}
	existing.AccessTokenHash, existing.AccessExpiresAt = g.AccessTokenHash, g.AccessExpiresAt
	existing.RefreshTokenHash, existing.RefreshExpiresAt = g.RefreshTokenHash, g.RefreshExpiresAt
	existing.RefreshedAt = g.RefreshedAt
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
	g := f.byID[id]
	if g == nil || g.UserID != userID {
		return false, nil
	}
	delete(f.byID, id)
	return true, nil
}

type movableClock struct{ now time.Time }

func (c *movableClock) Now() time.Time    { return c.now }
func (c *movableClock) Today() civil.Date { return civil.FromTime(c.now) }

type oauthFixture struct {
	uc     *OAuthUseCase
	grants *fakeOAuthGrantRepo
	clock  *movableClock
}

func newOAuthFixture() *oauthFixture {
	clock := &movableClock{now: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
	grants := &fakeOAuthGrantRepo{byID: map[uuid.UUID]*entities.OAuthGrant{}}
	uc := NewOAuthUseCase(
		&fakeOAuthClientRepo{byID: map[string]*entities.OAuthClient{}},
		&fakeOAuthCodeRepo{byHash: map[string]*entities.OAuthAuthorizationCode{}},
		grants, clock,
	)
	return &oauthFixture{uc: uc, grants: grants, clock: clock}
}

const testVerifier = "a-sufficiently-long-pkce-code-verifier-0123456789"

func testChallenge() string {
	sum := sha256.Sum256([]byte(testVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func mustOAuthCode(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func wantOAuthError(t *testing.T, err error, code string) {
	t.Helper()
	var oerr *OAuthError
	if !errors.As(err, &oerr) || oerr.Code != code {
		t.Fatalf("err = %v, want OAuth error %q", err, code)
	}
}

// authorize runs registration + an approved authorization request, the
// way the web layer would, and returns the client and the issued code.
func (f *oauthFixture) authorize(t *testing.T, userID uuid.UUID, redirectURI string) (*entities.OAuthClient, string) {
	t.Helper()
	ctx := context.Background()
	client, _, err := f.uc.RegisterClient(ctx, "Claude", []string{"http://localhost:33418/callback"}, false)
	mustOAuthCode(t, err)
	req := AuthorizeRequest{ClientID: client.ID, RedirectURI: redirectURI, ResponseType: "code", CodeChallenge: testChallenge(), CodeChallengeMethod: "S256"}
	_, err = f.uc.ValidateAuthorizeRequest(ctx, req)
	mustOAuthCode(t, err)
	code, err := f.uc.IssueCode(ctx, userID, req)
	mustOAuthCode(t, err)
	return client, code
}

func TestOAuth_FullFlow(t *testing.T) {
	f := newOAuthFixture()
	ctx := context.Background()
	userID := uuid.New()
	client, code := f.authorize(t, userID, "http://localhost:33418/callback")

	tokens, err := f.uc.ExchangeCode(ctx, client.ID, "", code, "http://localhost:33418/callback", testVerifier)
	mustOAuthCode(t, err)
	got, err := f.uc.AuthenticateAccessToken(ctx, tokens.AccessToken)
	mustOAuthCode(t, err)
	if got != userID {
		t.Fatalf("access token resolves to %s, want %s", got, userID)
	}

	// The code is single-use.
	_, err = f.uc.ExchangeCode(ctx, client.ID, "", code, "http://localhost:33418/callback", testVerifier)
	wantOAuthError(t, err, "invalid_grant")

	// Refresh rotates: the new pair works, the old refresh token doesn't.
	refreshed, err := f.uc.Refresh(ctx, client.ID, "", tokens.RefreshToken)
	mustOAuthCode(t, err)
	if _, err := f.uc.AuthenticateAccessToken(ctx, refreshed.AccessToken); err != nil {
		t.Fatalf("refreshed access token rejected: %v", err)
	}
	if _, err := f.uc.AuthenticateAccessToken(ctx, tokens.AccessToken); !errors.Is(err, ErrOAuthInvalidToken) {
		t.Fatalf("old access token still accepted after refresh (err = %v)", err)
	}
	_, err = f.uc.Refresh(ctx, client.ID, "", tokens.RefreshToken)
	wantOAuthError(t, err, "invalid_grant")
}

func TestOAuth_LoopbackRedirectAnyPort(t *testing.T) {
	f := newOAuthFixture()
	client, code := f.authorize(t, uuid.New(), "http://localhost:51234/callback")
	if _, err := f.uc.ExchangeCode(context.Background(), client.ID, "", code, "http://localhost:51234/callback", testVerifier); err != nil {
		t.Fatalf("loopback redirect on another port rejected: %v", err)
	}
}

func TestOAuth_ExchangeRejectsWrongVerifier(t *testing.T) {
	f := newOAuthFixture()
	client, code := f.authorize(t, uuid.New(), "http://localhost:33418/callback")
	_, err := f.uc.ExchangeCode(context.Background(), client.ID, "", code, "http://localhost:33418/callback", "not-the-verifier")
	wantOAuthError(t, err, "invalid_grant")
}

func TestOAuth_ExchangeRejectsExpiredCode(t *testing.T) {
	f := newOAuthFixture()
	client, code := f.authorize(t, uuid.New(), "http://localhost:33418/callback")
	f.clock.now = f.clock.now.Add(11 * time.Minute)
	_, err := f.uc.ExchangeCode(context.Background(), client.ID, "", code, "http://localhost:33418/callback", testVerifier)
	wantOAuthError(t, err, "invalid_grant")
}

func TestOAuth_AccessTokenExpires(t *testing.T) {
	f := newOAuthFixture()
	client, code := f.authorize(t, uuid.New(), "http://localhost:33418/callback")
	tokens, err := f.uc.ExchangeCode(context.Background(), client.ID, "", code, "http://localhost:33418/callback", testVerifier)
	mustOAuthCode(t, err)
	f.clock.now = f.clock.now.Add(61 * time.Minute)
	if _, err := f.uc.AuthenticateAccessToken(context.Background(), tokens.AccessToken); !errors.Is(err, ErrOAuthInvalidToken) {
		t.Fatalf("expired access token accepted (err = %v)", err)
	}
	// …but the refresh token still gets a new one.
	if _, err := f.uc.Refresh(context.Background(), client.ID, "", tokens.RefreshToken); err != nil {
		t.Fatalf("refresh after access expiry: %v", err)
	}
}

func TestOAuth_RevokeGrant(t *testing.T) {
	f := newOAuthFixture()
	ctx := context.Background()
	userID := uuid.New()
	client, code := f.authorize(t, userID, "http://localhost:33418/callback")
	tokens, err := f.uc.ExchangeCode(ctx, client.ID, "", code, "http://localhost:33418/callback", testVerifier)
	mustOAuthCode(t, err)
	grants, err := f.uc.ListGrants(ctx, userID)
	mustOAuthCode(t, err)
	if len(grants) != 1 {
		t.Fatalf("got %d grants, want 1", len(grants))
	}

	if ok, _ := f.uc.RevokeGrant(ctx, uuid.New(), grants[0].ID); ok {
		t.Fatal("another user revoked this user's grant")
	}
	if ok, _ := f.uc.RevokeGrant(ctx, userID, grants[0].ID); !ok {
		t.Fatal("RevokeGrant returned false for the user's own grant")
	}
	if _, err := f.uc.AuthenticateAccessToken(ctx, tokens.AccessToken); !errors.Is(err, ErrOAuthInvalidToken) {
		t.Fatalf("revoked access token still accepted (err = %v)", err)
	}
	_, err = f.uc.Refresh(ctx, client.ID, "", tokens.RefreshToken)
	wantOAuthError(t, err, "invalid_grant")
}

func TestOAuth_ConfidentialClientNeedsSecret(t *testing.T) {
	f := newOAuthFixture()
	ctx := context.Background()
	client, secret, err := f.uc.RegisterClient(ctx, "Web app", []string{"https://claude.ai/api/mcp/auth_callback"}, true)
	mustOAuthCode(t, err)
	req := AuthorizeRequest{ClientID: client.ID, RedirectURI: "https://claude.ai/api/mcp/auth_callback", ResponseType: "code", CodeChallenge: testChallenge(), CodeChallengeMethod: "S256"}
	code, err := f.uc.IssueCode(ctx, uuid.New(), req)
	mustOAuthCode(t, err)

	_, err = f.uc.ExchangeCode(ctx, client.ID, "wrong", code, req.RedirectURI, testVerifier)
	wantOAuthError(t, err, "invalid_client")
	if _, err := f.uc.ExchangeCode(ctx, client.ID, secret, code, req.RedirectURI, testVerifier); err != nil {
		t.Fatalf("exchange with the right secret: %v", err)
	}
}

func TestOAuth_RegisterRejectsUnsafeRedirectURIs(t *testing.T) {
	f := newOAuthFixture()
	for _, uri := range []string{"http://evil.example.com/cb", "javascript:alert(1)", "https://x.example.com/cb#frag", "not a url"} {
		_, _, err := f.uc.RegisterClient(context.Background(), "x", []string{uri}, false)
		wantOAuthError(t, err, "invalid_redirect_uri")
	}
}

func TestOAuth_ValidateAuthorizeRequest(t *testing.T) {
	f := newOAuthFixture()
	ctx := context.Background()
	client, _, err := f.uc.RegisterClient(ctx, "Claude", []string{"https://claude.ai/api/mcp/auth_callback"}, false)
	mustOAuthCode(t, err)
	valid := AuthorizeRequest{ClientID: client.ID, RedirectURI: "https://claude.ai/api/mcp/auth_callback", ResponseType: "code", CodeChallenge: testChallenge(), CodeChallengeMethod: "S256"}

	unknownClient := valid
	unknownClient.ClientID = "nope"
	if _, err := f.uc.ValidateAuthorizeRequest(ctx, unknownClient); !errors.Is(err, ErrOAuthUntrustedRedirect) {
		t.Errorf("unknown client: err = %v, want ErrOAuthUntrustedRedirect", err)
	}
	otherRedirect := valid
	otherRedirect.RedirectURI = "https://evil.example.com/cb"
	if _, err := f.uc.ValidateAuthorizeRequest(ctx, otherRedirect); !errors.Is(err, ErrOAuthUntrustedRedirect) {
		t.Errorf("unregistered redirect: err = %v, want ErrOAuthUntrustedRedirect", err)
	}
	noPKCE := valid
	noPKCE.CodeChallenge = ""
	_, err = f.uc.ValidateAuthorizeRequest(ctx, noPKCE)
	wantOAuthError(t, err, "invalid_request")
	plainPKCE := valid
	plainPKCE.CodeChallengeMethod = "plain"
	_, err = f.uc.ValidateAuthorizeRequest(ctx, plainPKCE)
	wantOAuthError(t, err, "invalid_request")
}
