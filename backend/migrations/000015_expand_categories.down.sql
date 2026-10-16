DELETE FROM categories
WHERE name IN ('Car purchase / loan', 'Car maintenance & repair', 'Taxi & ride-hailing',
               'Coffee & bakery', 'Alcohol & tobacco',
               'Optical', 'Dental', 'Therapy',
               'Culture', 'Sports & activities', 'Streaming', 'Musical instruments', 'Creative hobbies',
               'Home & furniture', 'Beauty & care')
  AND parent_id IN (SELECT id FROM categories WHERE parent_id IS NULL
                     AND name IN ('Transport', 'Food & Drink', 'Health', 'Entertainment', 'Shopping'));

DELETE FROM categories
WHERE parent_id IN (SELECT id FROM categories WHERE parent_id IS NULL
                     AND name IN ('Savings & Investments', 'Family & Children', 'Taxes', 'Gifts & Donations', 'Pets'));

DELETE FROM categories
WHERE parent_id IS NULL
  AND name IN ('Savings & Investments', 'Family & Children', 'Taxes', 'Gifts & Donations', 'Pets');
