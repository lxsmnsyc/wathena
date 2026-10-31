-- Castles (proposal 0004, section 2).
--
-- Rules:
--   - castles holds one row per castle record, keyed by the record's
--     full name, as world events key their rows. A row is written
--     whenever a field changes, as rAthena's guild_castledatasave. A
--     record without a row starts at zeros.
--   - owner_id NULL reads as 0, no owner. Deleting a guild clears it,
--     as int_guild.cpp clears the owner of a broken guild's castles.
--   - The fields are rAthena's CD_ indexes 2 to 9; guardians holds the
--     installed bits of guardians 0 to 7 (CD_ENABLED_GUARDIAN00 to 07).
--     Values are integer, since rAthena's columns are int(11) unsigned
--     and scripts never pass 2^31.
--   - The game role cannot delete rows: a castle record a mod removed
--     keeps its row, so putting the mod back restores the castle.
--   - mod_ids takes the castle kind of data/castles.yml.

-- +goose Up

ALTER TABLE mod_ids DROP CONSTRAINT mod_ids_kind_check;
ALTER TABLE mod_ids ADD CONSTRAINT mod_ids_kind_check
    CHECK (kind IN ('item', 'monster', 'skill', 'job', 'companion', 'mount', 'storage', 'currency', 'castle', 'quest', 'achievement'));

CREATE TABLE castles (
    castle text PRIMARY KEY CHECK (char_length(castle) BETWEEN 1 AND 160),
    owner_id bigint REFERENCES guilds (id) ON DELETE SET NULL,
    economy integer NOT NULL DEFAULT 0 CHECK (economy >= 0),
    defense integer NOT NULL DEFAULT 0 CHECK (defense >= 0),
    invested_economy integer NOT NULL DEFAULT 0 CHECK (invested_economy >= 0),
    invested_defense integer NOT NULL DEFAULT 0 CHECK (invested_defense >= 0),
    next_time integer NOT NULL DEFAULT 0 CHECK (next_time >= 0),
    pay_time integer NOT NULL DEFAULT 0 CHECK (pay_time >= 0),
    create_time integer NOT NULL DEFAULT 0 CHECK (create_time >= 0),
    kafra boolean NOT NULL DEFAULT false,
    guardians smallint NOT NULL DEFAULT 0 CHECK (guardians BETWEEN 0 AND 255),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX castles_owner ON castles (owner_id);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE ON castles TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE castles;

-- The old check does not take castles, so their IDs go.
DELETE FROM mod_ids WHERE kind = 'castle';
ALTER TABLE mod_ids DROP CONSTRAINT mod_ids_kind_check;
ALTER TABLE mod_ids ADD CONSTRAINT mod_ids_kind_check
    CHECK (kind IN ('item', 'monster', 'skill', 'job', 'companion', 'mount', 'storage', 'currency', 'quest', 'achievement'));
