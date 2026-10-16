// Use-case: an OAuth 2.1 authorization server, just big enough for MCP
// clients (Claude and others) to connect to a user's budget without the
// user pasting a session token around: dynamic client registration,
// authorization code + PKCE (S256 only), refresh token rotation, and
// revocation. The HTTP side — metadata documents, the login/consent pages,
// form parsing — lives in adapters/web; this only holds the rules.
//
// Tokens are opaque random strings (not JWTs), stored as SHA-256 hashes: a
// leaked database doesn't leak usable tokens, and revoking a grant takes
// effect on the very next request instead of when a JWT would expire. They
// carry a prefix so AuthenticateAccessToken's callers can tell one apart
// from a session JWT without a database round trip.
package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
)

const (
	OAuthAccessTokenPrefix  = "bmcp_at_"
	OAuthRefreshTokenPrefix = "bmcp_rt_"

	oauthCodeTTL         = 10 * time.Minute
	oauthAccessTokenTTL  = time.Hour
	oauthRefreshTokenTTL = 90 * 24 * time.Hour
)

// OAuthError is an error the OAuth spec has a name for (RFC 6749 §5.2):
// Code goes out verbatim as the "error" field, Description as
// "error_description".
type OAuthError struct {
	Code        string
	Description string
}

func (e *OAuthError) Error() string { return e.Code + ": " + e.Description }

func oauthErr(code, description string) *OAuthError {
	return &OAuthError{Code: code, Description: description}
}

// ErrOAuthInvalidToken: the access token is unknown, expired or revoked.
var ErrOAuthInvalidToken = errors.New("invalid access token")

type OAuthUseCase struct {
	clients ports.OAuthClientRepository
	codes   ports.OAuthAuthorizationCodeRepository
	grants  ports.OAuthGrantRepository
	clock   ports.Clock
}

func NewOAuthUseCase(clients ports.OAuthClientRepository, codes ports.OAuthAuthorizationCodeRepository, grants ports.OAuthGrantRepository, clock ports.Clock) *OAuthUseCase {
	return &OAuthUseCase{clients: clients, codes: codes, grants: grants, clock: clock}
}

// OAuthTokens is what the token endpoint hands a client.
type OAuthTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
}

// RegisterClient is dynamic client registration (RFC 7591). A confidential
// client gets a secret back — returned once, only its hash is kept.
func (uc *OAuthUseCase) RegisterClient(ctx context.Context, name string, redirectURIs []string, confidential bool) (*entities.OAuthClient, string, error) {
	if len(redirectURIs) == 0 {
		return nil, "", oauthErr("invalid_redirect_uri", "at least one redirect_uri is required")
	}
	for _, uri := range redirectURIs {
		if !isAllowedRedirectURI(uri) {
			return nil, "", oauthErr("invalid_redirect_uri", "redirect_uri must be https, or http on a loopback address: "+uri)
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Unnamed application"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	client := &entities.OAuthClient{ID: randomToken("bmcp_client_"), Name: name, RedirectURIs: redirectURIs, CreatedAt: uc.clock.Now()}
	var secret string
	if confidential {
		secret = randomToken("bmcp_secret_")
		hash := hashToken(secret)
		client.SecretHash = &hash
	}
	created, err := uc.clients.Add(ctx, client)
	if err != nil {
		return nil, "", err
	}
	return created, secret, nil
}

// AuthorizeRequest is what a client asks for at the authorization
// endpoint, before the user has said anything.
type AuthorizeRequest struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	CodeChallenge       string
	CodeChallengeMethod string
}

// ErrOAuthUntrustedRedirect: the client or its redirect_uri can't be
// trusted, so an error must be shown to the user rather than redirected to
// that URI (RFC 6749 §4.1.2.1) — otherwise this would be an open redirect.
var ErrOAuthUntrustedRedirect = errors.New("unknown client or unregistered redirect_uri")

// ValidateAuthorizeRequest checks an authorization request. It returns
// ErrOAuthUntrustedRedirect when the error must not go back to
// redirect_uri, or an *OAuthError that may.
func (uc *OAuthUseCase) ValidateAuthorizeRequest(ctx context.Context, req AuthorizeRequest) (*entities.OAuthClient, error) {
	client, err := uc.clients.GetByID(ctx, req.ClientID)
	if err != nil {
		return nil, err
	}
	if client == nil || !redirectURIRegistered(client, req.RedirectURI) {
		return nil, ErrOAuthUntrustedRedirect
	}
	if req.ResponseType != "code" {
		return client, oauthErr("unsupported_response_type", "only response_type=code is supported")
	}
	if req.CodeChallenge == "" || req.CodeChallengeMethod != "S256" {
		return client, oauthErr("invalid_request", "PKCE with code_challenge_method=S256 is required")
	}
	return client, nil
}

// IssueCode records the user's approval of an (already validated) request
// and returns the authorization code to send to its redirect_uri.
func (uc *OAuthUseCase) IssueCode(ctx context.Context, userID uuid.UUID, req AuthorizeRequest) (string, error) {
	code := randomToken("")
	err := uc.codes.Add(ctx, &entities.OAuthAuthorizationCode{
		CodeHash: hashToken(code), ClientID: req.ClientID, UserID: userID, RedirectURI: req.RedirectURI,
		CodeChallenge: req.CodeChallenge, ExpiresAt: uc.clock.Now().Add(oauthCodeTTL),
	})
	if err != nil {
		return "", err
	}
	return code, nil
}

// ExchangeCode is the authorization_code grant: code + PKCE verifier in,
// a new grant's tokens out.
func (uc *OAuthUseCase) ExchangeCode(ctx context.Context, clientID, clientSecret, code, redirectURI, codeVerifier string) (*OAuthTokens, error) {
	client, err := uc.authenticateClient(ctx, clientID, clientSecret)
	if err != nil {
		return nil, err
	}
	stored, err := uc.codes.Consume(ctx, hashToken(code))
	if err != nil {
		return nil, err
	}
	if stored == nil || stored.ClientID != client.ID || uc.clock.Now().After(stored.ExpiresAt) {
		return nil, oauthErr("invalid_grant", "authorization code is invalid or expired")
	}
	if stored.RedirectURI != redirectURI {
		return nil, oauthErr("invalid_grant", "redirect_uri does not match the authorization request")
	}
	if !pkceMatches(codeVerifier, stored.CodeChallenge) {
		return nil, oauthErr("invalid_grant", "code_verifier does not match code_challenge")
	}

	tokens, grant := uc.newTokens()
	grant.ID = uuid.New()
	grant.ClientID = client.ID
	grant.UserID = stored.UserID
	grant.CreatedAt = grant.RefreshedAt
	if _, err := uc.grants.Add(ctx, grant); err != nil {
		return nil, err
	}
	return tokens, nil
}

// Refresh is the refresh_token grant. Refresh tokens rotate: each one works
// exactly once, and the response carries its replacement.
func (uc *OAuthUseCase) Refresh(ctx context.Context, clientID, clientSecret, refreshToken string) (*OAuthTokens, error) {
	client, err := uc.authenticateClient(ctx, clientID, clientSecret)
	if err != nil {
		return nil, err
	}
	oldHash := hashToken(refreshToken)
	grant, err := uc.grants.GetByRefreshTokenHash(ctx, oldHash)
	if err != nil {
		return nil, err
	}
	if grant == nil || grant.ClientID != client.ID || uc.clock.Now().After(grant.RefreshExpiresAt) {
		return nil, oauthErr("invalid_grant", "refresh token is invalid, expired or revoked")
	}
	tokens, rotated := uc.newTokens()
	ok, err := uc.grants.Rotate(ctx, grant.ID, oldHash, rotated)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, oauthErr("invalid_grant", "refresh token is invalid, expired or revoked")
	}
	return tokens, nil
}

