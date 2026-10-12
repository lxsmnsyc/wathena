-- The sizes scripts expand storages to (proposal 0002, review decision
-- 32, decision 0027).
--
-- Rules:
--   - One row per account storage or cart whose size player:expand
--     changed. holder is account or cart. account_id is set for an
--     account's storage, character_id for a cart.
--   - slots is the size the script set. The server keeps it within the
--     limit's base and max when it reads it, so a ruleset that changed
--     the range applies at once. A storage at the limit's base has no
--     row.
--   - The inventory's size stays in characters.inventory_size, which
--     player:expand also changes.
--   - Slots go up to 10,000, the engine's ceiling of storage_slots, and
--     1,000 for cart_slots.

-- +goose Up

CREATE TABLE storage_sizes (
    holder text NOT NULL CHECK (holder IN ('account', 'cart')),
    account_id bigint REFERENCES accounts (id) ON DELETE CASCADE,
    character_id bigint REFERENCES characters (id) ON DELETE CASCADE,
    slots integer NOT NULL CHECK (slots BETWEEN 1 AND 10000),
    CHECK ((holder = 'account') = (account_id IS NOT NULL) AND (holder = 'account') = (character_id IS NULL)),
    CHECK (holder <> 'cart' OR slots <= 1000)
);

CREATE UNIQUE INDEX storage_sizes_account ON storage_sizes (account_id) WHERE holder = 'account';
CREATE UNIQUE INDEX storage_sizes_cart ON storage_sizes (character_id) WHERE holder = 'cart';

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON storage_sizes TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE storage_sizes;
