-- IDs of the records that mods add (strategy section 6.5, decision 0006).
--
-- A record a mod adds, such as the item example.halloween:Pumpkin_Hat,
-- gets the next free ID of its kind in the mod range the first time the
-- server loads it. The row keeps that ID for the name forever:
--   - Rows are never changed or deleted, so an ID is never given to
--     another name, even after its mod is removed.
--   - The IDs live in the same database as the inventories that hold
--     them, so a backup and its restore always agree.

-- +goose Up

CREATE TABLE mod_ids (
    -- The record kind, as server/mod's IDRange.Kind.
    kind text NOT NULL CHECK (kind IN ('item', 'monster', 'skill', 'job', 'quest', 'achievement')),
    -- The record's full name: the mod ID, a colon, and the mod's own
    -- name for it.
    name text NOT NULL CHECK (char_length(name) BETWEEN 3 AND 160 AND position(':' IN name) > 1),
    -- IDs are 32-bit unsigned integers in the game.
    id bigint NOT NULL CHECK (id BETWEEN 1 AND 4294967295),
    -- The mod that added the record when the ID was given.
    mod_id text NOT NULL CHECK (char_length(mod_id) BETWEEN 1 AND 64),
    assigned_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, name),
    UNIQUE (kind, id)
);

-- Rights, when the roles exist (deploy/postgres/roles.sql): waserver
-- reads the IDs and adds new ones. No service role can change or delete
-- one.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT ON mod_ids TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE mod_ids;
