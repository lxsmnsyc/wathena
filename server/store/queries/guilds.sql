-- Queries for guilds, guild storage and emblems (decision 0021).

-- name: Guilds :many
SELECT id, name, level, exp, skill_points, skills, positions, notice_title, notice_text, expulsions, last_master_change
FROM guilds
ORDER BY id;

-- name: GuildMembers :many
-- GuildMembers are every guild's members, with what the guild window
-- shows of an offline member.
SELECT m.guild_id, m.slot, m.position, m.exp, c.id, c.account_id, c.name, c.base_level, c.job, c.map
FROM guild_members m
JOIN characters c ON c.id = m.character_id
ORDER BY m.guild_id, m.slot;

-- name: GuildRelations :many
SELECT r.guild_id, r.other_id, r.enemy, g.name
FROM guild_relations r
JOIN guilds g ON g.id = r.other_id
ORDER BY r.guild_id, r.other_id;

-- name: GuildEmblems :many
SELECT guild_id, version, png FROM guild_emblems;

-- name: GuildStorage :many
SELECT guild_id, slot, item_id, amount, serial, refine, cards, rolls, card_rolls, random_options, identified, bound, broken, signed_by, signed_name
FROM guild_storage
ORDER BY guild_id, slot;

-- name: InsertGuild :exec
INSERT INTO guilds (id, name, level, exp, skill_points, skills, positions, notice_title, notice_text, expulsions, last_master_change)
VALUES (@id, @name, @level, @exp, @skill_points, @skills, @positions, @notice_title, @notice_text, @expulsions,
    sqlc.narg('last_master_change'));

-- name: UpdateGuild :exec
UPDATE guilds
SET name = @name, skills = @skills, positions = @positions, notice_title = @notice_title, notice_text = @notice_text, expulsions = @expulsions,
    last_master_change = sqlc.narg('last_master_change')
WHERE id = @id;

-- name: UpdateGuildExp :exec
UPDATE guilds SET level = @level, exp = @exp, skill_points = @skill_points WHERE id = @id;

-- name: UpdateGuildMemberExp :exec
UPDATE guild_members SET exp = @exp WHERE guild_id = @guild_id AND character_id = @character_id;

-- name: DeleteGuild :exec
DELETE FROM guilds WHERE id = @id;

-- name: DeleteGuildRelations :exec
-- DeleteGuildRelations removes a guild's own side of its relations,
-- before they are written again.
DELETE FROM guild_relations WHERE guild_id = @guild_id;

-- name: InsertGuildRelations :exec
-- InsertGuildRelations writes a guild's side. A relation to a guild
-- that is gone is left out.
INSERT INTO guild_relations (guild_id, other_id, enemy)
SELECT @guild_id::bigint, t.o, t.e
FROM (SELECT unnest(@other_ids::bigint[]) AS o, unnest(@enemies::boolean[]) AS e) t
WHERE EXISTS (SELECT FROM guilds g WHERE g.id = t.o)
ON CONFLICT DO NOTHING;

-- name: LeaveGuilds :exec
-- LeaveGuilds takes a character out of any guild, before it joins one.
DELETE FROM guild_members WHERE character_id = @character_id;

-- name: UpsertGuildMember :exec
INSERT INTO guild_members (guild_id, character_id, slot, position, exp)
VALUES (@guild_id, @character_id, @slot, @position, @exp)
ON CONFLICT (character_id) DO UPDATE SET guild_id = EXCLUDED.guild_id, slot = EXCLUDED.slot,
    position = EXCLUDED.position, exp = EXCLUDED.exp;

-- name: ClearGuildSlot :exec
-- ClearGuildSlot frees a slot for another character, as when the master
-- and a member swap slots.
DELETE FROM guild_members WHERE guild_id = @guild_id AND slot = @slot AND character_id <> @character_id;

-- name: DeleteGuildMember :exec
DELETE FROM guild_members WHERE guild_id = @guild_id AND character_id = @character_id;

-- name: UpsertGuildEmblem :exec
INSERT INTO guild_emblems (guild_id, version, png, uploaded_by, uploaded_at)
VALUES (@guild_id, @version, @png, @uploaded_by, now())
ON CONFLICT (guild_id) DO UPDATE SET version = EXCLUDED.version, png = EXCLUDED.png,
    uploaded_by = EXCLUDED.uploaded_by, uploaded_at = EXCLUDED.uploaded_at;

-- name: LockGuilds :many
-- LockGuilds locks the rows of guilds whose storage a commit writes, in
-- ID order.
SELECT id FROM guilds WHERE id = ANY(@ids::bigint[]) ORDER BY id FOR UPDATE;

-- name: DeleteGuildStorageSlots :exec
DELETE FROM guild_storage
WHERE guild_id = @guild_id AND slot = ANY(@slots::smallint[]);

-- name: InsertGuildStorage :exec
-- InsertGuildStorage writes items as InsertInventory does.
INSERT INTO guild_storage (guild_id, slot, item_id, amount, serial, refine, cards,
    rolls, card_rolls, random_options, identified, bound, broken, signed_by, signed_name)
SELECT @guild_id, unnest(@slots::smallint[]), unnest(@item_ids::integer[]),
    unnest(@amounts::integer[]), unnest(@serials::uuid[]),
    unnest(@refines::smallint[]), unnest(@cards::text[])::integer[],
    unnest(@rolls::text[])::jsonb, unnest(@card_rolls::text[])::jsonb,
    unnest(@random_options::text[])::jsonb, unnest(@identified::boolean[]), unnest(@bounds::smallint[]), unnest(@brokens::boolean[]), unnest(@signed_bys::integer[]), unnest(@signed_names::text[]);

-- name: GuildStorageLog :many
-- GuildStorageLog is the last moves of each guild's storage in the item
-- log, newest first, as storage_guild_log_read: rows between an
-- inventory and a guild storage in the last 90 days, at most max_rows
-- per guild.
SELECT t.guild_id::bigint AS guild_id, t.at, t.amount::integer AS amount, t.item_id, t.refine, t.cards, t.name FROM (
    SELECT CASE WHEN l.from_kind = 'guild_storage' THEN l.from_id ELSE l.to_id END AS guild_id,
        l.at, CASE WHEN l.from_kind = 'guild_storage' THEN -l.amount ELSE l.amount END AS amount,
        l.item_id, l.refine, l.cards, COALESCE(c.name, '')::text AS name,
        row_number() OVER (PARTITION BY CASE WHEN l.from_kind = 'guild_storage' THEN l.from_id ELSE l.to_id END
            ORDER BY l.at DESC, l.id DESC) AS n
    FROM item_log l
    LEFT JOIN characters c ON c.id = CASE WHEN l.from_kind = 'inventory' THEN l.from_id ELSE l.to_id END
    WHERE l.at > now() - interval '90 days'
        AND ((l.from_kind = 'guild_storage' AND l.to_kind = 'inventory')
            OR (l.from_kind = 'inventory' AND l.to_kind = 'guild_storage'))
) t
WHERE t.n <= @max_rows::bigint
ORDER BY t.guild_id, t.at DESC;

-- name: GuildIDFloor :one
-- GuildIDFloor is the highest guild ID that a guild that broke up left
-- in the item log or the reports, so a new guild never takes it and
-- shows its storage log.
SELECT GREATEST(
    COALESCE((SELECT max(from_id) FROM item_log WHERE from_kind = 'guild_storage'), 0),
    COALESCE((SELECT max(to_id) FROM item_log WHERE to_kind = 'guild_storage'), 0),
    COALESCE((SELECT max(guild_id) FROM reports), 0)
)::bigint AS floor;
