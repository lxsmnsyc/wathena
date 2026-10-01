-- The world itself, and staff accounts (strategy sections 3.9 and 8.9).
--
-- `wactl world new` fills both: one world row with its secret root seed,
-- and the first staff account. waserver refuses to start without the
-- world row, outside --dev.

-- +goose Up

-- The world. There is one row, with id 1.
CREATE TABLE world (
    id smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
    -- The rAthena mode of the base ruleset: renewal or pre-renewal.
    -- waserver refuses to start with another mode.
    ruleset text NOT NULL CHECK (ruleset IN ('renewal', 'pre-renewal')),
    -- The secret root of the seed tree (docs/specs/rng-v1.md). Generated
    -- worlds derive their maps from it. It never leaves the server.
    seed bytea NOT NULL CHECK (octet_length(seed) = 32),
    -- The version of the map generator the world was made with
    -- (strategy section 8.4). 0 is no generator: classic maps only.
    generator_version integer NOT NULL DEFAULT 0 CHECK (generator_version >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Staff accounts get staff tools in later phases (strategy section 7.8).
-- Only the migrator role sets it: the service roles may only insert the
-- columns below.
ALTER TABLE accounts ADD COLUMN staff boolean NOT NULL DEFAULT false;

-- Rights, when the roles exist (deploy/postgres/roles.sql):
--   - wathena_game reads the world without its seed, which no game code
--     needs yet.
--   - Both service roles insert accounts by column, so neither can make
--     a staff account.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT (id, name, ruleset, generator_version, created_at) ON world TO wathena_game;
        REVOKE INSERT ON accounts FROM wathena_game;
        GRANT INSERT (name, dev) ON accounts TO wathena_game;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_account') THEN
        REVOKE INSERT ON accounts FROM wathena_account;
        GRANT INSERT (name) ON accounts TO wathena_account;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT INSERT ON accounts TO wathena_game;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_account') THEN
        GRANT INSERT ON accounts TO wathena_account;
    END IF;
END
$$;
-- +goose StatementEnd
ALTER TABLE accounts DROP COLUMN staff;
DROP TABLE world;
