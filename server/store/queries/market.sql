-- Queries for currencies, shop stock, mail and stores (decision 0024).

-- name: CharacterCurrencies :many
SELECT currency, amount FROM character_currencies WHERE character_id = @character_id ORDER BY currency;

-- name: AccountCurrencies :many
SELECT currency, amount FROM account_currencies WHERE account_id = @account_id ORDER BY currency;

-- name: DeleteCharacterCurrencies :exec
DELETE FROM character_currencies WHERE character_id = @character_id AND currency = ANY(@currencies::integer[]);

-- name: UpsertCharacterCurrencies :exec
INSERT INTO character_currencies (character_id, currency, amount)
SELECT @character_id, unnest(@currencies::integer[]), unnest(@amounts::bigint[])
ON CONFLICT (character_id, currency) DO UPDATE SET amount = EXCLUDED.amount;

-- name: DeleteAccountCurrencies :exec
DELETE FROM account_currencies WHERE account_id = @account_id AND currency = ANY(@currencies::integer[]);

-- name: UpsertAccountCurrencies :exec
INSERT INTO account_currencies (account_id, currency, amount)
SELECT @account_id, unnest(@currencies::integer[]), unnest(@amounts::bigint[])
ON CONFLICT (account_id, currency) DO UPDATE SET amount = EXCLUDED.amount;

-- name: ShopStock :many
SELECT shop, item_id, stock, filled_at FROM shop_stock ORDER BY shop, item_id;

-- name: UpsertShopStock :exec
INSERT INTO shop_stock (shop, item_id, stock, filled_at)
SELECT unnest(@shops::text[]), unnest(@item_ids::integer[]), unnest(@stocks::integer[]), unnest(@filled::timestamptz[])
ON CONFLICT (shop, item_id) DO UPDATE SET stock = EXCLUDED.stock, filled_at = EXCLUDED.filled_at;

-- name: DeleteShopStock :exec
DELETE FROM shop_stock
WHERE (shop, item_id) IN (SELECT unnest(@shops::text[]), unnest(@item_ids::integer[]));

-- name: MailIDFloor :one
SELECT coalesce(max(id), 0)::bigint FROM mails;

-- name: Mailbox :many
SELECT id, sender_id, sender_name, recipient_id, recipient_name, title, body, zeny, sent_at, read, returned
FROM mails WHERE recipient_id = @recipient_id ORDER BY id;

-- name: MailItems :many
SELECT mail_id, slot, item_id, amount, serial, refine, cards, rolls, card_rolls, random_options, identified, bound
FROM mail_items WHERE mail_id = ANY(@mail_ids::bigint[]) ORDER BY mail_id, slot;

-- name: UpsertMail :exec
INSERT INTO mails (id, sender_id, sender_name, recipient_id, recipient_name, title, body, zeny, sent_at, read, returned)
VALUES (@id, sqlc.narg('sender_id')::bigint, @sender_name, @recipient_id, @recipient_name, @title, @body, @zeny, @sent_at,
    @read, @returned)
ON CONFLICT (id) DO UPDATE SET zeny = EXCLUDED.zeny, read = EXCLUDED.read;

-- name: DeleteMails :exec
DELETE FROM mails WHERE id = ANY(@ids::bigint[]);

-- name: DeleteMailItems :exec
DELETE FROM mail_items WHERE mail_id = @mail_id;

-- name: InsertMailItems :exec
INSERT INTO mail_items (mail_id, slot, item_id, amount, serial, refine, cards, rolls, card_rolls, random_options, identified, bound)
SELECT @mail_id, unnest(@slots::smallint[]), unnest(@item_ids::integer[]), unnest(@amounts::integer[]),
    unnest(@serials::uuid[]), unnest(@refines::smallint[]), unnest(@cards::text[])::integer[],
    unnest(@rolls::text[])::jsonb, unnest(@card_rolls::text[])::jsonb,
    unnest(@random_options::text[])::jsonb, unnest(@identified::boolean[]), unnest(@bounds::smallint[]);

-- name: RecipientByName :one
-- RecipientByName finds a character by name, and counts its mails.
SELECT c.id, c.name, (SELECT count(*) FROM mails m WHERE m.recipient_id = c.id)::integer AS mails
FROM characters c WHERE lower(c.name) = lower(@name);

-- name: RecipientByID :one
SELECT c.id, c.name, (SELECT count(*) FROM mails m WHERE m.recipient_id = c.id)::integer AS mails
FROM characters c WHERE c.id = @id;

-- name: ExpiredMails :many
-- ExpiredMails finds the mails that mail_return_timer and
-- mail_delete_timer act on: mails sent before return_before that did
-- not come back and have a sender, and mails that came back or have no
-- sender sent before delete_before.
SELECT id, sender_id, sender_name, recipient_id, recipient_name, title, body, zeny, sent_at, read, returned
FROM mails
WHERE (sqlc.narg('return_before')::timestamptz IS NOT NULL AND NOT returned AND sender_id IS NOT NULL
        AND sent_at <= sqlc.narg('return_before')::timestamptz)
    OR (sqlc.narg('delete_before')::timestamptz IS NOT NULL AND (returned OR sender_id IS NULL)
        AND sent_at <= sqlc.narg('delete_before')::timestamptz)
ORDER BY id
LIMIT @lim;

-- name: UpsertStore :exec
INSERT INTO stores (character_id, buying, title, map, x, y, dir, zeny_limit, autotrade)
VALUES (@character_id, @buying, @title, @map, @x, @y, @dir, @zeny_limit, @autotrade)
ON CONFLICT (character_id, buying) DO UPDATE SET title = EXCLUDED.title, map = EXCLUDED.map, x = EXCLUDED.x,
    y = EXCLUDED.y, dir = EXCLUDED.dir, zeny_limit = EXCLUDED.zeny_limit, autotrade = EXCLUDED.autotrade;

-- name: DeleteStore :exec
DELETE FROM stores WHERE character_id = @character_id AND buying = @buying;

-- name: DeleteStoreItems :exec
DELETE FROM store_items WHERE character_id = @character_id AND buying = @buying;

-- name: InsertStoreItems :exec
INSERT INTO store_items (character_id, buying, idx, item_id, cart_serial, amount, price)
SELECT @character_id, @buying, unnest(@idxs::smallint[]), unnest(@item_ids::integer[]), unnest(@cart_serials::uuid[]),
    unnest(@amounts::integer[]), unnest(@prices::bigint[]);

-- name: AutotradeStores :many
SELECT character_id, buying, title, map, x, y, dir, zeny_limit FROM stores WHERE autotrade ORDER BY character_id, buying;

-- name: StoreItems :many
SELECT character_id, buying, idx, item_id, cart_serial, amount, price FROM store_items ORDER BY character_id, buying, idx;

-- name: DeleteIdleStores :exec
-- DeleteIdleStores removes the stores that were not on autotrade, at
-- start, as do_init_vending_autotrade clears the tables.
DELETE FROM stores WHERE NOT autotrade;
