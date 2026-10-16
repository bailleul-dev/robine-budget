-- Lets a single `DELETE FROM users WHERE id = $1` (in-app account deletion,
-- required by the App Store) remove everything that user owns: their
-- accounts (which already cascade to all account data, see 000014), their
-- own bank profiles, custom categories and comments. Collaborators and bank
-- connections already cascade (000009, 000012).
--
-- References *other* users' data may hold to what's being deleted are
-- loosened rather than cascaded where cascading would destroy someone
-- else's data: an import batch just forgets which (now deleted) bank
-- profile parsed it. planned_items.category_id can't simply be SET NULL
-- (a category-mode budget requires one, see 000016), so UserRepository.Delete
-- clears those references itself before deleting the user.

ALTER TABLE accounts DROP CONSTRAINT accounts_user_id_fkey;
ALTER TABLE accounts ADD CONSTRAINT accounts_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE bank_profiles DROP CONSTRAINT bank_profiles_user_id_fkey;
ALTER TABLE bank_profiles ADD CONSTRAINT bank_profiles_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE import_batches ALTER COLUMN bank_profile_id DROP NOT NULL;
ALTER TABLE import_batches DROP CONSTRAINT import_batches_bank_profile_id_fkey;
ALTER TABLE import_batches ADD CONSTRAINT import_batches_bank_profile_id_fkey
    FOREIGN KEY (bank_profile_id) REFERENCES bank_profiles(id) ON DELETE SET NULL;

ALTER TABLE comments DROP CONSTRAINT comments_user_id_fkey;
ALTER TABLE comments ADD CONSTRAINT comments_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE categories DROP CONSTRAINT categories_user_id_fkey;
ALTER TABLE categories ADD CONSTRAINT categories_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE categories DROP CONSTRAINT categories_parent_id_fkey;
ALTER TABLE categories ADD CONSTRAINT categories_parent_id_fkey
    FOREIGN KEY (parent_id) REFERENCES categories(id) ON DELETE CASCADE;

ALTER TABLE operation_category_splits DROP CONSTRAINT operation_category_splits_category_id_fkey;
ALTER TABLE operation_category_splits ADD CONSTRAINT operation_category_splits_category_id_fkey
    FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE;
