-- Guilds, guild storage and emblems (phase 5 step 5, decision 0021).
--
-- Rules:
--   - guilds are rAthena's guild table with its positions, skills and
--     expulsion list as JSON, since the server reads and writes each
--     guild whole. The server gives guild IDs itself, from the highest
--     saved ID.
--   - guild_members puts a character in a guild's slot, as
--     guild.member[]. Slot 0 is the master. A character is in at most
--     one guild. A slot is below 260: 200 members, the hard maximum of the
--     guild_members limit, and 60 more from Guild Extension.
--   - guild_relations holds allied and enemy guilds. An alliance has a
--     row on each side. An enemy relation has one on the side that made
--     it.
--   - guild_emblems holds each guild's emblem as the server re-encoded
--     it, never the uploaded file (strategy section 7.6).
--   - guild_storage holds the guild storage's stacks, as inventory does,
--     with slots up to 2,000, the hard maximum of the guild_storage
--     limit. A serial is in one place at a time: the transfer commits move
--     it between inventory and guild_storage in one transaction.
--   - Reports may name a guild's name or emblem, with a copy of the
--     emblem as it was.
--   - The chat log keeps guild lines.

-- +goose Up

CREATE TABLE guilds (
    id bigint PRIMARY KEY CHECK (id > 0),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 24),
    level smallint NOT NULL DEFAULT 1 CHECK (level BETWEEN 1 AND 50),
    -- exp is the exp toward the next level, at most MAX_GUILD_EXP.
    exp bigint NOT NULL DEFAULT 0 CHECK (exp BETWEEN 0 AND 2147483647),
    skill_points integer NOT NULL DEFAULT 0 CHECK (skill_points >= 0),
    -- skills maps skill IDs to levels, such as {"10004": 2}.
    skills jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(skills) = 'object'),
    -- positions is a list of { name, invite, expel, storage, exp_tax },
    -- at most 40, the hard maximum of guild_positions.
    positions jsonb NOT NULL CHECK (jsonb_typeof(positions) = 'array' AND jsonb_array_length(positions) BETWEEN 2 AND 40),
    notice_title text NOT NULL DEFAULT '' CHECK (char_length(notice_title) <= 59),
    notice_text text NOT NULL DEFAULT '' CHECK (char_length(notice_text) <= 119),
    -- expulsions is a list of { character_id, name, reason }, at most 32.
    expulsions jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(expulsions) = 'array' AND jsonb_array_length(expulsions) <= 32),
    last_master_change timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX guilds_name_key ON guilds (lower(name));

CREATE TABLE guild_members (
    guild_id bigint NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    character_id bigint NOT NULL UNIQUE REFERENCES characters (id) ON DELETE CASCADE,
    slot smallint NOT NULL CHECK (slot BETWEEN 0 AND 259),
    position smallint NOT NULL DEFAULT 0 CHECK (position BETWEEN 0 AND 39),
    -- exp is what the member paid through the exp tax.
    exp bigint NOT NULL DEFAULT 0 CHECK (exp BETWEEN 0 AND 2147483647),
    PRIMARY KEY (guild_id, slot)
);

CREATE TABLE guild_relations (
    guild_id bigint NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    other_id bigint NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    enemy boolean NOT NULL,
    PRIMARY KEY (guild_id, other_id, enemy),
    CHECK (guild_id <> other_id)
);

CREATE INDEX guild_relations_other ON guild_relations (other_id);

CREATE TABLE guild_emblems (
    guild_id bigint PRIMARY KEY REFERENCES guilds (id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version > 0),
    -- png is the re-encoded 24 x 24 emblem.
    png bytea NOT NULL CHECK (octet_length(png) BETWEEN 1 AND 50000),
    -- uploaded_by is the uploader's account, or 0 for the development
    -- server and scripts.
    uploaded_by bigint NOT NULL DEFAULT 0 CHECK (uploaded_by >= 0),
    uploaded_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE guild_storage (
    serial uuid PRIMARY KEY,
    guild_id bigint NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    slot smallint NOT NULL CHECK (slot BETWEEN 0 AND 1999),
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
    UNIQUE (guild_id, slot)
);

ALTER TABLE reports
    ADD COLUMN guild_id bigint NOT NULL DEFAULT 0 CHECK (guild_id >= 0),
    ADD COLUMN emblem_version integer NOT NULL DEFAULT 0 CHECK (emblem_version >= 0),
    ADD COLUMN emblem_png bytea CHECK (octet_length(emblem_png) <= 50000);

ALTER TABLE chat_log DROP CONSTRAINT chat_log_kind_check;
ALTER TABLE chat_log ADD CONSTRAINT chat_log_kind_check CHECK (kind IN ('local', 'whisper', 'channel', 'party', 'guild'));

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON guilds, guild_members, guild_relations, guild_emblems, guild_storage TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DELETE FROM chat_log WHERE kind = 'guild';
ALTER TABLE chat_log DROP CONSTRAINT chat_log_kind_check;
ALTER TABLE chat_log ADD CONSTRAINT chat_log_kind_check CHECK (kind IN ('local', 'whisper', 'channel', 'party'));

DELETE FROM reports WHERE guild_id <> 0;
ALTER TABLE reports DROP COLUMN emblem_png, DROP COLUMN emblem_version, DROP COLUMN guild_id;

DROP TABLE guild_storage;
DROP TABLE guild_emblems;
DROP TABLE guild_relations;
DROP TABLE guild_members;
DROP TABLE guilds;
