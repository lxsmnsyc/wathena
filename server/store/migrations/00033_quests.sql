-- Quests (phase 7 step 6a, decision 0043, proposal 0002 review decision
-- 45), as rAthena's quest table.
--
-- Rules:
--   - character_quests holds each character's quest log: one row per
--     quest it has, in any state. ord is the quest's place in the log.
--   - quest_id is the record's ID of data/quests.yml. A row whose record
--     is gone, such as a removed mod's quest, stays.
--   - state is 0 inactive, 1 active or 2 complete, as rAthena's
--     e_quest_state.
--   - ends_at is when the quest's time limit ends, or null without one.
--     rAthena keeps the time in seconds; wAthena keeps the instant.
--   - count1 to count3 are the kills counted for each hunt objective, as
--     rAthena's columns.
--   - The game role reads and writes the rows with the character's save.
--     Deleting a character, or its account, deletes them.

-- +goose Up

CREATE TABLE character_quests (
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    quest_id bigint NOT NULL CHECK (quest_id BETWEEN 1 AND 4294967295),
    ord integer NOT NULL CHECK (ord >= 0),
    state smallint NOT NULL CHECK (state BETWEEN 0 AND 2),
    ends_at timestamptz,
    count1 integer NOT NULL DEFAULT 0 CHECK (count1 BETWEEN 0 AND 65535),
    count2 integer NOT NULL DEFAULT 0 CHECK (count2 BETWEEN 0 AND 65535),
    count3 integer NOT NULL DEFAULT 0 CHECK (count3 BETWEEN 0 AND 65535),
    PRIMARY KEY (character_id, quest_id)
);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON character_quests TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE character_quests;
