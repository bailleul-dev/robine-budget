-- Restores the pre-000017 constraints. Fails if any import batch has lost
-- its bank profile in the meantime (bank_profile_id NULL).

ALTER TABLE operation_category_splits DROP CONSTRAINT operation_category_splits_category_id_fkey;
ALTER TABLE operation_category_splits ADD CONSTRAINT operation_category_splits_category_id_fkey
    FOREIGN KEY (category_id) REFERENCES categories(id);

ALTER TABLE categories DROP CONSTRAINT categories_parent_id_fkey;
ALTER TABLE categories ADD CONSTRAINT categories_parent_id_fkey
    FOREIGN KEY (parent_id) REFERENCES categories(id);

ALTER TABLE categories DROP CONSTRAINT categories_user_id_fkey;
ALTER TABLE categories ADD CONSTRAINT categories_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE comments DROP CONSTRAINT comments_user_id_fkey;
ALTER TABLE comments ADD CONSTRAINT comments_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE import_batches DROP CONSTRAINT import_batches_bank_profile_id_fkey;
ALTER TABLE import_batches ALTER COLUMN bank_profile_id SET NOT NULL;
ALTER TABLE import_batches ADD CONSTRAINT import_batches_bank_profile_id_fkey
    FOREIGN KEY (bank_profile_id) REFERENCES bank_profiles(id);

ALTER TABLE bank_profiles DROP CONSTRAINT bank_profiles_user_id_fkey;
ALTER TABLE bank_profiles ADD CONSTRAINT bank_profiles_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE accounts DROP CONSTRAINT accounts_user_id_fkey;
ALTER TABLE accounts ADD CONSTRAINT accounts_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);
