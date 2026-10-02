-- World events (phase 3 step 6, strategy section 8.8).
--
-- Rules:
--   - One row per event: its last run. The scheduler writes it before
--     the run starts, so a restart never runs the same time twice.
--   - Times are in UTC. The server's time zone is only used to match
--     schedules.

-- +goose Up

CREATE TABLE world_event_runs (
    -- The event's full name, such as example.halloween:Pumpkin_King.
    event text PRIMARY KEY CHECK (char_length(event) BETWEEN 1 AND 160),
    -- The time the trigger gave, when the run started, and when it ends.
    scheduled_at timestamptz NOT NULL,
    started_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL CHECK (ends_at > started_at)
);

-- Rights, when the roles exist (deploy/postgres/roles.sql): waserver
-- reads and writes the runs.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE ON world_event_runs TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE world_event_runs;