// AuthenticateAccessToken resolves a bearer access token to the user it
// acts for.
func (uc *OAuthUseCase) AuthenticateAccessToken(ctx context.Context, token string) (uuid.UUID, error) {
	if !strings.HasPrefix(token, OAuthAccessTokenPrefix) {
		return uuid.UUID{}, ErrOAuthInvalidToken
	}
	grant, err := uc.grants.GetByAccessTokenHash(ctx, hashToken(token))
	if err != nil {
		return uuid.UUID{}, err
	}
	if grant == nil || uc.clock.Now().After(grant.AccessExpiresAt) {
		return uuid.UUID{}, ErrOAuthInvalidToken
	}
	return grant.UserID, nil
}

func (uc *OAuthUseCase) ListGrants(ctx context.Context, userID uuid.UUID) ([]entities.OAuthGrant, error) {
	return uc.grants.ListForUser(ctx, userID)
}

// RevokeGrant disconnects one app: its access and refresh tokens stop
// working immediately. False if userID has no such grant.
func (uc *OAuthUseCase) RevokeGrant(ctx context.Context, userID, grantID uuid.UUID) (bool, error) {
	return uc.grants.Delete(ctx, grantID, userID)
}

func (uc *OAuthUseCase) authenticateClient(ctx context.Context, clientID, clientSecret string) (*entities.OAuthClient, error) {
	client, err := uc.clients.GetByID(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, oauthErr("invalid_client", "unknown client")
	}
	if client.SecretHash != nil && subtle.ConstantTimeCompare([]byte(hashToken(clientSecret)), []byte(*client.SecretHash)) != 1 {
		return nil, oauthErr("invalid_client", "client authentication failed")
	}
	return client, nil
}

// newTokens mints a fresh access/refresh pair, returned both in the clear
// (for the client) and as the hashes/expiries to store.
func (uc *OAuthUseCase) newTokens() (*OAuthTokens, *entities.OAuthGrant) {
	now := uc.clock.Now()
	access := randomToken(OAuthAccessTokenPrefix)
	refresh := randomToken(OAuthRefreshTokenPrefix)
	return &OAuthTokens{AccessToken: access, RefreshToken: refresh, ExpiresIn: oauthAccessTokenTTL},
		&entities.OAuthGrant{
			AccessTokenHash: hashToken(access), AccessExpiresAt: now.Add(oauthAccessTokenTTL),
			RefreshTokenHash: hashToken(refresh), RefreshExpiresAt: now.Add(oauthRefreshTokenTTL),
			RefreshedAt: now,
		}
}

func randomToken(prefix string) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func pkceMatches(verifier, challenge string) bool {
	if verifier == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(expected), []byte(challenge)) == 1
}

// isAllowedRedirectURI accepts https URLs, and plain http only on a
// loopback address — where native/CLI clients (Claude Code) listen for the
// redirect (RFC 8252 §7.3). Anything else (javascript:, http to a remote
// host…) could leak the authorization code.
func isAllowedRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Fragment != "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		return isLoopbackHost(u.Hostname())
	}
	return false
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// redirectURIRegistered matches exactly, except that a loopback URI may
// use any port (RFC 8252 §7.3): a CLI client picks a free one per login.
func redirectURIRegistered(client *entities.OAuthClient, raw string) bool {
	if slices.Contains(client.RedirectURIs, raw) {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || !isLoopbackHost(u.Hostname()) {
		return false
	}
	for _, registered := range client.RedirectURIs {
		r, err := url.Parse(registered)
		if err == nil && r.Scheme == u.Scheme && r.Hostname() == u.Hostname() && r.Path == u.Path && r.RawQuery == u.RawQuery {
			return true
		}
	}
	return false
}
