-- Chat and moderation (phase 5 step 2, decision 0018, strategy section
-- 7.10).
--
-- Rules:
--   - character_blocks is a character's block list, in the order the
--     names were added. A name may be any valid character name, as
--     rAthena's ignore list takes any name.
--   - sanctions keeps every mute and jail, ended ones too, for the staff
--     tools. A sanction that ended early has lifted_at. The server loads
--     the ones that have not ended.
--   - chat_log keeps the lines players said. Rows have no foreign keys,
--     so they outlive the characters they name. The store deletes rows
--     older than the retention period, 30 days by default, except rows
--     that an open report holds.
--   - reports are players' reports of a player or a line. report_lines
--     ties a report to the chat lines its reporter saw in the 5 minutes
--     before it. A line goes once its report is closed and it is older
--     than the retention period.

-- +goose Up

CREATE TABLE character_blocks (
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    -- position keeps the order. MAX_IGNORE_LIST is 20.
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 19),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 24),
    PRIMARY KEY (character_id, position)
);

CREATE TABLE sanctions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('mute', 'jail')),
    reason text NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 255),
    started_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL CHECK (ends_at > started_at),
    lifted_at timestamptz,
    issued_by text NOT NULL DEFAULT '' CHECK (octet_length(issued_by) <= 200),
    -- Where a jailed character goes back to, as SC_JAILED's val3 and
    -- val4. Empty for a mute.
    return_map text NOT NULL DEFAULT '' CHECK (char_length(return_map) <= 64),
    return_x integer NOT NULL DEFAULT 0 CHECK (return_x BETWEEN 0 AND 1023),
    return_y integer NOT NULL DEFAULT 0 CHECK (return_y BETWEEN 0 AND 1023),
    UNIQUE (character_id, kind, started_at)
);

CREATE INDEX sanctions_active ON sanctions (character_id, ends_at) WHERE lifted_at IS NULL;

CREATE TABLE chat_log (
    id uuid PRIMARY KEY,
    at timestamptz NOT NULL,
    kind text NOT NULL CHECK (kind IN ('local', 'whisper', 'channel')),
    -- character_id is 0 for a character that only lives in memory.
    character_id bigint NOT NULL DEFAULT 0 CHECK (character_id >= 0),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 24),
    -- target is the whisper's target or the channel's name.
    target text NOT NULL DEFAULT '' CHECK (char_length(target) <= 32),
    map text NOT NULL CHECK (char_length(map) <= 64),
    text text NOT NULL CHECK (char_length(text) BETWEEN 1 AND 255)
);

CREATE INDEX chat_log_at ON chat_log (at);
CREATE INDEX chat_log_character ON chat_log (character_id, at);

CREATE TABLE reports (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL,
    reporter_id bigint NOT NULL DEFAULT 0 CHECK (reporter_id >= 0),
    reporter_name text NOT NULL CHECK (char_length(reporter_name) BETWEEN 1 AND 24),
    target_id bigint NOT NULL DEFAULT 0 CHECK (target_id >= 0),
    target_name text NOT NULL CHECK (char_length(target_name) BETWEEN 1 AND 24),
    -- line_id is the reported line, or NULL for a report of a player.
    line_id uuid,
    reason text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 255),
    map text NOT NULL CHECK (char_length(map) <= 64),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    closed_at timestamptz,
    CHECK ((status = 'closed') = (closed_at IS NOT NULL))
);

CREATE INDEX reports_open ON reports (created_at) WHERE status = 'open';
CREATE INDEX reports_target ON reports (lower(target_name), created_at);

CREATE TABLE report_lines (
    report_id bigint NOT NULL REFERENCES reports (id) ON DELETE CASCADE,
    line_id uuid NOT NULL REFERENCES chat_log (id) ON DELETE CASCADE,
    PRIMARY KEY (report_id, line_id)
);

CREATE INDEX report_lines_line ON report_lines (line_id);

-- Rights, when the roles exist (deploy/postgres/roles.sql): waserver
-- reads and writes the rows. The account service has no rights on them.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON character_blocks, sanctions, chat_log, reports, report_lines TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE report_lines;
DROP TABLE reports;
DROP TABLE chat_log;
DROP TABLE sanctions;
DROP TABLE character_blocks;
