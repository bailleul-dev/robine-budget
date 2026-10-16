-- Mirrors bf46558598fb_add_debit_credit_signed_amount_mode.py. Postgres 12+
-- allows ALTER TYPE ... ADD VALUE inside a transaction as long as the new
-- value isn't used in that same transaction (which this migration doesn't),
-- so — unlike the Alembic version — no autocommit workaround is needed here.
ALTER TYPE amount_mode ADD VALUE IF NOT EXISTS 'debit_credit_signed';
