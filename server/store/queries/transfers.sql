-- Queries for transfer commits and their logs (decision 0017).

-- name: LockCharacters :many
-- LockCharacters locks the rows of the characters a commit saves, in ID
-- order, so two commits never wait on each other in a cycle.
SELECT id FROM characters
WHERE id = ANY(@ids::bigint[])
ORDER BY id
FOR UPDATE;

-- name: InsertItemLogs :exec
-- InsertItemLogs writes item log lines in one statement. source is NULL
-- for a line without one. Cards come as array literals and rolls as
-- JSON text, as in InsertInventory.
INSERT INTO item_log (at, action, from_kind, from_id, to_kind, to_id, serial, source,
    item_id, amount, refine, cards, rolls, card_rolls, random_options, identified, from_storage, to_storage, bound)
SELECT unnest(@ats::timestamptz[]), unnest(@actions::text[]), unnest(@from_kinds::text[]),
    unnest(@from_ids::bigint[]), unnest(@to_kinds::text[]), unnest(@to_ids::bigint[]),
    unnest(@serials::uuid[]), unnest(@sources::uuid[]), unnest(@item_ids::integer[]),
    unnest(@amounts::integer[]), unnest(@refines::smallint[]), unnest(@cards::text[])::integer[],
    unnest(@rolls::text[])::jsonb, unnest(@card_rolls::text[])::jsonb,
    unnest(@random_options::text[])::jsonb, unnest(@identified::boolean[]),
    unnest(@from_storages::integer[]), unnest(@to_storages::integer[]), unnest(@bounds::smallint[]);

-- name: InsertZenyLogs :exec
-- InsertZenyLogs writes zeny and currency log lines. currency is 0 for
-- zeny.
INSERT INTO zeny_log (at, action, from_kind, from_id, to_kind, to_id, amount, currency)
SELECT unnest(@ats::timestamptz[]), unnest(@actions::text[]), unnest(@from_kinds::text[]),
    unnest(@from_ids::bigint[]), unnest(@to_kinds::text[]), unnest(@to_ids::bigint[]),
    unnest(@amounts::bigint[]), unnest(@currencies::integer[]);

-- name: EnsureLogPartitions :exec
-- EnsureLogPartitions makes the month partitions of the logs for the
-- month of day and the month after.
SELECT ensure_log_partitions(@day::timestamptz);
