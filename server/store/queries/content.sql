-- Queries for the procedural content of a procedural world (decision
-- 0044). `wactl world new` runs the inserts as the migrator role, in the
-- world's transaction. waserver reads the rows as wathena_game.

-- name: InsertWorldPlace :exec
INSERT INTO world_places (map_id, ord, x, y, tags, source)
VALUES (@map_id, @ord, @x, @y, @tags::text[], @source);

-- name: InsertWorldObject :exec
INSERT INTO world_objects (map_id, ord, model, x, y, turn, source)
VALUES (@map_id, @ord, @model, @x, @y, @turn, @source);

-- name: InsertWorldQuest :exec
INSERT INTO world_quests (id, town, purpose, guild, stage, step, template, kind, title,
    monster, item, count, drop_rate, to_town, base_exp, job_exp, zeny)
VALUES (@id, @town, @purpose, @guild, @stage, @step, @template, @kind, @title,
    @monster, @item, @count, @drop_rate, sqlc.narg(to_town), @base_exp, @job_exp, @zeny);

-- name: WorldPlaces :many
SELECT map_id, x, y, tags, source FROM world_places ORDER BY map_id, ord;

-- name: WorldObjects :many
SELECT map_id, model, x, y, turn, source FROM world_objects ORDER BY map_id, ord;

-- name: WorldQuests :many
SELECT id, town, purpose, guild, stage, step, template, kind, title, monster, item,
    count, drop_rate, to_town, base_exp, job_exp, zeny
FROM world_quests ORDER BY id;
