-- Timed bonuses follow rAthena's bonus_script for a key the player
-- already has (proposal 0002, review decision 19).
--
-- Rules:
--   - The repeat mode of player:add_timed_bonuses only decides what one
--     call does. A saved copy does not need it, so the stack column goes.
--   - A character may have several copies with one key, from the repeat
--     mode "add". Each is its own row, by position, as before.

-- +goose Up

ALTER TABLE character_timed_bonuses DROP COLUMN stack;

-- +goose Down

ALTER TABLE character_timed_bonuses ADD COLUMN stack boolean NOT NULL DEFAULT false;
