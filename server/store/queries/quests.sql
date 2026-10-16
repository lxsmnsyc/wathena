-- Queries for the quest log (decision 0043). waserver reads and writes a
-- character's quests with its save, as wathena_game.

-- name: CharacterQuests :many
SELECT quest_id, state, ends_at, count1, count2, count3
FROM character_quests WHERE character_id = @character_id ORDER BY ord, quest_id;

-- name: UpsertCharacterQuests :exec
-- UpsertCharacterQuests writes quests of a character's log that are new
-- or changed, in one statement.
INSERT INTO character_quests (character_id, quest_id, ord, state, ends_at, count1, count2, count3)
SELECT @character_id, unnest(@quest_ids::bigint[]), unnest(@ords::integer[]), unnest(@states::smallint[]),
    unnest(@ends::timestamptz[]), unnest(@count1s::integer[]), unnest(@count2s::integer[]), unnest(@count3s::integer[])
ON CONFLICT (character_id, quest_id) DO UPDATE
SET ord = excluded.ord, state = excluded.state, ends_at = excluded.ends_at,
    count1 = excluded.count1, count2 = excluded.count2, count3 = excluded.count3;

-- name: DeleteCharacterQuests :exec
DELETE FROM character_quests WHERE character_id = @character_id AND quest_id = ANY(@quest_ids::bigint[]);
