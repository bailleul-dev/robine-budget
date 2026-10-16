-- Mirrors backend/alembic/versions/441ee91c40bf_initial_schema.py exactly
-- (same tables, columns, constraint and index names) so a fresh database
-- built from these Go migrations matches one built from the Python
-- backend's Alembic history.

CREATE TYPE amount_mode AS ENUM ('single', 'debit_credit');
CREATE TYPE planned_item_kind AS ENUM ('charge', 'income');
CREATE TYPE recurrence_type AS ENUM ('one_off', 'perpetual', 'fixed_count', 'every_x_weeks');
CREATE TYPE occurrence_status AS ENUM ('forecast', 'reconciled', 'skipped');

CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(255) UNIQUE,
    password_hash VARCHAR(255),
    robine_sub VARCHAR(255) UNIQUE,
    display_name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE accounts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    name VARCHAR(255) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_accounts_user_id ON accounts (user_id);

CREATE TABLE bank_profiles (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    name VARCHAR(255) NOT NULL,
    delimiter VARCHAR(1) NOT NULL,
    encoding VARCHAR(32) NOT NULL,
    date_format VARCHAR(32) NOT NULL,
    decimal_separator VARCHAR(1) NOT NULL,
    amount_mode amount_mode NOT NULL,
    column_mapping JSON NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_bank_profiles_user_id ON bank_profiles (user_id);

CREATE TABLE import_batches (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    bank_profile_id UUID NOT NULL REFERENCES bank_profiles(id),
    filename VARCHAR(500) NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    row_count INTEGER NOT NULL,
    imported_count INTEGER NOT NULL,
    duplicate_count INTEGER NOT NULL,
    errors JSON NOT NULL
);
CREATE INDEX ix_import_batches_account_id ON import_batches (account_id);

CREATE TABLE planned_items (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    kind planned_item_kind NOT NULL,
    label VARCHAR(255) NOT NULL,
    amount NUMERIC(14, 2) NOT NULL,
    start_date DATE NOT NULL,
    recurrence_type recurrence_type NOT NULL,
    occurrence_count INTEGER,
    interval_weeks INTEGER,
    day_of_month INTEGER,
    active BOOLEAN NOT NULL,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_planned_items_account_id ON planned_items (account_id);

CREATE TABLE operations (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    import_batch_id UUID REFERENCES import_batches(id),
    date DATE NOT NULL,
    label VARCHAR(500) NOT NULL,
    amount NUMERIC(14, 2) NOT NULL,
    balance_after NUMERIC(14, 2),
    raw_reference VARCHAR(255),
    dedup_hash VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_operation_account_dedup_hash UNIQUE (account_id, dedup_hash),
    CONSTRAINT uq_operation_account_reference UNIQUE (account_id, raw_reference)
);
CREATE INDEX ix_operations_account_id ON operations (account_id);
CREATE INDEX ix_operations_date ON operations (date);

CREATE TABLE comments (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES operations(id),
    user_id UUID NOT NULL REFERENCES users(id),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_comments_operation_id ON comments (operation_id);

CREATE TABLE planned_occurrences (
    id UUID PRIMARY KEY,
    planned_item_id UUID NOT NULL REFERENCES planned_items(id),
    due_date DATE NOT NULL,
    expected_amount NUMERIC(14, 2) NOT NULL,
    status occurrence_status NOT NULL,
    reconciled_operation_id UUID REFERENCES operations(id),
    reconciled_amount NUMERIC(14, 2),
    CONSTRAINT uq_occurrence_item_due_date UNIQUE (planned_item_id, due_date),
    CONSTRAINT uq_occurrence_reconciled_operation UNIQUE (reconciled_operation_id)
);
CREATE INDEX ix_planned_occurrences_due_date ON planned_occurrences (due_date);
CREATE INDEX ix_planned_occurrences_planned_item_id ON planned_occurrences (planned_item_id);
