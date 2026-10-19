-- Queries for the world row and its first account. `wactl world new`
-- runs CreateWorld, CreateAdminAccount and SetWorldOwner as the migrator
-- role. waserver runs World as wathena_game.

-- name: CreateWorld :execrows
-- CreateWorld makes the world row. It makes nothing when the world
-- exists.
INSERT INTO world (name, ruleset, seed, mode, generator_version, rng_version, public_node)
VALUES (@name, @ruleset, @seed, @mode, @generator_version, @rng_version, @public_node)
ON CONFLICT (id) DO NOTHING;

-- name: World :one
-- World returns the world without its seed, with its public branch.
SELECT name, ruleset, mode, generator_version, rng_version, public_node, owner_account_id, created_at FROM world WHERE id = 1;

-- name: CreateAdminAccount :one
-- CreateAdminAccount makes the first account and its password in one
-- statement. It returns no row when the name is taken.
WITH a AS (
    INSERT INTO accounts (name) VALUES (@name)
    ON CONFLICT DO NOTHING
    RETURNING id
)
INSERT INTO account_passwords (account_id, hash)
SELECT id, @hash FROM a
RETURNING account_id;

-- name: SetWorldOwner :exec
-- SetWorldOwner records the first admin account as the world's owner
-- (review decision 54).
UPDATE world SET owner_account_id = @account_id WHERE id = 1;
