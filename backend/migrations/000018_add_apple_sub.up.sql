-- "Sign in with Apple" links an account to Apple's stable subject id, the
-- same way robine_sub/google_sub do for the other providers. Apple users
-- may hide their address behind a private relay, so email stays nullable.
ALTER TABLE users ADD COLUMN apple_sub TEXT UNIQUE;
