-- Accounts, characters and their inventories.
--
-- Rules from strategy section 3.5:
--   - bigint for IDs, zeny and exp, timestamptz for times.
--   - Every table has a primary key, and references are foreign keys.
--   - CHECK constraints reject negative and overflowing values, even when
--     the server has a bug.
--   - Every item stack or instance has a unique serial.

-- +goose Up

-- An account. The account service (roadmap step 2) adds its passwords,
-- sessions and settings in tables of their own that reference it.
CREATE TABLE accounts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 24),
    created_at timestamptz NOT NULL DEFAULT now(),
    -- dev is set for accounts that waserver --dev made on first use. They
    -- have no password and must not exist on a public server.
    dev boolean NOT NULL DEFAULT false
);

-- Names are unique without regard to case.
CREATE UNIQUE INDEX accounts_name_key ON accounts (lower(name));

-- A character, as rAthena's char table without equipment, skills and
-- social data, which later migrations add.
CREATE TABLE characters (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    -- slot is the place on the character select screen, below MAX_CHARS.
    slot smallint NOT NULL CHECK (slot BETWEEN 0 AND 14),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 24),
    job integer NOT NULL CHECK (job >= 0),
    -- sex is 0 for female and 1 for male, as rAthena's SEX_FEMALE and
    -- SEX_MALE.
    sex smallint NOT NULL CHECK (sex IN (0, 1)),
    hair integer NOT NULL DEFAULT 0 CHECK (hair >= 0),
    hair_color integer NOT NULL DEFAULT 0 CHECK (hair_color >= 0),
    cloth_color integer NOT NULL DEFAULT 0 CHECK (cloth_color >= 0),
    -- MAX_LEVEL is 275.
    base_level integer NOT NULL CHECK (base_level BETWEEN 1 AND 275),
    job_level integer NOT NULL CHECK (job_level BETWEEN 1 AND 275),
    base_exp bigint NOT NULL DEFAULT 0 CHECK (base_exp >= 0),
    job_exp bigint NOT NULL DEFAULT 0 CHECK (job_exp >= 0),
    status_points integer NOT NULL DEFAULT 0 CHECK (status_points >= 0),
    skill_points integer NOT NULL DEFAULT 0 CHECK (skill_points >= 0),
    -- Raised stats, without job bonuses. rAthena keeps them in an
    -- unsigned short.
    str integer NOT NULL CHECK (str BETWEEN 1 AND 65535),
    agi integer NOT NULL CHECK (agi BETWEEN 1 AND 65535),
    vit integer NOT NULL CHECK (vit BETWEEN 1 AND 65535),
    int integer NOT NULL CHECK (int BETWEEN 1 AND 65535),
    dex integer NOT NULL CHECK (dex BETWEEN 1 AND 65535),
    luk integer NOT NULL CHECK (luk BETWEEN 1 AND 65535),
    -- hp is 0 for a character that logged out dead.
    hp integer NOT NULL CHECK (hp >= 0),
    sp integer NOT NULL CHECK (sp >= 0),
    -- map is the zone's map name, such as map/prontera. x and y are its
    -- cells, which are at most 1024 per side.
    map text NOT NULL CHECK (char_length(map) BETWEEN 1 AND 64),
    x integer NOT NULL CHECK (x BETWEEN 0 AND 1023),
    y integer NOT NULL CHECK (y BETWEEN 0 AND 1023),
    -- MAX_ZENY.
    zeny bigint NOT NULL DEFAULT 0 CHECK (zeny BETWEEN 0 AND 2147483647),
    -- inventory_slots: the slots a character may fill, up to
    -- MAX_INVENTORY.
    inventory_size smallint NOT NULL DEFAULT 100 CHECK (inventory_size BETWEEN 0 AND 200),
    -- save_version goes up at each load and each save. A save names the
    -- version it expects, so an older session cannot overwrite newer
    -- data.
    save_version bigint NOT NULL DEFAULT 0 CHECK (save_version >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    login_at timestamptz,
    saved_at timestamptz,
    UNIQUE (account_id, slot)
);

CREATE UNIQUE INDEX characters_name_key ON characters (lower(name));

CREATE INDEX characters_account_id ON characters (account_id);

-- A stack or item instance in a character's inventory.
CREATE TABLE inventory (
    -- serial identifies the stack or instance for item logs and for
    -- finding duplicated or stolen items. It is the primary key, so one
    -- serial can only be in one place.
    serial uuid PRIMARY KEY DEFAULT uuidv7(),
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    -- MAX_INVENTORY is 200.
    slot smallint NOT NULL CHECK (slot BETWEEN 0 AND 199),
    item_id integer NOT NULL CHECK (item_id > 0),
    -- MAX_AMOUNT is 30000.
    amount integer NOT NULL CHECK (amount BETWEEN 1 AND 30000),
    UNIQUE (character_id, slot)
);

-- The game servers' role, when it exists, gets only the rights it needs:
-- no DDL and no deletes of accounts or characters. See
-- deploy/postgres/roles.sql.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT ON accounts TO wathena_game;
        GRANT SELECT, INSERT, UPDATE ON characters TO wathena_game;
        GRANT SELECT, INSERT, UPDATE, DELETE ON inventory TO wathena_game;
        -- waserver reads the applied migrations to refuse to start with
        -- pending ones.
        GRANT SELECT ON goose_db_version TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE inventory;
DROP TABLE characters;
DROP TABLE accounts;
