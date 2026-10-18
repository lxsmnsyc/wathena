-- Queries of the account page (decision 0032): block lists, reports and
-- sanctions, data exports and account deletion. The account service
-- runs the first group as wathena_account. waserver runs the rest as
-- wathena_game.

-- name: AccountOverview :one
SELECT id, name, created_at FROM accounts WHERE id = @account_id;

-- name: LastExport :one
SELECT exported_at FROM account_exports WHERE account_id = @account_id;

-- name: RecordExport :execrows
-- RecordExport records an export now. It changes nothing, and returns
-- 0, when the last export was less than a day ago.
INSERT INTO account_exports (account_id, exported_at) VALUES (@account_id, now())
ON CONFLICT (account_id) DO UPDATE SET exported_at = EXCLUDED.exported_at
WHERE account_exports.exported_at <= now() - interval '1 day';

-- name: RecentLogins :many
-- RecentLogins are the account's login records since a time,
-- newest first.
SELECT at, ip, user_agent FROM account_logins
WHERE account_id = @account_id AND at > @since
ORDER BY at DESC, id DESC;

-- name: AccountDeletion :one
SELECT requested_at, delete_after FROM account_deletions WHERE account_id = @account_id;

-- name: RequestDeletion :one
-- RequestDeletion asks for the account's deletion after wait, with
-- request_account_deletion, which sets both times from the database
-- clock. It returns false when a request waits already.
SELECT request_account_deletion(@account_id::bigint, @wait::interval)::boolean AS requested;

-- name: CancelDeletion :execrows
DELETE FROM account_deletions WHERE account_id = @account_id;

-- name: DeletionOfCharacter :one
-- DeletionOfCharacter is the deletion request of a character's account,
-- which refuses the character's game logins.
SELECT d.requested_at, d.delete_after
FROM account_deletions d JOIN characters c ON c.account_id = d.account_id
WHERE c.id = @character_id;

-- name: DueDeletions :many
-- DueDeletions are the accounts whose deletion is due at at, and on the
-- database clock, oldest first.
SELECT account_id FROM account_deletions
WHERE delete_after <= least(@at::timestamptz, now())
ORDER BY delete_after, account_id
LIMIT @max_rows;

-- name: DeleteAccount :one
-- DeleteAccount deletes an account whose deletion is due at at, with
-- delete_account. It returns false when none is due.
SELECT delete_account(@account_id::bigint, @at::timestamptz, @account_storages::integer[])::boolean AS deleted;

-- name: CharactersOfAccount :many
SELECT id, slot, name FROM characters WHERE account_id = @account_id ORDER BY slot;

-- name: BlocksOfCharacters :many
SELECT character_id, name FROM character_blocks
WHERE character_id = ANY(@ids::bigint[])
ORDER BY character_id, position;

-- name: DeleteCharacterBlock :execrows
-- DeleteCharacterBlock takes a name off an offline character's block
-- list, in any case. The positions keep their order.
DELETE FROM character_blocks
WHERE character_id = @character_id AND lower(name) = lower(@name);

-- name: ReportsFiled :many
-- ReportsFiled are the reports some characters filed, newest first.
SELECT id, created_at, reporter_id, reporter_name, target_name, line_id, reason, status, outcome, guild_id, emblem_version
FROM reports
WHERE reporter_id = ANY(@ids::bigint[]) AND reporter_id <> 0
ORDER BY id DESC
LIMIT @max_rows;

-- name: ChatLinesOf :many
-- ChatLinesOf are the chat lines some characters said that the log
-- still keeps, oldest first.
SELECT id, at, kind, character_id, name, target, map, text
FROM chat_log
WHERE character_id = ANY(@ids::bigint[]) AND character_id <> 0
ORDER BY at, id
LIMIT @max_rows;

-- name: AccountItemLog :many
-- AccountItemLog lists the item log lines of an account's characters,
-- oldest first: from or to their inventories, carts, vending shops and
-- their own extra storages, and the account's storages.
SELECT id, at, action, from_kind, from_id, from_storage, to_kind, to_id, to_storage, serial, source,
    item_id, amount, refine, cards, bound
FROM item_log
WHERE (from_kind = ANY(@kinds::text[]) AND from_id = ANY(@ids::bigint[]))
    OR (to_kind = ANY(@kinds::text[]) AND to_id = ANY(@ids::bigint[]))
    OR (from_kind = 'storage' AND from_id = @account_id AND (from_storage = 0 OR from_storage = ANY(@account_storages::integer[])))
    OR (to_kind = 'storage' AND to_id = @account_id AND (to_storage = 0 OR to_storage = ANY(@account_storages::integer[])))
    OR (from_kind = 'storage' AND from_id = ANY(@ids::bigint[]) AND from_storage = ANY(@character_storages::integer[]))
    OR (to_kind = 'storage' AND to_id = ANY(@ids::bigint[]) AND to_storage = ANY(@character_storages::integer[]))
ORDER BY at, id
LIMIT @max_rows;

-- name: AccountZenyLog :many
-- AccountZenyLog lists the zeny and currency log lines of an account's
-- characters and of its storage, oldest first.
SELECT id, at, action, from_kind, from_id, to_kind, to_id, amount, currency
FROM zeny_log
WHERE (from_kind = ANY(@kinds::text[]) AND from_id = ANY(@ids::bigint[]))
    OR (to_kind = ANY(@kinds::text[]) AND to_id = ANY(@ids::bigint[]))
    OR (from_kind = 'storage' AND from_id = @account_id)
    OR (to_kind = 'storage' AND to_id = @account_id)
ORDER BY at, id
LIMIT @max_rows;
