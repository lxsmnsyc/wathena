-- name: LoadCastles :many
SELECT castle, COALESCE(owner_id, 0)::bigint AS owner_id, economy, defense, invested_economy, invested_defense,
    next_time, pay_time, create_time, kafra, guardians
FROM castles;

-- name: UpsertCastle :exec
-- UpsertCastle writes every field of a castle. An owner whose guild is
-- gone saves as none: the guild's delete may run first.
INSERT INTO castles (castle, owner_id, economy, defense, invested_economy, invested_defense,
    next_time, pay_time, create_time, kafra, guardians, updated_at)
VALUES (@castle, (SELECT id FROM guilds WHERE id = @owner_id::bigint), @economy, @defense, @invested_economy, @invested_defense,
    @next_time, @pay_time, @create_time, @kafra, @guardians, now())
ON CONFLICT (castle) DO UPDATE SET owner_id = EXCLUDED.owner_id, economy = EXCLUDED.economy, defense = EXCLUDED.defense,
    invested_economy = EXCLUDED.invested_economy, invested_defense = EXCLUDED.invested_defense,
    next_time = EXCLUDED.next_time, pay_time = EXCLUDED.pay_time, create_time = EXCLUDED.create_time,
    kafra = EXCLUDED.kafra, guardians = EXCLUDED.guardians, updated_at = EXCLUDED.updated_at;
