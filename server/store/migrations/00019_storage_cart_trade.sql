-- Storage, carts, trades, bound items and limit ranges (phase 5 step
-- 6, decision 0023).
--
-- Rules:
--   - storage_items holds the stacks of accounts' storages, extra
--     storages and carts. holder is account, character or cart.
--     account_id is set for an account's storages, character_id for a
--     character's storages and its cart. storage is the extra storage's
--     record ID, or 0 for the account's storage and for a cart.
--   - Slots go up to 9,999: the engine's ceiling of storage_slots and
--     guild_storage is 10,000, and of cart_slots 1,000.
--   - bound is rAthena's bound_type: 0 none, 1 account, 2 guild, 3 party,
--     4 character. Bound and unbound copies never stack.
--   - The serials of inventory, guild_storage and storage_items are
--     checked at the end of each transaction. A commit writes each
--     character and storage in turn, so a whole stack that changed hands
--     is in two rows until the row it left is deleted.
--   - item_log names an extra storage in from_storage and to_storage.
--   - The checks that held the old hard maximums now hold the engine's
--     ceilings: 1,000 inventory slots, 256 party members, 512 guild
--     members and 1,000 block list names.
--   - mod_ids takes every kind of server/mod's IDRanges. Migration 00005
--     took only item, monster, skill, job, quest and achievement, so with
--     a database the IDs of mod companions and mounts (decision 0016)
--     and of mod storages failed to save.

-- +goose Up

ALTER TABLE mod_ids DROP CONSTRAINT mod_ids_kind_check;
ALTER TABLE mod_ids ADD CONSTRAINT mod_ids_kind_check
    CHECK (kind IN ('item', 'monster', 'skill', 'job', 'companion', 'mount', 'storage', 'quest', 'achievement'));

CREATE TABLE storage_items (
    serial uuid NOT NULL,
    holder text NOT NULL CHECK (holder IN ('account', 'character', 'cart')),
    account_id bigint REFERENCES accounts (id) ON DELETE CASCADE,
    character_id bigint REFERENCES characters (id) ON DELETE CASCADE,
    storage integer NOT NULL DEFAULT 0 CHECK (storage >= 0),
    slot smallint NOT NULL CHECK (slot BETWEEN 0 AND 9999),
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
    CONSTRAINT storage_items_pkey PRIMARY KEY (serial) DEFERRABLE INITIALLY DEFERRED,
    CHECK ((holder = 'account') = (account_id IS NOT NULL) AND (holder = 'account') = (character_id IS NULL)),
    CHECK (holder <> 'cart' OR storage = 0)
);

CREATE UNIQUE INDEX storage_items_account_slot ON storage_items (account_id, storage, slot) WHERE holder = 'account';
CREATE UNIQUE INDEX storage_items_character_slot ON storage_items (character_id, holder, storage, slot) WHERE holder <> 'account';

ALTER TABLE inventory DROP CONSTRAINT inventory_pkey;
ALTER TABLE inventory ADD CONSTRAINT inventory_pkey PRIMARY KEY (serial) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE inventory ADD COLUMN bound smallint NOT NULL DEFAULT 0 CHECK (bound BETWEEN 0 AND 4);
ALTER TABLE inventory DROP CONSTRAINT inventory_slot_check;
ALTER TABLE inventory ADD CONSTRAINT inventory_slot_check CHECK (slot BETWEEN 0 AND 999);
ALTER TABLE characters DROP CONSTRAINT characters_inventory_size_check;
ALTER TABLE characters ADD CONSTRAINT characters_inventory_size_check CHECK (inventory_size BETWEEN 0 AND 1000);

ALTER TABLE guild_storage DROP CONSTRAINT guild_storage_pkey;
ALTER TABLE guild_storage ADD CONSTRAINT guild_storage_pkey PRIMARY KEY (serial) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE guild_storage ADD COLUMN bound smallint NOT NULL DEFAULT 0 CHECK (bound BETWEEN 0 AND 4);
ALTER TABLE guild_storage DROP CONSTRAINT guild_storage_slot_check;
ALTER TABLE guild_storage ADD CONSTRAINT guild_storage_slot_check CHECK (slot BETWEEN 0 AND 9999);

