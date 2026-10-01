-- Queries for the world row and staff accounts. `wactl world new` runs
-- CreateWorld and CreateStaffAccount as the migrator role. waserver runs
-- World as wathena_game.

-- name: CreateWorld :execrows
-- CreateWorld makes the world row. It makes nothing when the world
-- exists.
INSERT INTO world (name, ruleset, seed, generator_version)
VALUES (@name, @ruleset, @seed, @generator_version)
ON CONFLICT (id) DO NOTHING;

-- name: World :one
-- World returns the world without its seed.
SELECT name, ruleset, generator_version, created_at FROM world WHERE id = 1;

-- name: CreateStaffAccount :one
-- CreateStaffAccount makes a staff account and its password in one
-- statement. It returns no row when the name is taken.
WITH a AS (
    INSERT INTO accounts (name, staff) VALUES (@name, true)
    ON CONFLICT DO NOTHING
    RETURNING id
)
INSERT INTO account_passwords (account_id, hash)
SELECT id, @hash FROM a
RETURNING account_id;
