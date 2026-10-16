-- Mirrors 166a0d50fe28_add_categories.py, including the standard category
-- seed data (same names, same two levels) — user_id/parent_id NULL marks a
-- standard, shared category rather than one owned by any single user.
CREATE TABLE categories (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id),
    parent_id UUID REFERENCES categories(id),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_category_user_parent_name UNIQUE (user_id, parent_id, name)
);
CREATE INDEX ix_categories_parent_id ON categories (parent_id);
CREATE INDEX ix_categories_user_id ON categories (user_id);

ALTER TABLE operations ADD COLUMN category_id UUID;
ALTER TABLE operations
    ADD CONSTRAINT fk_operations_category_id
    FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE SET NULL;

-- gen_random_uuid() is built into Postgres 16+ (pg_catalog), no extension needed.
WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Income') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Salary'), ('Other income')) AS sub(name);

WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Housing') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Rent / mortgage'), ('Utilities')) AS sub(name);

WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Transport') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Public transit'), ('Fuel & parking')) AS sub(name);

WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Food & Drink') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Groceries'), ('Restaurants')) AS sub(name);

WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Shopping') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Clothing'), ('Electronics')) AS sub(name);

WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Bills & Subscriptions') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Insurance'), ('Subscriptions')) AS sub(name);

WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Health') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Doctor & pharmacy'), ('Fitness')) AS sub(name);

WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Entertainment') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Leisure'), ('Travel')) AS sub(name);

INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Other');
