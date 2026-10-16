ALTER TABLE operations DROP CONSTRAINT fk_operations_category_id;
ALTER TABLE operations DROP COLUMN category_id;
DROP TABLE categories;
