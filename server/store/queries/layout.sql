-- Queries for the world layout (decision 0041). `wactl world new` runs
-- InsertWorldMap and InsertWorldLink as the migrator role, in the
-- world's transaction. waserver reads the layout, renames maps, and keeps
-- the maps each character entered and its memo points, as wathena_game.

-- name: InsertWorldMap :exec
INSERT INTO world_maps (id, ord, kind, display_name, min_level, max_level, distance,
    x, y, size, biome, record, params, town, dungeon, floor, floors, nomemo)
VALUES (@id, @ord, @kind, @display_name, @min_level, @max_level, @distance,
    @x, @y, @size, @biome, @record, @params, @town, @dungeon, @floor, @floors, @nomemo);

-- name: InsertWorldLink :exec
INSERT INTO world_links (ord, a_map, a_tag, a_index, b_map, b_tag, b_index)
VALUES (@ord, @a_map, @a_tag, @a_index, @b_map, @b_tag, @b_index);

-- name: WorldMaps :many
SELECT id, kind, display_name, min_level, max_level, distance, x, y, size, biome,
    record, params, town, dungeon, floor, floors, nomemo
FROM world_maps ORDER BY ord;

-- name: WorldLinks :many
SELECT a_map, a_tag, a_index, b_map, b_tag, b_index FROM world_links ORDER BY ord;

-- name: RenameWorldMap :execrows
UPDATE world_maps SET display_name = @display_name WHERE id = @id;

-- name: CharacterMaps :many
SELECT map_id FROM character_maps WHERE character_id = @character_id ORDER BY seq;

-- name: InsertCharacterMaps :exec
-- InsertCharacterMaps records maps a character entered, in one
-- statement. A map it entered before stays as it was.
INSERT INTO character_maps (character_id, map_id)
SELECT @character_id, m.map_id
FROM unnest(@map_ids::text[]) WITH ORDINALITY AS m (map_id, n)
ORDER BY m.n
ON CONFLICT DO NOTHING;

-- name: CharacterMemos :many
SELECT slot, map_id, x, y FROM character_memos WHERE character_id = @character_id ORDER BY slot;

-- name: DeleteCharacterMemos :exec
DELETE FROM character_memos WHERE character_id = @character_id;

-- name: InsertCharacterMemos :exec
INSERT INTO character_memos (character_id, slot, map_id, x, y)
SELECT @character_id, unnest(@slots::smallint[]), unnest(@map_ids::text[]),
    unnest(@xs::integer[]), unnest(@ys::integer[]);
