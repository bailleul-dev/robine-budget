DELETE FROM categories
WHERE name IN ('Water', 'Electricity', 'Internet', 'Phone', 'Bank fees')
  AND parent_id IN (SELECT id FROM categories WHERE parent_id IS NULL AND name IN ('Housing', 'Bills & Subscriptions'));
