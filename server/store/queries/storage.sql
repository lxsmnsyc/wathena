-- Queries for storages and carts (decision 0023).

-- name: AccountStorage :many
-- AccountStorage is the stacks of an account's storage or of one of its
-- extra storages.
SELECT slot, item_id, amount, serial, refine, cards, rolls, card_rolls, random_options, identified, bound
FROM storage_items
WHERE holder = 'account' AND account_id = @account_id AND storage = @storage
ORDER BY slot;

-- name: CharacterStorage :many
-- CharacterStorage is the stacks of a character's cart or of one of its
-- extra storages. holder is cart or character.
SELECT slot, item_id, amount, serial, refine, cards, rolls, card_rolls, random_options, identified, bound
FROM storage_items
WHERE holder = @holder AND character_id = @character_id AND storage = @storage
ORDER BY slot;

-- name: DeleteAccountStorageSlots :exec
DELETE FROM storage_items
WHERE holder = 'account' AND account_id = @account_id AND storage = @storage AND slot = ANY(@slots::smallint[]);

-- name: DeleteCharacterStorageSlots :exec
DELETE FROM storage_items
WHERE holder = @holder AND character_id = @character_id AND storage = @storage AND slot = ANY(@slots::smallint[]);

-- name: InsertStorageItems :exec
-- InsertStorageItems writes items as InsertInventory does. account_id is
-- NULL for a character's storage and character_id for an account's.
INSERT INTO storage_items (holder, account_id, character_id, storage, slot, item_id, amount, serial, refine, cards,
    rolls, card_rolls, random_options, identified, bound)
SELECT @holder, sqlc.narg('account_id')::bigint, sqlc.narg('character_id')::bigint, @storage, unnest(@slots::smallint[]),
    unnest(@item_ids::integer[]), unnest(@amounts::integer[]), unnest(@serials::uuid[]),
    unnest(@refines::smallint[]), unnest(@cards::text[])::integer[],
    unnest(@rolls::text[])::jsonb, unnest(@card_rolls::text[])::jsonb,
    unnest(@random_options::text[])::jsonb, unnest(@identified::boolean[]), unnest(@bounds::smallint[]);

-- name: LockAccountStorages :exec
-- LockAccountStorages takes a transaction lock on the storages of each
-- account a commit saves, in ID order. The game role cannot lock account
-- rows, so the lock is advisory: bit 62 set, with the account ID.
SELECT pg_advisory_xact_lock((1::bigint << 62) | id)
FROM unnest(@ids::bigint[]) AS id
ORDER BY id;

-- name: AccountStorageSize :many
-- AccountStorageSize is the slots a script expanded an account's storage
-- to: no row for none.
SELECT slots FROM storage_sizes WHERE holder = 'account' AND account_id = @account_id;

-- name: CartSize :many
-- CartSize is the slots a script expanded a character's cart to: no row
-- for none.
SELECT slots FROM storage_sizes WHERE holder = 'cart' AND character_id = @character_id;

-- name: SetAccountStorageSize :exec
INSERT INTO storage_sizes (holder, account_id, slots) VALUES ('account', @account_id, @slots)
ON CONFLICT (account_id) WHERE holder = 'account' DO UPDATE SET slots = excluded.slots;

-- name: SetCartSize :exec
INSERT INTO storage_sizes (holder, character_id, slots) VALUES ('cart', @character_id, @slots)
ON CONFLICT (character_id) WHERE holder = 'cart' DO UPDATE SET slots = excluded.slots;

-- name: DeleteAccountStorageSize :exec
DELETE FROM storage_sizes WHERE holder = 'account' AND account_id = @account_id;

-- name: DeleteCartSize :exec
DELETE FROM storage_sizes WHERE holder = 'cart' AND character_id = @character_id;
