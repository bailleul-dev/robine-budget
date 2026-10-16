-- Rounds out the standard category taxonomy: five new top-level categories
-- plus subcategories that were missing under the existing ones (car
-- ownership costs, health specialties, streaming/hobbies, etc).
WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Savings & Investments') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Savings transfer'), ('Investments'), ('Crypto')) AS sub(name);

WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Family & Children') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Childcare'), ('School fees'), ('Child support'), ('Children''s activities')) AS sub(name);

WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Taxes') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Income tax'), ('Property tax'), ('Fines')) AS sub(name);

WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Gifts & Donations') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Gifts'), ('Charity & donations')) AS sub(name);

WITH top AS (
    INSERT INTO categories (id, parent_id, name) VALUES (gen_random_uuid(), NULL, 'Pets') RETURNING id
)
INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), top.id, sub.name FROM top, (VALUES ('Veterinary'), ('Pet food & supplies')) AS sub(name);

INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), transport.id, sub.name
FROM (SELECT id FROM categories WHERE parent_id IS NULL AND name = 'Transport') AS transport,
     (VALUES ('Car purchase / loan'), ('Car maintenance & repair'), ('Taxi & ride-hailing')) AS sub(name);

INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), food.id, sub.name
FROM (SELECT id FROM categories WHERE parent_id IS NULL AND name = 'Food & Drink') AS food,
     (VALUES ('Coffee & bakery'), ('Alcohol & tobacco')) AS sub(name);

INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), health.id, sub.name
FROM (SELECT id FROM categories WHERE parent_id IS NULL AND name = 'Health') AS health,
     (VALUES ('Optical'), ('Dental'), ('Therapy')) AS sub(name);

INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), entertainment.id, sub.name
FROM (SELECT id FROM categories WHERE parent_id IS NULL AND name = 'Entertainment') AS entertainment,
     (VALUES ('Culture'), ('Sports & activities'), ('Streaming'), ('Musical instruments'), ('Creative hobbies')) AS sub(name);

INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), shopping.id, sub.name
FROM (SELECT id FROM categories WHERE parent_id IS NULL AND name = 'Shopping') AS shopping,
     (VALUES ('Home & furniture'), ('Beauty & care')) AS sub(name);
