-- Queries for the saved variables of mods (proposal 0002, "Variables").
-- waserver runs them as wathena_game.

-- name: CharacterVariables :many
SELECT mod_id, name, value FROM mod_character_variables
WHERE character_id = @character_id;

-- name: AccountVariables :many
SELECT mod_id, name, value FROM mod_account_variables
WHERE account_id = @account_id;

-- name: UpsertCharacterVariables :exec
INSERT INTO mod_character_variables (character_id, mod_id, name, value)
SELECT @character_id, unnest(@mod_ids::text[]), unnest(@names::text[]), unnest(@vals::jsonb[])
ON CONFLICT (character_id, mod_id, name) DO UPDATE SET value = excluded.value;

-- name: DeleteCharacterVariables :exec
DELETE FROM mod_character_variables
WHERE character_id = @character_id
    AND (mod_id, name) IN (SELECT unnest(@mod_ids::text[]), unnest(@names::text[]));

-- name: UpsertAccountVariables :exec
INSERT INTO mod_account_variables (account_id, mod_id, name, value)
SELECT @account_id, unnest(@mod_ids::text[]), unnest(@names::text[]), unnest(@vals::jsonb[])
ON CONFLICT (account_id, mod_id, name) DO UPDATE SET value = excluded.value;

-- name: DeleteAccountVariables :exec
DELETE FROM mod_account_variables
WHERE account_id = @account_id
    AND (mod_id, name) IN (SELECT unnest(@mod_ids::text[]), unnest(@names::text[]));

-- name: ServerVariables :many
SELECT mod_id, name, value FROM mod_server_variables;

-- name: SetServerVariable :exec
INSERT INTO mod_server_variables (mod_id, name, value)
VALUES (@mod_id, @name, @value)
ON CONFLICT (mod_id, name) DO UPDATE SET value = excluded.value, updated_at = now();

-- name: DeleteServerVariable :exec
DELETE FROM mod_server_variables WHERE mod_id = @mod_id AND name = @name;
