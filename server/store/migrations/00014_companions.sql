-- Companions and mounts (proposal 0002, review decision 25).
--
-- Rules:
--   - companion and mount are the full names of the character's records
--     of data/companions.yml and data/mounts.yml, such as falcon or
--     example.familiars:Owl. Empty means none.
--   - options keeps rAthena's option bits that show them, as migration
--     00012 saved them. A character saved before this migration has empty
--     names: the server finds its core companion and mount from the bits.
--   - A name whose record a removed mod took away is dropped when the
--     character loads, with a warning in the log.

-- +goose Up

ALTER TABLE characters
    ADD COLUMN companion text NOT NULL DEFAULT '' CHECK (octet_length(companion) <= 200),
    ADD COLUMN mount text NOT NULL DEFAULT '' CHECK (octet_length(mount) <= 200);

-- +goose Down

ALTER TABLE characters
    DROP COLUMN mount,
    DROP COLUMN companion;