ALTER TABLE party_members DROP CONSTRAINT party_members_slot_check;
ALTER TABLE party_members ADD CONSTRAINT party_members_slot_check CHECK (slot BETWEEN 0 AND 255);
ALTER TABLE guild_members DROP CONSTRAINT guild_members_slot_check;
ALTER TABLE guild_members ADD CONSTRAINT guild_members_slot_check CHECK (slot BETWEEN 0 AND 511);
ALTER TABLE character_blocks DROP CONSTRAINT character_blocks_position_check;
ALTER TABLE character_blocks ADD CONSTRAINT character_blocks_position_check CHECK (position BETWEEN 0 AND 999);

ALTER TABLE item_log
    ADD COLUMN from_storage integer NOT NULL DEFAULT 0 CHECK (from_storage >= 0),
    ADD COLUMN to_storage integer NOT NULL DEFAULT 0 CHECK (to_storage >= 0),
    ADD COLUMN bound smallint NOT NULL DEFAULT 0 CHECK (bound BETWEEN 0 AND 4);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON storage_items TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

-- The old check does not take the new kinds, so their IDs go. A mod's
-- record of these kinds gets a new ID if the migration runs again.
DELETE FROM mod_ids WHERE kind IN ('companion', 'mount', 'storage');
ALTER TABLE mod_ids DROP CONSTRAINT mod_ids_kind_check;
ALTER TABLE mod_ids ADD CONSTRAINT mod_ids_kind_check
    CHECK (kind IN ('item', 'monster', 'skill', 'job', 'quest', 'achievement'));

ALTER TABLE item_log DROP COLUMN bound, DROP COLUMN to_storage, DROP COLUMN from_storage;

DELETE FROM character_blocks WHERE position > 99;
ALTER TABLE character_blocks DROP CONSTRAINT character_blocks_position_check;
ALTER TABLE character_blocks ADD CONSTRAINT character_blocks_position_check CHECK (position BETWEEN 0 AND 99);
DELETE FROM guild_members WHERE slot > 259;
ALTER TABLE guild_members DROP CONSTRAINT guild_members_slot_check;
ALTER TABLE guild_members ADD CONSTRAINT guild_members_slot_check CHECK (slot BETWEEN 0 AND 259);
DELETE FROM party_members WHERE slot > 63;
ALTER TABLE party_members DROP CONSTRAINT party_members_slot_check;
ALTER TABLE party_members ADD CONSTRAINT party_members_slot_check CHECK (slot BETWEEN 0 AND 63);

DELETE FROM guild_storage WHERE slot > 1999;
ALTER TABLE guild_storage DROP CONSTRAINT guild_storage_slot_check;
ALTER TABLE guild_storage ADD CONSTRAINT guild_storage_slot_check CHECK (slot BETWEEN 0 AND 1999);
ALTER TABLE guild_storage DROP COLUMN bound;
ALTER TABLE guild_storage DROP CONSTRAINT guild_storage_pkey;
ALTER TABLE guild_storage ADD CONSTRAINT guild_storage_pkey PRIMARY KEY (serial);

UPDATE characters SET inventory_size = 200 WHERE inventory_size > 200;
ALTER TABLE characters DROP CONSTRAINT characters_inventory_size_check;
ALTER TABLE characters ADD CONSTRAINT characters_inventory_size_check CHECK (inventory_size BETWEEN 0 AND 200);
DELETE FROM inventory WHERE slot > 199;
ALTER TABLE inventory DROP CONSTRAINT inventory_slot_check;
ALTER TABLE inventory ADD CONSTRAINT inventory_slot_check CHECK (slot BETWEEN 0 AND 199);
ALTER TABLE inventory DROP COLUMN bound;
ALTER TABLE inventory DROP CONSTRAINT inventory_pkey;
ALTER TABLE inventory ADD CONSTRAINT inventory_pkey PRIMARY KEY (serial);

DROP TABLE storage_items;
