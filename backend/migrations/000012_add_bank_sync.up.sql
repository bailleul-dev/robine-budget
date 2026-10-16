-- One bank connection per user for now (mono-bank, mono-connection): stores
-- the credentials and the woob session state (cookies + last 2FA
-- validation), both encrypted at rest by the app (see adapters/security
-- SecretBox) - a DB leak alone shouldn't hand out live bank access.
CREATE TABLE bank_connections (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    login_encrypted BYTEA NOT NULL,
    password_encrypted BYTEA NOT NULL,
    -- NULL until the first successful sync produces a session to save.
    session_encrypted BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Lets a sync tell "this woob account already has a matching Account row"
-- from "create one" without fuzzy-matching on name. NULL for accounts
-- created any other way (CSV import, manual).
ALTER TABLE accounts ADD COLUMN bank_sync_account_id TEXT;
CREATE UNIQUE INDEX uq_accounts_user_bank_sync_account_id
    ON accounts (user_id, bank_sync_account_id)
    WHERE bank_sync_account_id IS NOT NULL;
