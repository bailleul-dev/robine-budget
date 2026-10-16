-- Adds finer-grained standard subcategories the seed data from
-- 000004_add_categories.up.sql was missing — water/electricity split out
-- from the catch-all "Utilities", plus internet, phone, and bank fees.
-- A fresh migration rather than editing 000004 in place: that one has
-- already run on every existing database, and golang-migrate never
-- re-applies a migration it has recorded as done.
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), housing.id, sub.name
FROM (SELECT id FROM categories WHERE parent_id IS NULL AND name = 'Housing') AS housing,
     (VALUES ('Water'), ('Electricity')) AS sub(name);

INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), bills.id, sub.name
FROM (SELECT id FROM categories WHERE parent_id IS NULL AND name = 'Bills & Subscriptions') AS bills,
     (VALUES ('Internet'), ('Phone'), ('Bank fees')) AS sub(name);
