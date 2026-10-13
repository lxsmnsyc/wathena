-- Queries of the staff tools on the game side (decision 0029): item
-- history, a character's logs, where a serial is, and the views of
-- offline characters. waserver runs them as the wathena_game role.

-- name: ItemLogBySerials :many
-- ItemLogBySerials lists the item log lines of some serials. Each
-- partition finds them through item_log_serial.
SELECT id, at, action, from_kind, from_id, from_storage, to_kind, to_id, to_storage, serial, source,
    item_id, amount, refine, cards, bound
FROM item_log
WHERE serial = ANY(@serials::uuid[])
ORDER BY at, id
LIMIT @max_rows;

-- name: ItemLogBySources :many
-- ItemLogBySources lists the item log lines whose source is one of some
-- serials: the splits and merges that came from them. Each partition
-- finds them through item_log_source.
SELECT id, at, action, from_kind, from_id, from_storage, to_kind, to_id, to_storage, serial, source,
    item_id, amount, refine, cards, bound
FROM item_log
WHERE source = ANY(@serials::uuid[])
ORDER BY at, id
LIMIT @max_rows;

-- name: LastItemLine :one
-- LastItemLine is the newest log line of a serial, with the copy's
-- details, for a restore that makes the serial again.
SELECT id, at, action, to_kind, to_id, item_id, amount, refine, cards, rolls, card_rolls, random_options, identified, bound
FROM item_log
WHERE serial = @serial
ORDER BY at DESC, id DESC
LIMIT 1;

-- name: ItemHolders :many
-- ItemHolders lists the saved places that hold a serial. A serial is in
-- at most one place, unless it was duplicated.
SELECT 'inventory'::text AS holder, character_id::bigint AS id, 0::integer AS storage, slot::integer AS slot, item_id, amount
FROM inventory WHERE inventory.serial = @serial
UNION ALL
SELECT s.holder, coalesce(s.account_id, s.character_id)::bigint, s.storage, s.slot::integer, s.item_id, s.amount
FROM storage_items s WHERE s.serial = @serial
UNION ALL
SELECT 'mail'::text, m.mail_id, 0, m.slot::integer, m.item_id, m.amount
FROM mail_items m WHERE m.serial = @serial
UNION ALL
SELECT 'guild_storage'::text, g.guild_id, 0, g.slot::integer, g.item_id, g.amount
FROM guild_storage g WHERE g.serial = @serial;

-- name: CharacterItemLog :many
-- CharacterItemLog is a page of a character's item log, newest first:
-- the lines from or to its inventory, cart or vending shop, before a
-- time and row ID, and within since and until.
SELECT id, at, action, from_kind, from_id, from_storage, to_kind, to_id, to_storage, serial, source,
    item_id, amount, refine, cards, bound
FROM item_log
WHERE ((from_kind = ANY(@kinds::text[]) AND from_id = @character_id)
        OR (to_kind = ANY(@kinds::text[]) AND to_id = @character_id))
    AND at <= @before_at AND (at, id) < (@before_at::timestamptz, @before_id::bigint)
    AND at >= @since AND at < @until
ORDER BY at DESC, id DESC
LIMIT @max_rows;

-- name: CharacterZenyLog :many
-- CharacterZenyLog is a page of a character's zeny and currency log,
-- newest first, as CharacterItemLog.
SELECT id, at, action, from_kind, from_id, to_kind, to_id, amount, currency
FROM zeny_log
WHERE ((from_kind = ANY(@kinds::text[]) AND from_id = @character_id)
        OR (to_kind = ANY(@kinds::text[]) AND to_id = @character_id))
    AND at <= @before_at AND (at, id) < (@before_at::timestamptz, @before_id::bigint)
    AND at >= @since AND at < @until
ORDER BY at DESC, id DESC
LIMIT @max_rows;

-- name: CharacterNames :many
SELECT id, name, account_id FROM characters WHERE id = ANY(@ids::bigint[]);

-- name: AccountNames :many
SELECT id, name FROM accounts WHERE id = ANY(@ids::bigint[]);

-- name: GuildNames :many
SELECT id, name FROM guilds WHERE id = ANY(@ids::bigint[]);

-- name: MailOwners :many
-- MailOwners names the recipients of some mails.
SELECT id, recipient_id, recipient_name FROM mails WHERE id = ANY(@ids::bigint[]);

-- name: ReadCharacter :one
-- ReadCharacter reads a character without starting a session, for the
-- staff tools' view of an offline character.
SELECT * FROM characters WHERE id = @id;

-- name: StorageItemsOf :many
-- StorageItemsOf lists the stacks of an account's storages and of a
-- character's cart and extra storages.
SELECT holder, storage, slot, item_id, amount, serial, refine, cards, rolls, card_rolls, random_options, identified, bound
FROM storage_items
WHERE (holder = 'account' AND account_id = @account_id) OR (holder <> 'account' AND character_id = @character_id)
ORDER BY holder, storage, slot;

-- name: CharacterGroups :one
-- CharacterGroups names a character's party and guild, or empty.
SELECT coalesce((SELECT p.name FROM party_members m JOIN parties p ON p.id = m.party_id
            WHERE m.character_id = @id LIMIT 1), '')::text AS party,
    coalesce((SELECT g.name FROM guild_members m JOIN guilds g ON g.id = m.guild_id
            WHERE m.character_id = @id LIMIT 1), '')::text AS guild;

-- name: SanctionsOf :many
-- SanctionsOf lists the sanctions in effect of some characters.
SELECT id, character_id::bigint AS character_id, kind, reason, started_at, ends_at, issued_by
FROM sanctions
WHERE character_id = ANY(@ids::bigint[]) AND lifted_at IS NULL AND ends_at > now()
ORDER BY character_id, started_at, id;

-- name: CharacterIDsOfAccount :many
SELECT id FROM characters WHERE account_id = @account_id ORDER BY id;

-- name: MailRecipient :one
-- MailRecipient is the recipient of a saved mail.
SELECT recipient_id FROM mails WHERE id = @id;
