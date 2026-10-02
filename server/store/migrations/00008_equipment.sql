-- Equipment (phase 4 step 1): where an item instance is equipped, its
-- refine level, and its cards.
--
-- Rules:
--   - equip holds the positions the item is equipped in, as rAthena's
--     EQP_ bits. 0 is not equipped.
--   - Cards are a list in slot order, not four columns (strategy section
--     6.8). Only filled slots are stored.
--   - The checks allow more than rAthena's limits (MAX_SLOTS 4,
--     MAX_REFINE 20), so a ruleset can raise a limit without a migration.

-- +goose Up

ALTER TABLE inventory
    ADD COLUMN equip integer NOT NULL DEFAULT 0 CHECK (equip BETWEEN 0 AND 4194303),
    ADD COLUMN refine smallint NOT NULL DEFAULT 0 CHECK (refine BETWEEN 0 AND 255),
    ADD COLUMN cards integer[] NOT NULL DEFAULT '{}' CHECK (
        cardinality(cards) <= 16
        AND (cardinality(cards) = 0 OR array_ndims(cards) = 1)
        AND array_position(cards, NULL) IS NULL
        AND 0 < ALL (cards)
    );

-- +goose Down

ALTER TABLE inventory DROP COLUMN cards, DROP COLUMN refine, DROP COLUMN equip;
