-- Rolled item values and random options (phase 4 step 6, strategy
-- section 5.4).
--
-- Rules:
--   - rolls holds a copy's rolled values by name, such as
--     {"boss_damage": 31}. Values are whole numbers. The server checks
--     them against the item's data when it loads them: values outside a
--     roll's range are clamped and logged.
--   - card_rolls holds the rolls of each card, in the order of cards,
--     such as [{"boss_damage": 31}, {}]. It may be shorter than cards:
--     the cards after its end have no rolls.
--   - random_options holds rAthena's random options, in option order,
--     such as [{"option": "max_hp", "value": 100, "param": 0}]. Options
--     are stored by their name in data/random_options.yml, not by
--     rAthena's ID, so mods can add options without IDs.
--   - identified is false for a copy whose hidden rolls the player has
--     not seen yet.
--   - The checks allow more than the server's limits (8 rolls, 5
--     options), so a later change can raise a limit without a migration.

-- +goose Up

ALTER TABLE inventory
    ADD COLUMN rolls jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(rolls) = 'object'),
    ADD COLUMN card_rolls jsonb NOT NULL DEFAULT '[]' CHECK (
        jsonb_typeof(card_rolls) = 'array' AND jsonb_array_length(card_rolls) <= 16
    ),
    ADD COLUMN random_options jsonb NOT NULL DEFAULT '[]' CHECK (
        jsonb_typeof(random_options) = 'array' AND jsonb_array_length(random_options) <= 16
    ),
    ADD COLUMN identified boolean NOT NULL DEFAULT true;

-- +goose Down

ALTER TABLE inventory
    DROP COLUMN identified,
    DROP COLUMN random_options,
    DROP COLUMN card_rolls,
    DROP COLUMN rolls;
