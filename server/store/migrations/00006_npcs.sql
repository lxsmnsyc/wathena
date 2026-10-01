-- Save points and the saved variables of mods (phase 3 step 4).
--
-- Rules:
--   - A character's save point is where Butterfly Wings and death take
--     it, as rAthena's save_point. An empty save_map is the world's
--     start point, which new characters have until an NPC saves them.
--   - Mod variables are stored under the mod's ID (strategy section
--     6.7). Each value is JSON: a number, a boolean, a string, or a
--     table of plain data (proposal 0002, "Variables").
--   - A variable at its default value has no row.

-- +goose Up

ALTER TABLE characters
    ADD COLUMN save_map text NOT NULL DEFAULT '' CHECK (char_length(save_map) <= 64),
    ADD COLUMN save_x integer NOT NULL DEFAULT 0 CHECK (save_x BETWEEN 0 AND 1023),
    ADD COLUMN save_y integer NOT NULL DEFAULT 0 CHECK (save_y BETWEEN 0 AND 1023);

-- Variables of one character, written with the character's saves.
CREATE TABLE mod_character_variables (
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    mod_id text NOT NULL CHECK (char_length(mod_id) BETWEEN 1 AND 64),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
    value jsonb NOT NULL CHECK (octet_length(value::text) <= 65536),
    PRIMARY KEY (character_id, mod_id, name)
);

-- Variables of one account, which all its characters share. A
-- character's saves write the ones it changed.
CREATE TABLE mod_account_variables (
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    mod_id text NOT NULL CHECK (char_length(mod_id) BETWEEN 1 AND 64),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
    value jsonb NOT NULL CHECK (octet_length(value::text) <= 65536),
    PRIMARY KEY (account_id, mod_id, name)
);

-- Variables of the whole world, written soon after they change.
CREATE TABLE mod_server_variables (
    mod_id text NOT NULL CHECK (char_length(mod_id) BETWEEN 1 AND 64),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
    value jsonb NOT NULL CHECK (octet_length(value::text) <= 65536),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (mod_id, name)
);

-- Rights, when the roles exist (deploy/postgres/roles.sql): waserver
-- reads and writes variable rows. The account service has no rights on
-- them.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON mod_character_variables TO wathena_game;
        GRANT SELECT, INSERT, UPDATE, DELETE ON mod_account_variables TO wathena_game;
        GRANT SELECT, INSERT, UPDATE, DELETE ON mod_server_variables TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE mod_server_variables;
DROP TABLE mod_account_variables;
DROP TABLE mod_character_variables;
ALTER TABLE characters DROP COLUMN save_y, DROP COLUMN save_x, DROP COLUMN save_map;
