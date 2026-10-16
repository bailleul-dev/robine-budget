-- Splits the standard "Alcohol & tobacco" subcategory of Food & Drink (from
-- 000015) in two. The existing row is renamed to "Alcohol", keeping its id,
-- so everything already categorized under it stays put; a new "Tobacco"
-- sibling takes the operations whose label says so ("tabac", as
-- auto-categorization matched them — see categorization.go).
UPDATE categories SET name = 'Alcohol'
WHERE user_id IS NULL AND name = 'Alcohol & tobacco'
  AND parent_id = (SELECT id FROM categories WHERE user_id IS NULL AND parent_id IS NULL AND name = 'Food & Drink');

INSERT INTO categories (id, parent_id, name)
SELECT gen_random_uuid(), id, 'Tobacco'
FROM categories WHERE user_id IS NULL AND parent_id IS NULL AND name = 'Food & Drink';

UPDATE operations SET category_id = tobacco.id
FROM (SELECT c.id FROM categories c JOIN categories p ON p.id = c.parent_id
      WHERE c.user_id IS NULL AND c.name = 'Tobacco' AND p.name = 'Food & Drink') AS tobacco,
     (SELECT c.id FROM categories c JOIN categories p ON p.id = c.parent_id
      WHERE c.user_id IS NULL AND c.name = 'Alcohol' AND p.name = 'Food & Drink') AS alcohol
WHERE operations.category_id = alcohol.id
  AND lower(operations.label) LIKE '%tabac%';
