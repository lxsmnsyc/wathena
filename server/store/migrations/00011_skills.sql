-- Learned skills (phase 4 step 3).
--
-- Rules:
--   - A row is one skill a character learned with skill points, as
--     rAthena's skill table with SKILL_FLAG_PERMANENT. Skills that items
--     grant are not saved.
--   - skill_id is core's ID from skill_db, or a mod skill's ID from the
--     mod range, which mod_ids keeps for good. A row whose skill no longer
--     exists is left out when the character loads.
--   - A save replaces a character's rows when they changed.

-- +goose Up

CREATE TABLE character_skills (
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    skill_id integer NOT NULL CHECK (skill_id > 0),
    level smallint NOT NULL CHECK (level BETWEEN 1 AND 1000),
    PRIMARY KEY (character_id, skill_id)
);

-- Rights, when the roles exist (deploy/postgres/roles.sql): waserver
-- reads and writes the rows. The account service has no rights on them.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON character_skills TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE character_skills;
