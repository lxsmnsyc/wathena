-- Rotating dungeons and world changes (phase 7 step 7, decision 0045,
-- proposal 0002 review decision 47, strategy sections 8.8 and 8.10).
--
-- Rules:
--   - world_rotations holds a procedural world's rotating dungeon, which
--     `wactl world new` marks: the first dungeon whose style has a
--     rotation. every, at and weekday are the style's rotation, kept so a
--     changed record never changes a made world. key is the derived key
--     child(child(secret, "rotation"), dungeon), from which the game role
--     computes each rotation's spawns. It reveals the spawns of that
--     dungeon's floors and nothing else.
--   - active_date is the date of the floors that run, or NULL before the
--     server first starts the world. The game role sets it in the same
--     transaction that moves the points of the old floors, so a change
--     that a stop cut short happens again at the next start.
--   - world.season_key is child(secret, "season"), from which the game
--     role computes the spawns of changed regions. It reveals those
--     spawns and nothing else.
--   - world_seasons has one row once procedural.seasons.days turned
--     seasons on: the date they started, and how many season ends have
--     changed a region.
--   - world_regions holds each changed region's season, by its town's
--     ID. A region without a row is in season 0.
--   - The game role reads these, and changes only active_date, the
--     seasons and the regions.

-- +goose Up

CREATE TABLE world_rotations (
    dungeon text PRIMARY KEY CHECK (char_length(dungeon) BETWEEN 1 AND 32),
    style text NOT NULL CHECK (char_length(style) BETWEEN 1 AND 160),
    every text NOT NULL CHECK (every IN ('daily', 'weekly')),
    at text NOT NULL CHECK (at ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'),
    weekday text NOT NULL DEFAULT '' CHECK (
        (every = 'daily' AND weekday = '')
        OR (every = 'weekly' AND weekday IN ('sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'))
    ),
    key bytea NOT NULL CHECK (octet_length(key) = 32),
    active_date date,
    changed_at timestamptz
);

ALTER TABLE world
    ADD COLUMN season_key bytea CHECK (season_key IS NULL OR octet_length(season_key) = 32);

CREATE TABLE world_seasons (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    started date NOT NULL,
    ends integer NOT NULL DEFAULT 0 CHECK (ends >= 0)
);

CREATE TABLE world_regions (
    town text PRIMARY KEY CHECK (town ~ '^t[1-9][0-9]{0,5}$'),
    season integer NOT NULL CHECK (season >= 1),
    changed_at timestamptz NOT NULL DEFAULT now()
);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT ON world_rotations TO wathena_game;
        GRANT UPDATE (active_date, changed_at) ON world_rotations TO wathena_game;
        GRANT SELECT (season_key) ON world TO wathena_game;
        GRANT SELECT, INSERT, UPDATE ON world_seasons, world_regions TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE world_regions;
DROP TABLE world_seasons;
ALTER TABLE world DROP COLUMN season_key;
DROP TABLE world_rotations;
