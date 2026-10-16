package entities

import (
	"time"

	"github.com/google/uuid"
)

// OAuthClient is an application (Claude, an MCP client…) that registered
// itself through OAuth dynamic client registration to act on a user's
// behalf over the MCP endpoint. A public client (SecretHash nil) proves
// itself with PKCE alone; a confidential one also sends its secret.
type OAuthClient struct {
	ID           string
	SecretHash   *string
	Name         string
	RedirectURIs []string
	CreatedAt    time.Time
}

// OAuthAuthorizationCode is the short-lived, single-use code handed to a
// client's redirect_uri once the user approves it — exchanged at the token
// endpoint, together with the PKCE verifier, for an OAuthGrant's tokens.
// Only the code's SHA-256 is stored.
type OAuthAuthorizationCode struct {
	CodeHash      string
	ClientID      string
	UserID        uuid.UUID
	RedirectURI   string
	CodeChallenge string
	ExpiresAt     time.Time
}

// OAuthGrant is one approval of one client by one user: the access token
// the client presents on every MCP request, and the refresh token it trades
// for a new pair when that expires. Both are opaque and stored as SHA-256
// hashes only; deleting the row revokes the client immediately.
type OAuthGrant struct {
	ID       uuid.UUID
	ClientID string
	// ClientName is the client's registered name, joined in on reads for
	// display (the "connected apps" list); not written.
	ClientName       string
	UserID           uuid.UUID
	AccessTokenHash  string
	AccessExpiresAt  time.Time
	RefreshTokenHash string
	RefreshExpiresAt time.Time
	CreatedAt        time.Time
	RefreshedAt      time.Time
}
