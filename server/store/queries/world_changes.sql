-- Queries for rotating dungeons and world changes (decision 0045).
-- `wactl world new` runs InsertWorldRotation and SetSeasonKey as the
-- migrator role, in the world's transaction. waserver reads the state
-- and records each change, with the moves of the points of the maps it
-- removed, as wathena_game.

-- name: InsertWorldRotation :exec
INSERT INTO world_rotations (dungeon, style, every, at, weekday, key)
VALUES (@dungeon, @style, @every, @at, @weekday, @key);

-- name: SetSeasonKey :exec
UPDATE world SET season_key = @season_key WHERE id = 1;

-- name: WorldRotations :many
SELECT dungeon, style, every, at, weekday, key, active_date FROM world_rotations ORDER BY dungeon;

-- name: SeasonKey :one
SELECT season_key FROM world WHERE id = 1;

-- name: WorldSeasons :many
SELECT started, ends FROM world_seasons;

-- name: WorldRegions :many
SELECT town, season FROM world_regions ORDER BY town;

-- name: SetRotationDate :execrows
UPDATE world_rotations SET active_date = @active_date, changed_at = now() WHERE dungeon = @dungeon;

-- name: StartSeasons :exec
INSERT INTO world_seasons (started) VALUES (@started) ON CONFLICT (id) DO NOTHING;

-- name: SetSeasonEnds :execrows
UPDATE world_seasons SET ends = @ends WHERE id;

-- name: SetRegionSeason :exec
INSERT INTO world_regions (town, season) VALUES (@town, @season)
ON CONFLICT (town) DO UPDATE SET season = excluded.season, changed_at = now();

-- name: MoveCharacterMaps :execrows
-- MoveCharacterMaps moves the characters saved on a removed map to its
-- town's save spot.
UPDATE characters SET map = @to_map, x = @x, y = @y WHERE map = @from_map;

-- name: MoveSavePoints :execrows
UPDATE characters SET save_map = @to_map, save_x = @x, save_y = @y WHERE save_map = @from_map;

-- name: DropDoubleMemos :execrows
-- DropDoubleMemos deletes the memo points on a removed map of the
-- characters that have one on its town already: a character keeps one
-- memo point per map, as /memo.
DELETE FROM character_memos m
WHERE m.map_id = @from_map
    AND EXISTS (SELECT 1 FROM character_memos o WHERE o.character_id = m.character_id AND o.map_id = @to_map);

-- name: MoveMemos :execrows
UPDATE character_memos SET map_id = @to_map, x = @x, y = @y WHERE map_id = @from_map;
