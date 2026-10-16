ALTER TABLE planned_items DROP CONSTRAINT planned_items_match_mode_check;

-- Category-mode items have no label by definition — delete them rather than
-- leave rows SET NOT NULL can't reconcile with (there's no meaningful label
-- to invent for them on rollback).
DELETE FROM planned_items WHERE label IS NULL;

ALTER TABLE planned_items ALTER COLUMN label SET NOT NULL;
ALTER TABLE planned_items DROP COLUMN match_mode;

DROP TYPE planned_item_match_mode;
