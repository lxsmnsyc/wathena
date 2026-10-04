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
    job_change_level = @job_change_level, rebirth_job = @rebirth_job,
    options = @options, companion = @companion, mount = @mount,
    mail_day = @mail_day, mail_sent = @mail_sent,
    save_version = save_version + 1, saved_at = now()
WHERE id = @id AND save_version = @save_version;

-- name: Inventory :many
SELECT slot, item_id, amount, serial, equip, refine, cards, rolls, card_rolls, random_options, identified, bound FROM inventory
WHERE character_id = @character_id
ORDER BY slot;

-- name: DeleteInventorySlots :exec
DELETE FROM inventory
WHERE character_id = @character_id AND slot = ANY(@slots::smallint[]);

-- name: InsertInventory :exec
-- InsertInventory writes items in one statement. Each item's cards come
-- as an array literal, such as {4001,4002}, since PostgreSQL arrays of
-- arrays must all have the same length. Rolls and random options come
-- as JSON text.
INSERT INTO inventory (character_id, slot, item_id, amount, serial, equip, refine, cards,
    rolls, card_rolls, random_options, identified, bound)
SELECT @character_id, unnest(@slots::smallint[]), unnest(@item_ids::integer[]),
    unnest(@amounts::integer[]), unnest(@serials::uuid[]), unnest(@equips::integer[]),
    unnest(@refines::smallint[]), unnest(@cards::text[])::integer[],
    unnest(@rolls::text[])::jsonb, unnest(@card_rolls::text[])::jsonb,
    unnest(@random_options::text[])::jsonb, unnest(@identified::boolean[]), unnest(@bounds::smallint[]);

-- name: Statuses :many
SELECT name, val1, val2, val3, val4, tick, end_ms FROM character_statuses
WHERE character_id = @character_id
ORDER BY name;

-- name: DeleteStatuses :exec
DELETE FROM character_statuses WHERE character_id = @character_id;

-- name: InsertStatuses :exec
INSERT INTO character_statuses (character_id, name, val1, val2, val3, val4, tick, end_ms)
SELECT @character_id, unnest(@names::text[]), unnest(@val1s::integer[]),
    unnest(@val2s::integer[]), unnest(@val3s::integer[]), unnest(@val4s::integer[]),
    unnest(@ticks::bigint[]), unnest(@ends::bigint[]);

-- name: TimedBonuses :many
SELECT key, remaining_ms, bonuses, ends_on, icon, kind FROM character_timed_bonuses
WHERE character_id = @character_id
ORDER BY position;

-- name: DeleteTimedBonuses :exec
DELETE FROM character_timed_bonuses WHERE character_id = @character_id;

-- name: InsertTimedBonus :exec
INSERT INTO character_timed_bonuses (character_id, position, key, remaining_ms, bonuses, ends_on, icon, kind)
VALUES (@character_id, @position, @key, @remaining_ms, @bonuses, @ends_on, @icon, @kind);

-- name: Skills :many
SELECT skill_id, level FROM character_skills
WHERE character_id = @character_id
ORDER BY skill_id;

-- name: DeleteSkills :exec
DELETE FROM character_skills WHERE character_id = @character_id;

-- name: InsertSkills :exec
INSERT INTO character_skills (character_id, skill_id, level)
SELECT @character_id, unnest(@skill_ids::integer[]), unnest(@levels::smallint[]);
