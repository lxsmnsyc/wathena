-- Parties, friend lists and limits (phase 5 step 4, decision 0020).
--
-- Rules:
--   - parties are rAthena's party table: a name, the options and the
--     leader. The server gives party IDs itself, from the highest saved
--     ID, since it holds every party in memory.
--   - party_members puts a character in a party's slot, as
--     party.member[MAX_PARTY]. A character is in at most one party. A
--     slot is below 64, the hard maximum of the party_members limit.
--   - character_friends holds one side of a friendship: friend_id on the
--     list of character_id, in the order added.
--   - The block list holds up to 100 names, the hard maximum of the
--     block_list limit. A ruleset that lowers a limit deletes nothing.
--   - The chat log keeps party lines.

-- +goose Up

CREATE TABLE parties (
    id bigint PRIMARY KEY CHECK (id > 0),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 24),
    -- exp_even is exp 1, item_even is item & 2, and pickup_shared is
    -- item & 1.
    exp_even boolean NOT NULL DEFAULT false,
    item_even boolean NOT NULL DEFAULT false,
    pickup_shared boolean NOT NULL DEFAULT false,
    -- leader_id has no foreign key: a party whose leader was deleted
    -- loads with its first member as the leader.
    leader_id bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX parties_name_key ON parties (lower(name));

CREATE TABLE party_members (
    party_id bigint NOT NULL REFERENCES parties (id) ON DELETE CASCADE,
    character_id bigint NOT NULL UNIQUE REFERENCES characters (id) ON DELETE CASCADE,
    slot smallint NOT NULL CHECK (slot BETWEEN 0 AND 63),
    PRIMARY KEY (party_id, slot)
);

CREATE TABLE character_friends (
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    friend_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    -- added keeps the order the friends were added in.
    added bigint GENERATED ALWAYS AS IDENTITY,
    PRIMARY KEY (character_id, friend_id),
    CHECK (character_id <> friend_id)
);

CREATE INDEX character_friends_friend ON character_friends (friend_id);

ALTER TABLE character_blocks DROP CONSTRAINT character_blocks_position_check;
ALTER TABLE character_blocks ADD CONSTRAINT character_blocks_position_check CHECK (position BETWEEN 0 AND 99);

ALTER TABLE chat_log DROP CONSTRAINT chat_log_kind_check;
ALTER TABLE chat_log ADD CONSTRAINT chat_log_kind_check CHECK (kind IN ('local', 'whisper', 'channel', 'party'));

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON parties, party_members, character_friends TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DELETE FROM chat_log WHERE kind = 'party';
ALTER TABLE chat_log DROP CONSTRAINT chat_log_kind_check;
ALTER TABLE chat_log ADD CONSTRAINT chat_log_kind_check CHECK (kind IN ('local', 'whisper', 'channel'));

DELETE FROM character_blocks WHERE position > 19;
ALTER TABLE character_blocks DROP CONSTRAINT character_blocks_position_check;
ALTER TABLE character_blocks ADD CONSTRAINT character_blocks_position_check CHECK (position BETWEEN 0 AND 19);

DROP TABLE character_friends;
DROP TABLE party_members;
DROP TABLE parties;
