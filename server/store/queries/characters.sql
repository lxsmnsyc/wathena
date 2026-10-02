-- Queries for accounts, characters and inventories. sqlc turns each one
-- into a Go function with bound parameters in server/store/db.

-- name: AccountByName :one
SELECT id FROM accounts WHERE lower(name) = lower(@name);

-- name: CreateAccount :one
INSERT INTO accounts (name, dev) VALUES (@name, @dev)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: CharacterByName :one
SELECT id, account_id FROM characters WHERE lower(name) = lower(@name);

-- name: CreateCharacter :one
INSERT INTO characters (
    account_id, slot, name, job, sex, hair, hair_color, cloth_color,
    base_level, job_level, base_exp, job_exp, status_points, skill_points,
    str, agi, vit, int, dex, luk, hp, sp, map, x, y, zeny, inventory_size
) VALUES (
    @account_id, @slot, @name, @job, @sex, @hair, @hair_color, @cloth_color,
    @base_level, @job_level, @base_exp, @job_exp, @status_points, @skill_points,
    @str, @agi, @vit, @int, @dex, @luk, @hp, @sp, @map, @x, @y, @zeny, @inventory_size
)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: ClaimCharacter :one
-- ClaimCharacter starts a session: it raises the save version, so saves
-- from any earlier session fail.
UPDATE characters
SET save_version = save_version + 1, login_at = now()
WHERE id = @id
RETURNING *;

-- name: SaveCharacter :execrows
-- SaveCharacter writes a character if its save version is still the one
-- the session holds, and raises the version.
UPDATE characters SET
    job = @job, sex = @sex, hair = @hair, hair_color = @hair_color,
    cloth_color = @cloth_color, base_level = @base_level,
    job_level = @job_level, base_exp = @base_exp, job_exp = @job_exp,
    status_points = @status_points, skill_points = @skill_points,
    str = @str, agi = @agi, vit = @vit, int = @int, dex = @dex, luk = @luk,
    hp = @hp, sp = @sp, map = @map, x = @x, y = @y, zeny = @zeny,
    inventory_size = @inventory_size,
    save_map = @save_map, save_x = @save_x, save_y = @save_y,
    save_version = save_version + 1, saved_at = now()
WHERE id = @id AND save_version = @save_version;

-- name: Inventory :many
SELECT slot, item_id, amount, serial, equip, refine, cards FROM inventory
WHERE character_id = @character_id
ORDER BY slot;

-- name: DeleteInventorySlots :exec
DELETE FROM inventory
WHERE character_id = @character_id AND slot = ANY(@slots::smallint[]);

-- name: InsertInventory :exec
-- InsertInventory writes items in one statement. Each item's cards come
-- as an array literal, such as {4001,4002}, since PostgreSQL arrays of
-- arrays must all have the same length.
INSERT INTO inventory (character_id, slot, item_id, amount, serial, equip, refine, cards)
SELECT @character_id, unnest(@slots::smallint[]), unnest(@item_ids::integer[]),
    unnest(@amounts::integer[]), unnest(@serials::uuid[]), unnest(@equips::integer[]),
    unnest(@refines::smallint[]), unnest(@cards::text[])::integer[];
