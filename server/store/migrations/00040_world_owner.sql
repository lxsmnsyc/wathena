-- The world's owner by account ID (proposal 0002, review decision 54,
-- docs/security/review-2026-10.md, "An owner name with no account").
--
-- Rules:
--   - `wactl world new` records the first admin account's ID as the
--     owner, in the same transaction that makes the world and the
--     account. Only the migrator role writes it.
--   - The game role reads it with the world row, and waserver gives the
--     owner role to that account. staff.owners by name still works, with
--     a warning at start.
--   - The account role gets no right on it: waserver reads it once with
--     the game role and passes it on.
--   - The owner's account cannot be deleted while it is the owner: the
--     foreign key refuses it.
--   - Worlds made before this migration have no owner ID. Their owners
--     come from staff.owners.

-- +goose Up

ALTER TABLE world ADD COLUMN owner_account_id bigint REFERENCES accounts (id);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT (owner_account_id) ON world TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

ALTER TABLE world DROP COLUMN owner_account_id;
