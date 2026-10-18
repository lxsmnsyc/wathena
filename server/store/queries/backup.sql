-- Queries of `wactl world backup` and `wactl world verify-backup`
-- (decision 0048). They run as the migrator role, or as the role that
-- restored a backup into a temporary database.

-- name: WorldCounts :one
-- WorldCounts counts what a backup's manifest records: accounts,
-- characters, item serials and the zeny total. Item serials are the
-- stacks of inventories, storages and carts, guild storages and mails.
-- The zeny total is the zeny of characters and of mails.
SELECT
    (SELECT count(*) FROM accounts)::bigint AS accounts,
    (SELECT count(*) FROM characters)::bigint AS characters,
    ((SELECT count(*) FROM inventory)
        + (SELECT count(*) FROM storage_items)
        + (SELECT count(*) FROM guild_storage)
        + (SELECT count(*) FROM mail_items))::bigint AS item_serials,
    ((SELECT coalesce(sum(zeny), 0) FROM characters)
        + (SELECT coalesce(sum(zeny), 0) FROM mails))::bigint AS zeny;

-- name: ExportSnapshot :one
-- ExportSnapshot names the snapshot of the transaction it runs in, so
-- pg_dump --snapshot dumps the rows that WorldCounts counted.
SELECT pg_export_snapshot()::text AS snapshot;
