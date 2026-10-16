DROP TABLE auth_tokens;
ALTER TABLE users DROP COLUMN email_verified_at;
ALTER TABLE users DROP COLUMN token_version;
