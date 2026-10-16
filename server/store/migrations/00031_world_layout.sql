-- The world layout (phase 7 step 4, decision 0041, proposal 0002 review
-- decision 43, strategy section 8.10).
--
-- Rules:
--   - world_maps holds every map of a procedural world's layout:
--     fields, towns, dungeon floors and the home map. `wactl world new`
--     makes the rows with the world, so a new generator version or a mod
--     change never renames a town or moves a warp.
--   - id is the map ID, such as gen:town:t2. ord is the layout's order.
--     display_name is the name players see, unique in any case. A town's
--     name comes from its naming style. Other maps take theirs from the
--     nearest town, and change with it.
--   - min_level and max_level are the map's level band, and distance its
--     number of warps from the home area.
--   - x and y are a field's coordinates, or the field near which a town,
--     the home map or a dungeon stands. size is a field's side in cells.
--     biome is a field's or a town's biome index, -1 for other maps.
--   - record is a town's template, a floor's dungeon style, or the home
--     map's mod:folder. params are a town's or a floor's params
--     (docs/specs/generator-v3.md), kept so a changed record never
--     changes a made map.
--   - world_links holds the links: a warp at each end, at the a_index-th
--     place with the tag a_tag on map a_map, leads to the other end.
--   - character_maps holds the maps each character entered, for the
--     teleport service and the world map, in the order it entered
--     them (seq). character_memos holds the
--     Warp Portal memo points by map ID, 3 at most.
--   - The game role reads the layout and changes only display names,
--     with @renametown. The migrator role makes the rows.

-- +goose Up

CREATE TABLE world_maps (
    id text PRIMARY KEY CHECK (id LIKE 'gen:%' AND char_length(id) <= 64),
    ord integer NOT NULL UNIQUE CHECK (ord >= 0),
    kind text NOT NULL CHECK (kind IN ('field', 'town', 'dungeon', 'home')),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 64),
    min_level integer NOT NULL CHECK (min_level >= 1),
    max_level integer NOT NULL CHECK (max_level >= min_level),
    distance integer NOT NULL CHECK (distance >= 0),
    x integer NOT NULL DEFAULT 0,
    y integer NOT NULL DEFAULT 0,
    size integer NOT NULL DEFAULT 0 CHECK (size BETWEEN 0 AND 1024),
    biome integer NOT NULL DEFAULT -1 CHECK (biome >= -1),
    record text NOT NULL DEFAULT '' CHECK (char_length(record) <= 160),
    params bytea NOT NULL DEFAULT '' CHECK (octet_length(params) <= 16384),
    town integer NOT NULL DEFAULT 0 CHECK (town >= 0),
    dungeon text NOT NULL DEFAULT '' CHECK (char_length(dungeon) <= 32),
    floor integer NOT NULL DEFAULT 0 CHECK (floor >= 0),
    floors integer NOT NULL DEFAULT 0 CHECK (floors >= 0),
    nomemo boolean NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX world_maps_display_name ON world_maps (lower(display_name));

CREATE TABLE world_links (
    ord integer PRIMARY KEY CHECK (ord >= 0),
    a_map text NOT NULL REFERENCES world_maps (id),
    a_tag text NOT NULL CHECK (char_length(a_tag) BETWEEN 1 AND 64),
    a_index integer NOT NULL CHECK (a_index >= 0),
    b_map text NOT NULL REFERENCES world_maps (id),
    b_tag text NOT NULL CHECK (char_length(b_tag) BETWEEN 1 AND 64),
    b_index integer NOT NULL CHECK (b_index >= 0)
);

CREATE TABLE character_maps (
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    map_id text NOT NULL CHECK (char_length(map_id) BETWEEN 1 AND 64),
    seq bigint GENERATED ALWAYS AS IDENTITY,
    visited_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (character_id, map_id)
);

CREATE TABLE character_memos (
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    slot smallint NOT NULL CHECK (slot BETWEEN 0 AND 2),
    map_id text NOT NULL CHECK (char_length(map_id) BETWEEN 1 AND 64),
    x integer NOT NULL CHECK (x >= 0),
    y integer NOT NULL CHECK (y >= 0),
    PRIMARY KEY (character_id, slot)
);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT ON world_maps, world_links TO wathena_game;
        GRANT UPDATE (display_name) ON world_maps TO wathena_game;
        GRANT SELECT, INSERT, UPDATE, DELETE ON character_maps, character_memos TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE character_memos;
DROP TABLE character_maps;
DROP TABLE world_links;
DROP TABLE world_maps;
