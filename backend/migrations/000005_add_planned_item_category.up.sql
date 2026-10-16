ALTER TABLE planned_items ADD COLUMN category_id UUID REFERENCES categories(id);
