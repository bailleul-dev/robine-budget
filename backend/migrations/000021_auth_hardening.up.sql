-- Revocable sessions: every session JWT carries the user's token_version
-- ("ver" claim) and is rejected once it no longer matches. Bumping it signs
-- the user out everywhere (logout-all, password reset).
ALTER TABLE users ADD COLUMN token_version INTEGER NOT NULL DEFAULT 0;

-- NULL until the address is proven (email link, or a verified identity from
-- Google / Apple / Robine ID). An unverified password account is never linked
-- automatically to a social login (see AuthUseCase), which closes the
-- "register someone else's address first" pre-hijacking attack.
ALTER TABLE users ADD COLUMN email_verified_at TIMESTAMPTZ;

-- Accounts that exist before this migration were created by people we know:
-- treat their address as verified rather than locking them out of linking.
UPDATE users SET email_verified_at = created_at WHERE email IS NOT NULL;

-- Single-use, expiring tokens for the two emailed flows. Only a SHA-256 of
-- the token is stored, so a database leak doesn't hand out live reset links.
CREATE TABLE auth_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('email_verification', 'password_reset')),
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_auth_tokens_user_kind ON auth_tokens (user_id, kind);
