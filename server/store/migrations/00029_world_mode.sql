-- The world mode and the rng spec version (phase 7 step 1, decision
-- 0038, strategy sections 8.1 and 8.4).
--
-- Rules:
--   - mode is classic or procedural, chosen when the world is made.
--     Worlds made before it are classic.
--   - generator_version is the map generator version, 0 in a classic
--     world. rng_version is the seed tree spec, such as rng-v1, empty in
--     a classic world. A procedural world records both.
--   - The game role reads both columns. Only the migrator role makes
--     the world.

-- +goose Up

ALTER TABLE world
    ADD COLUMN mode text NOT NULL DEFAULT 'classic' CHECK (mode IN ('classic', 'procedural')),
    ADD COLUMN rng_version text NOT NULL DEFAULT '' CHECK (rng_version = '' OR rng_version ~ '^rng-v[1-9][0-9]*$'),
    ADD CONSTRAINT world_mode_versions CHECK (
        (mode = 'classic' AND generator_version = 0 AND rng_version = '')
        OR (mode = 'procedural' AND generator_version > 0 AND rng_version <> '')
    );

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT (mode, rng_version) ON world TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

ALTER TABLE world
    DROP CONSTRAINT world_mode_versions,
    DROP COLUMN rng_version,
    DROP COLUMN mode;
