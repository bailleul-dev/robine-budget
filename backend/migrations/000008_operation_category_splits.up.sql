CREATE TABLE operation_category_splits (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id),
    amount NUMERIC(14, 2) NOT NULL
);
CREATE INDEX ix_operation_category_splits_operation_id ON operation_category_splits (operation_id);
