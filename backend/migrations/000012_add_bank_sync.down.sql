DROP INDEX IF EXISTS uq_accounts_user_bank_sync_account_id;
ALTER TABLE accounts DROP COLUMN IF EXISTS bank_sync_account_id;
DROP TABLE IF EXISTS bank_connections;
