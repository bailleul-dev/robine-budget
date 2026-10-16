-- Mirrors 029c10c1c5c9_add_balance_checkpoints.py.
CREATE TABLE balance_checkpoints (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    as_of_date DATE NOT NULL,
    balance NUMERIC(14, 2) NOT NULL,
    note VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_balance_checkpoint_account_date UNIQUE (account_id, as_of_date)
);
CREATE INDEX ix_balance_checkpoints_account_id ON balance_checkpoints (account_id);
CREATE INDEX ix_balance_checkpoints_as_of_date ON balance_checkpoints (as_of_date);
