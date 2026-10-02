-- Status effects and timed bonuses (phase 4 step 2).
--
-- Rules:
--   - A saved status is one rAthena's chrif_save_scdata saves: every
--     active status without the NoSave flag. tick is the time to its
--     next timer in milliseconds, or -1 for a status without an end. For
--     a mod status that ticks, end_ms is the time to its end.
--   - A status is stored by its name in wa.statuses, such as blessing,
--     or a mod status's full name, so mod status IDs may change between
--     runs.
--   - A timed bonus is one of player:add_timed_bonuses (rAthena's
--     bonus_script). bonuses holds its entries as JSON. They are kept in
--     the order they started.
--   - A save replaces a character's rows when they changed.

-- +goose Up

CREATE TABLE character_statuses (
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 160),
    val1 integer NOT NULL,
    val2 integer NOT NULL,
    val3 integer NOT NULL,
    val4 integer NOT NULL,
    tick bigint NOT NULL CHECK (tick >= -1),
    end_ms bigint NOT NULL DEFAULT 0 CHECK (end_ms >= 0),
    PRIMARY KEY (character_id, name)
);

CREATE TABLE character_timed_bonuses (
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 1023),
    key text NOT NULL CHECK (char_length(key) BETWEEN 1 AND 64),
    remaining_ms bigint NOT NULL CHECK (remaining_ms >= 0),
    bonuses jsonb NOT NULL CHECK (octet_length(bonuses::text) <= 65536),
    ends_on text[] NOT NULL DEFAULT '{}' CHECK (
        cardinality(ends_on) <= 16
        AND (cardinality(ends_on) = 0 OR array_ndims(ends_on) = 1)
        AND array_position(ends_on, NULL) IS NULL
    ),
    stack boolean NOT NULL DEFAULT false,
    icon integer NOT NULL DEFAULT -1 CHECK (icon >= -1),
    kind text NOT NULL DEFAULT '' CHECK (kind IN ('', 'buff', 'debuff')),
    PRIMARY KEY (character_id, position)
);

-- Rights, when the roles exist (deploy/postgres/roles.sql): waserver
-- reads and writes the rows. The account service has no rights on them.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON character_statuses TO wathena_game;
        GRANT SELECT, INSERT, UPDATE, DELETE ON character_timed_bonuses TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE character_timed_bonuses;
DROP TABLE character_statuses;
