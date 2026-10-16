-- OAuth 2.1 authorization server for the MCP endpoint (see
-- internal/domain/services/oauth.go). Every token and code is stored as a
-- SHA-256 hash, never in the clear. Everything cascades from users, so
-- deleting an account also disconnects every app it authorized.

CREATE TABLE oauth_clients (
    id TEXT PRIMARY KEY,
    -- NULL for a public client (PKCE only), set for a confidential one.
    secret_hash TEXT,
    name TEXT NOT NULL,
    redirect_uris TEXT[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE oauth_authorization_codes (
    code_hash TEXT PRIMARY KEY,
    client_id TEXT NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    redirect_uri TEXT NOT NULL,
    code_challenge TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE oauth_grants (
    id UUID PRIMARY KEY,
    client_id TEXT NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    access_token_hash TEXT NOT NULL UNIQUE,
    access_expires_at TIMESTAMPTZ NOT NULL,
    refresh_token_hash TEXT NOT NULL UNIQUE,
    refresh_expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    refreshed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_oauth_grants_user_id ON oauth_grants (user_id);
