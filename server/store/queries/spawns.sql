-- Queries for the spawns of a procedural world (decision 0042). `wactl
-- world new` runs InsertWorldSpawn as the migrator role, in the world's
-- transaction. waserver reads the rows as wathena_game.

-- name: InsertWorldSpawn :exec
INSERT INTO world_spawns (map_id, ord, monster, count, respawn_ms, role,
    area_x, area_y, area_w, area_h, avoid_x, avoid_y, avoid_r)
VALUES (@map_id, @ord, @monster, @count, @respawn_ms, @role,
    @area_x, @area_y, @area_w, @area_h, @avoid_x::integer[], @avoid_y::integer[], @avoid_r::integer[]);

-- name: WorldSpawns :many
SELECT map_id, monster, count, respawn_ms, role, area_x, area_y, area_w, area_h, avoid_x, avoid_y, avoid_r
FROM world_spawns ORDER BY map_id, ord;
