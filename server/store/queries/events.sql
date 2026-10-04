-- Queries for the runs of world events (strategy section 8.8). waserver
-- runs them as wathena_game.

-- name: EventRuns :many
SELECT event, scheduled_at, started_at, ends_at FROM world_event_runs;

-- name: RecordEventRun :exec
INSERT INTO world_event_runs (event, scheduled_at, started_at, ends_at)
VALUES (@event, @scheduled_at, @started_at, @ends_at)
ON CONFLICT (event) DO UPDATE
SET scheduled_at = excluded.scheduled_at, started_at = excluded.started_at, ends_at = excluded.ends_at;

-- name: DeleteOldEventProgress :exec
DELETE FROM world_event_progress WHERE event = @event AND scheduled_at < @scheduled_at;

-- name: EventProgress :many
SELECT unit FROM world_event_progress WHERE event = @event AND scheduled_at = @scheduled_at ORDER BY unit;

-- name: RecordEventProgress :exec
INSERT INTO world_event_progress (event, scheduled_at, unit) VALUES (@event, @scheduled_at, @unit)
ON CONFLICT DO NOTHING;
