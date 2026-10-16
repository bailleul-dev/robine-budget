CREATE TYPE planned_item_match_mode AS ENUM ('label', 'category');

ALTER TABLE planned_items ADD COLUMN match_mode planned_item_match_mode NOT NULL DEFAULT 'label';
ALTER TABLE planned_items ALTER COLUMN label DROP NOT NULL;

-- A label-mode item matches operations by exact label, a category-mode item
-- ("budget" in the UI) matches by category and aggregates every operation in
-- it over each period instead of reconciling one operation at a time — see
-- ReconcileUseCase.CategoryBudgetActuals. Exactly one of the two match keys
-- must be set, matching the mode.
ALTER TABLE planned_items ADD CONSTRAINT planned_items_match_mode_check CHECK (
  (match_mode = 'label' AND label IS NOT NULL) OR
  (match_mode = 'category' AND category_id IS NOT NULL)
);
