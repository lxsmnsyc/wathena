-- Job changes and saved options (phase 4 step 4).
--
-- Rules:
--   - job_change_level is rAthena's change_level_2nd, which it keeps in
--     the JOBCHANGE2ND_VAR variable: the job level at the change from a
--     first class to a second class. 0 means not known: the skill tree
--     then counts the first class's maximum job level, as rAthena does.
--   - rebirth_job is the second class a character had before its rebirth,
--     which picks its transcendent line, as the ADVJOB variable of
--     rAthena's rebirth NPC. 0 means none.
--   - options are the bits of rAthena's status.option that it saves: a
--     cart (OPTION_CART1 to OPTION_CART5), a falcon (OPTION_FALCON) and a
--     Peco Peco (OPTION_RIDING).

-- +goose Up

ALTER TABLE characters
    ADD COLUMN job_change_level integer NOT NULL DEFAULT 0 CHECK (job_change_level BETWEEN 0 AND 275),
    ADD COLUMN rebirth_job integer NOT NULL DEFAULT 0 CHECK (rebirth_job >= 0),
    ADD COLUMN options integer NOT NULL DEFAULT 0 CHECK (options >= 0);

-- +goose Down

ALTER TABLE characters
    DROP COLUMN options,
    DROP COLUMN rebirth_job,
    DROP COLUMN job_change_level;
