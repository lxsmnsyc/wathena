-- Broken items, rentals and named items (review decision 58, area B9,
-- decision 0064).
--
-- Rules:
--   - broken is rAthena's item.attribute of 1. Before this migration the
--     zone kept it only while the character played.
--   - expires_at is a rental's end, as rAthena's item.expire_time: Unix
--     seconds, or 0 for an item that is not rented. Rentals live in the
--     inventory, a storage or a cart, as rAthena lets them go only
--     there, so only inventory and storage_items have the column.
--   - signed_by and signed_name are the character a script named the
--     item for, as getnameditem's CARD0_CREATE, or 0 and ''.
--   - character_skills.granted is rAthena's SKILL_FLAG_PERM_GRANTED: a
--     skill a script gave with SKILL_PERM_GRANT, which resetskill keeps.

-- +goose Up

ALTER TABLE inventory
    ADD COLUMN broken boolean NOT NULL DEFAULT false,
    ADD COLUMN expires_at bigint NOT NULL DEFAULT 0 CHECK (expires_at >= 0),
    ADD COLUMN signed_by integer NOT NULL DEFAULT 0 CHECK (signed_by >= 0),
    ADD COLUMN signed_name text NOT NULL DEFAULT '' CHECK (char_length(signed_name) <= 24);

ALTER TABLE storage_items
    ADD COLUMN broken boolean NOT NULL DEFAULT false,
    ADD COLUMN expires_at bigint NOT NULL DEFAULT 0 CHECK (expires_at >= 0),
    ADD COLUMN signed_by integer NOT NULL DEFAULT 0 CHECK (signed_by >= 0),
    ADD COLUMN signed_name text NOT NULL DEFAULT '' CHECK (char_length(signed_name) <= 24);

ALTER TABLE guild_storage
    ADD COLUMN broken boolean NOT NULL DEFAULT false,
    ADD COLUMN signed_by integer NOT NULL DEFAULT 0 CHECK (signed_by >= 0),
    ADD COLUMN signed_name text NOT NULL DEFAULT '' CHECK (char_length(signed_name) <= 24);

ALTER TABLE mail_items
    ADD COLUMN broken boolean NOT NULL DEFAULT false,
    ADD COLUMN signed_by integer NOT NULL DEFAULT 0 CHECK (signed_by >= 0),
    ADD COLUMN signed_name text NOT NULL DEFAULT '' CHECK (char_length(signed_name) <= 24);

ALTER TABLE character_skills ADD COLUMN granted boolean NOT NULL DEFAULT false;

-- +goose Down

ALTER TABLE character_skills DROP COLUMN granted;

DELETE FROM inventory WHERE expires_at > 0;
DELETE FROM storage_items WHERE expires_at > 0;
ALTER TABLE inventory DROP COLUMN broken, DROP COLUMN expires_at, DROP COLUMN signed_by, DROP COLUMN signed_name;
ALTER TABLE storage_items DROP COLUMN broken, DROP COLUMN expires_at, DROP COLUMN signed_by, DROP COLUMN signed_name;
ALTER TABLE guild_storage DROP COLUMN broken, DROP COLUMN signed_by, DROP COLUMN signed_name;
ALTER TABLE mail_items DROP COLUMN broken, DROP COLUMN signed_by, DROP COLUMN signed_name;
