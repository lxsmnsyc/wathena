-- The spawns of a procedural world (phase 7 step 5, decision 0042,
-- proposal 0002 review decision 44, strategy section 8.11).
--
-- Rules:
--   - world_spawns holds each field's and dungeon floor's spawn list.
--     `wactl world new` picks the lists from the secret branch of the
--     world's seed, which it alone holds, runs the mods' spawn_table
--     hooks on them, and writes the rows with the world, so a new
--     generator version or a mod change never changes them.
--   - map_id is a map of world_maps, and ord the line's place in its
--     map's list. monster is the monster's name: an AegisName, such as
--     PORING, or a mod monster's full name. waserver finds its stats by
--     that name at each start, and leaves out a monster that no longer
--     loads.
--   - count is how many monsters the line keeps alive, respawn_ms how
--     long a dead one takes to come back, and role its part in the list:
--     common, uncommon, plant, rare, mini_boss or field_boss.
--   - area_x, area_y, area_w and area_h are where the monsters appear:
--     within area_w and area_h cells of the center, or anywhere when all
--     are 0.
--   - avoid_x, avoid_y and avoid_r are cells the line keeps clear: no
--     monster of it appears within avoid_r cells of each. They keep
--     aggressive monsters 15 cells from arrivals and bosses 30 cells from
--     warps to towns.
--   - The game role reads the rows and never writes them. It never
--     reads the seed, so it cannot pick or predict spawns itself.

-- +goose Up

CREATE TABLE world_spawns (
    map_id text NOT NULL REFERENCES world_maps (id),
    ord integer NOT NULL CHECK (ord >= 0),
    monster text NOT NULL CHECK (char_length(monster) BETWEEN 1 AND 160),
    count integer NOT NULL CHECK (count BETWEEN 1 AND 1000),
    respawn_ms integer NOT NULL CHECK (respawn_ms BETWEEN 1000 AND 268435455),
    role text NOT NULL CHECK (role IN ('common', 'uncommon', 'plant', 'rare', 'mini_boss', 'field_boss')),
    area_x integer NOT NULL DEFAULT 0 CHECK (area_x BETWEEN 0 AND 1024),
    area_y integer NOT NULL DEFAULT 0 CHECK (area_y BETWEEN 0 AND 1024),
    area_w integer NOT NULL DEFAULT 0 CHECK (area_w BETWEEN 0 AND 1024),
    area_h integer NOT NULL DEFAULT 0 CHECK (area_h BETWEEN 0 AND 1024),
    avoid_x integer[] NOT NULL DEFAULT '{}',
    avoid_y integer[] NOT NULL DEFAULT '{}',
    avoid_r integer[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (map_id, ord),
    CHECK (cardinality(avoid_x) = cardinality(avoid_y) AND cardinality(avoid_x) = cardinality(avoid_r)),
    CHECK (cardinality(avoid_x) <= 256)
);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT ON world_spawns TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE world_spawns;
