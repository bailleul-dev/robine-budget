-- Folds "Tobacco" back into "Alcohol & tobacco". Anything pointing at
-- Tobacco (operations, splits, planned items) moves to the merged category
-- first, so the delete can't hit a foreign key.
WITH food AS (
    SELECT id FROM categories WHERE user_id IS NULL AND parent_id IS NULL AND name = 'Food & Drink'
), tobacco AS (
    SELECT id FROM categories WHERE user_id IS NULL AND name = 'Tobacco' AND parent_id = (SELECT id FROM food)
), alcohol AS (
    SELECT id FROM categories WHERE user_id IS NULL AND name = 'Alcohol' AND parent_id = (SELECT id FROM food)
), ops AS (
    UPDATE operations SET category_id = (SELECT id FROM alcohol) WHERE category_id = (SELECT id FROM tobacco)
), splits AS (
    UPDATE operation_category_splits SET category_id = (SELECT id FROM alcohol) WHERE category_id = (SELECT id FROM tobacco)
)
UPDATE planned_items SET category_id = (SELECT id FROM alcohol) WHERE category_id = (SELECT id FROM tobacco);

DELETE FROM categories
WHERE user_id IS NULL AND name = 'Tobacco'
  AND parent_id = (SELECT id FROM categories WHERE user_id IS NULL AND parent_id IS NULL AND name = 'Food & Drink');

UPDATE categories SET name = 'Alcohol & tobacco'
WHERE user_id IS NULL AND name = 'Alcohol'
  AND parent_id = (SELECT id FROM categories WHERE user_id IS NULL AND parent_id IS NULL AND name = 'Food & Drink');
