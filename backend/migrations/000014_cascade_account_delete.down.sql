ALTER TABLE planned_occurrences DROP CONSTRAINT planned_occurrences_reconciled_operation_id_fkey;
ALTER TABLE planned_occurrences ADD CONSTRAINT planned_occurrences_reconciled_operation_id_fkey
    FOREIGN KEY (reconciled_operation_id) REFERENCES operations(id);

ALTER TABLE planned_occurrences DROP CONSTRAINT planned_occurrences_planned_item_id_fkey;
ALTER TABLE planned_occurrences ADD CONSTRAINT planned_occurrences_planned_item_id_fkey
    FOREIGN KEY (planned_item_id) REFERENCES planned_items(id);

ALTER TABLE comments DROP CONSTRAINT comments_operation_id_fkey;
ALTER TABLE comments ADD CONSTRAINT comments_operation_id_fkey
    FOREIGN KEY (operation_id) REFERENCES operations(id);

ALTER TABLE balance_checkpoints DROP CONSTRAINT balance_checkpoints_account_id_fkey;
ALTER TABLE balance_checkpoints ADD CONSTRAINT balance_checkpoints_account_id_fkey
    FOREIGN KEY (account_id) REFERENCES accounts(id);

ALTER TABLE operations DROP CONSTRAINT operations_import_batch_id_fkey;
ALTER TABLE operations ADD CONSTRAINT operations_import_batch_id_fkey
    FOREIGN KEY (import_batch_id) REFERENCES import_batches(id);

ALTER TABLE operations DROP CONSTRAINT operations_account_id_fkey;
ALTER TABLE operations ADD CONSTRAINT operations_account_id_fkey
    FOREIGN KEY (account_id) REFERENCES accounts(id);

ALTER TABLE planned_items DROP CONSTRAINT planned_items_account_id_fkey;
ALTER TABLE planned_items ADD CONSTRAINT planned_items_account_id_fkey
    FOREIGN KEY (account_id) REFERENCES accounts(id);

ALTER TABLE import_batches DROP CONSTRAINT import_batches_account_id_fkey;
ALTER TABLE import_batches ADD CONSTRAINT import_batches_account_id_fkey
    FOREIGN KEY (account_id) REFERENCES accounts(id);
