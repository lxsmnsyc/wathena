-- The progress of typed world events (phase 5 step 8, decision 0025).
--
-- Rules:
--   - One row per unit of a run's progress: a reward that was given,
--     such as "boss", "chest:3" or "result", an invasion wave that
--     started, such as "wave:2", or "end" for a run that ended before
--     its time.
--   - A reward's row is written in the same transaction as the
--     characters it changed, so a restart never gives it twice.
--   - A run is its event and its scheduled time, as in
--     world_event_runs. Recording a new run of an event deletes the
--     rows of its older runs.

-- +goose Up

CREATE TABLE world_event_progress (
    event text NOT NULL CHECK (char_length(event) BETWEEN 1 AND 160),
    scheduled_at timestamptz NOT NULL,
    unit text NOT NULL CHECK (char_length(unit) BETWEEN 1 AND 32),
    at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event, scheduled_at, unit)
);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, DELETE ON world_event_progress TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE world_event_progress;
