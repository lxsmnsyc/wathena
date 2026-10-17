-- The procedural content of a procedural world (phase 7 step 6b,
-- decision 0044, proposal 0002 review decision 46, strategy sections
-- 8.6, 8.7 and 11.4).
--
-- Rules:
--   - `wactl world new` makes the rows with the world, from the stored
--     spawns, the quest templates and the secret branch of the seed,
--     after the mods' generation layers ran. A new generator version or
--     a mod change never changes them.
--   - world_places holds the named places that content and generation
--     layers added to generated maps: each town's quest_board, each job
--     guild's guild:<family>, the rebirth place, and the layers' places.
--     source is core or the mod's ID. ord is the place's order on its
--     map, after the map's generated places.
--   - world_objects holds the objects that generation layers put on
--     generated maps: a prop of the map's tileset by name, at a cell,
--     with 0 to 3 quarter turns. Clients draw them from the map's info.
--   - world_quests holds the quests a world made, with their quest IDs
--     from 900,000 to 999,999: each town's quest board quests (purpose
--     board), and each job guild's tests (purpose guild, with guild,
--     stage and step). kind is hunt, collect or delivery. monster and
--     item are names, as spawns store them, and count the kills or items
--     a quest asks for. drop_rate is a collect quest's drop out of
--     10,000. to_town is a delivery's town. base_exp, job_exp and zeny
--     are the reward. The server makes a quest record of each row at
--     start, and leaves out a row whose monster or item no longer
--     loads.
--   - The layers' bosses join world_spawns as mini_boss and field_boss
--     lines.
--   - The game role reads the rows and never writes them.

-- +goose Up

CREATE TABLE world_places (
    map_id text NOT NULL REFERENCES world_maps (id),
    ord integer NOT NULL CHECK (ord >= 0),
    x integer NOT NULL CHECK (x BETWEEN 0 AND 1024),
    y integer NOT NULL CHECK (y BETWEEN 0 AND 1024),
    tags text[] NOT NULL CHECK (cardinality(tags) BETWEEN 1 AND 8),
    source text NOT NULL CHECK (char_length(source) BETWEEN 1 AND 128),
    PRIMARY KEY (map_id, ord)
);

CREATE TABLE world_objects (
    map_id text NOT NULL REFERENCES world_maps (id),
    ord integer NOT NULL CHECK (ord >= 0),
    model text NOT NULL CHECK (char_length(model) BETWEEN 1 AND 64),
    x integer NOT NULL CHECK (x BETWEEN 0 AND 1024),
    y integer NOT NULL CHECK (y BETWEEN 0 AND 1024),
    turn smallint NOT NULL DEFAULT 0 CHECK (turn BETWEEN 0 AND 3),
    source text NOT NULL CHECK (char_length(source) BETWEEN 1 AND 128),
    PRIMARY KEY (map_id, ord)
);

CREATE TABLE world_quests (
    id integer PRIMARY KEY CHECK (id BETWEEN 900000 AND 999999),
    town text NOT NULL REFERENCES world_maps (id),
    purpose text NOT NULL CHECK (purpose IN ('board', 'guild')),
    guild text NOT NULL DEFAULT '' CHECK (char_length(guild) <= 32),
    stage text NOT NULL DEFAULT '' CHECK (stage IN ('', 'first', 'second')),
    step smallint NOT NULL DEFAULT 0 CHECK (step BETWEEN 0 AND 8),
    template text NOT NULL CHECK (char_length(template) BETWEEN 1 AND 160),
    kind text NOT NULL CHECK (kind IN ('hunt', 'collect', 'delivery')),
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 255),
    monster text NOT NULL DEFAULT '' CHECK (char_length(monster) <= 160),
    item text NOT NULL DEFAULT '' CHECK (char_length(item) <= 160),
    count integer NOT NULL DEFAULT 0 CHECK (count BETWEEN 0 AND 1000),
    drop_rate integer NOT NULL DEFAULT 0 CHECK (drop_rate BETWEEN 0 AND 10000),
    to_town text REFERENCES world_maps (id),
    base_exp bigint NOT NULL DEFAULT 0 CHECK (base_exp >= 0),
    job_exp bigint NOT NULL DEFAULT 0 CHECK (job_exp >= 0),
    zeny bigint NOT NULL DEFAULT 0 CHECK (zeny BETWEEN 0 AND 2147483647),
    CHECK ((purpose = 'guild') = (guild <> '' AND stage <> '' AND step > 0)),
    CHECK ((kind = 'delivery') = (to_town IS NOT NULL)),
    CHECK (kind = 'delivery' OR (monster <> '' AND count > 0)),
    CHECK (kind <> 'collect' OR item <> '')
);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT ON world_places, world_objects, world_quests TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE world_quests;
DROP TABLE world_objects;
DROP TABLE world_places;
