-- Mail, vending, buying stores, shops and currencies (phase 5 step 7,
-- decision 0024).
--
-- Rules:
--   - mod_ids takes the currency kind of data/currencies.yml.
--   - character_currencies holds a character's own currencies, and
--     account_currencies an account's, which its characters share. A
--     currency at 0 has no row. Amounts are at most MAX_ZENY, as
--     pc_getcash caps cash points.
--   - zeny_log names a currency's moves: currency is its ID, 0 for zeny.
--   - shop_stock holds the stock of the shop items of data/npcs.yml that
--     have one, which every player shares, and when it last filled.
--   - mails are RODEX mails, and mail_items their attachments. A mail
--     from a script has no sender_id, as rAthena's send_id 0. Deleting a
--     character deletes its mails. returned marks a mail that came back
--     to its sender. Slots go up to 19: the ceiling of mail_items is 20.
--   - stores are the open vending shops and buying stores, and
--     store_items their lines, saved in the commits of their sales, so
--     autotrade stores open again after a restart. cart_serial names a
--     vending line's cart stack.
--   - characters count the mails sent today, for mail_daily_count:
--     mail_day is the day as YYYYMMDD.

-- +goose Up

ALTER TABLE mod_ids DROP CONSTRAINT mod_ids_kind_check;
ALTER TABLE mod_ids ADD CONSTRAINT mod_ids_kind_check
    CHECK (kind IN ('item', 'monster', 'skill', 'job', 'companion', 'mount', 'storage', 'currency', 'quest', 'achievement'));

ALTER TABLE characters
    ADD COLUMN mail_day integer NOT NULL DEFAULT 0 CHECK (mail_day >= 0),
    ADD COLUMN mail_sent integer NOT NULL DEFAULT 0 CHECK (mail_sent >= 0);

CREATE TABLE character_currencies (
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    currency integer NOT NULL CHECK (currency > 0),
    amount bigint NOT NULL CHECK (amount BETWEEN 1 AND 2147483647),
    PRIMARY KEY (character_id, currency)
);

CREATE TABLE account_currencies (
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    currency integer NOT NULL CHECK (currency > 0),
    amount bigint NOT NULL CHECK (amount BETWEEN 1 AND 2147483647),
    PRIMARY KEY (account_id, currency)
);

ALTER TABLE zeny_log ADD COLUMN currency integer NOT NULL DEFAULT 0 CHECK (currency >= 0);

CREATE TABLE shop_stock (
    shop text NOT NULL CHECK (char_length(shop) BETWEEN 1 AND 256),
    item_id integer NOT NULL CHECK (item_id > 0),
    stock integer NOT NULL CHECK (stock >= 0),
    filled_at timestamptz NOT NULL,
    PRIMARY KEY (shop, item_id)
);

CREATE TABLE mails (
    id bigint PRIMARY KEY CHECK (id > 0),
    sender_id bigint REFERENCES characters (id) ON DELETE SET NULL,
    sender_name text NOT NULL CHECK (char_length(sender_name) BETWEEN 1 AND 24),
    recipient_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    recipient_name text NOT NULL CHECK (char_length(recipient_name) BETWEEN 1 AND 24),
    title text NOT NULL CHECK (octet_length(title) BETWEEN 1 AND 39),
    body text NOT NULL DEFAULT '' CHECK (octet_length(body) <= 499),
    zeny bigint NOT NULL DEFAULT 0 CHECK (zeny BETWEEN 0 AND 2147483647),
    sent_at timestamptz NOT NULL,
    read boolean NOT NULL DEFAULT false,
    returned boolean NOT NULL DEFAULT false
);

CREATE INDEX mails_recipient ON mails (recipient_id, id);
CREATE INDEX mails_sent ON mails (sent_at);

CREATE TABLE mail_items (
    serial uuid NOT NULL,
    mail_id bigint NOT NULL REFERENCES mails (id) ON DELETE CASCADE,
    slot smallint NOT NULL CHECK (slot BETWEEN 0 AND 19),
    item_id integer NOT NULL CHECK (item_id > 0),
    amount integer NOT NULL CHECK (amount BETWEEN 1 AND 30000),
    refine smallint NOT NULL DEFAULT 0 CHECK (refine BETWEEN 0 AND 255),
    cards integer[] NOT NULL DEFAULT '{}' CHECK (
        cardinality(cards) <= 16
        AND (cardinality(cards) = 0 OR array_ndims(cards) = 1)
        AND array_position(cards, NULL) IS NULL
        AND 0 < ALL (cards)
    ),
    rolls jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(rolls) = 'object'),
    card_rolls jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(card_rolls) = 'array' AND jsonb_array_length(card_rolls) <= 16),
    random_options jsonb NOT NULL DEFAULT '[]' CHECK (
        jsonb_typeof(random_options) = 'array' AND jsonb_array_length(random_options) <= 16
    ),
    identified boolean NOT NULL DEFAULT true,
    bound smallint NOT NULL DEFAULT 0 CHECK (bound BETWEEN 0 AND 4),
    CONSTRAINT mail_items_pkey PRIMARY KEY (serial) DEFERRABLE INITIALLY DEFERRED,
    UNIQUE (mail_id, slot)
);

CREATE TABLE stores (
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    buying boolean NOT NULL,
    title text NOT NULL CHECK (octet_length(title) BETWEEN 1 AND 79),
    map text NOT NULL CHECK (char_length(map) BETWEEN 1 AND 64),
    x integer NOT NULL CHECK (x BETWEEN 0 AND 1023),
    y integer NOT NULL CHECK (y BETWEEN 0 AND 1023),
    dir smallint NOT NULL DEFAULT 0 CHECK (dir BETWEEN 0 AND 7),
    zeny_limit bigint NOT NULL DEFAULT 0 CHECK (zeny_limit BETWEEN 0 AND 2147483647),
    autotrade boolean NOT NULL DEFAULT false,
    PRIMARY KEY (character_id, buying)
);

CREATE TABLE store_items (
    character_id bigint NOT NULL,
    buying boolean NOT NULL,
    idx smallint NOT NULL CHECK (idx BETWEEN 0 AND 11),
    item_id integer NOT NULL CHECK (item_id > 0),
    cart_serial uuid,
    amount integer NOT NULL CHECK (amount BETWEEN 1 AND 30000),
    price bigint NOT NULL CHECK (price BETWEEN 1 AND 2147483647),
    PRIMARY KEY (character_id, buying, idx),
    FOREIGN KEY (character_id, buying) REFERENCES stores (character_id, buying) ON DELETE CASCADE,
    CHECK (buying OR cart_serial IS NOT NULL)
);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON character_currencies, account_currencies, shop_stock, mails, mail_items,
            stores, store_items TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE store_items;
DROP TABLE stores;
DROP TABLE mail_items;
DROP TABLE mails;
DROP TABLE shop_stock;
ALTER TABLE zeny_log DROP COLUMN currency;
DROP TABLE account_currencies;
DROP TABLE character_currencies;
ALTER TABLE characters DROP COLUMN mail_sent, DROP COLUMN mail_day;

-- The old check does not take currencies, so their IDs go.
DELETE FROM mod_ids WHERE kind = 'currency';
ALTER TABLE mod_ids DROP CONSTRAINT mod_ids_kind_check;
ALTER TABLE mod_ids ADD CONSTRAINT mod_ids_kind_check
    CHECK (kind IN ('item', 'monster', 'skill', 'job', 'companion', 'mount', 'storage', 'quest', 'achievement'));
