-- The world, events and mods tools (phase 6 step 4, proposal 0002
-- review decision 36, decision 0031).
--
-- Rules:
--   - event_overrides keeps staff changes of world events, over the
--     config: whether the event is on, its trigger and its duration. A
--     NULL column keeps the config's value. A reset to the config
--     deletes the row. Each row says who made the change and when.
--   - trigger is the trigger in the form of data/events.yml, such as
--     {"daily": "20:00"}.
--   - mod_switches keeps the mods that staff turned off: one row per
--     mod and map, where map '' is every zone. waserver reads them at
--     start, so a mod stays off across restarts.
--   - The game role writes both, with the audit row of each change in
--     the same transaction. The account role has no rights on them.

-- +goose Up

CREATE TABLE event_overrides (
    -- The event's full name, such as example.halloween:Pumpkin_King.
    event text PRIMARY KEY CHECK (char_length(event) BETWEEN 1 AND 160),
    enabled boolean,
    trigger jsonb CHECK (trigger IS NULL OR jsonb_typeof(trigger) = 'object' AND octet_length(trigger::text) <= 1024),
    duration_minutes integer CHECK (duration_minutes IS NULL OR duration_minutes BETWEEN 1 AND 10080),
    -- The staff account that made the last change.
    changed_by text NOT NULL CHECK (char_length(changed_by) BETWEEN 1 AND 24),
    changed_by_id bigint NOT NULL CHECK (changed_by_id > 0),
    changed_at timestamptz NOT NULL DEFAULT now(),
    CHECK (enabled IS NOT NULL OR trigger IS NOT NULL OR duration_minutes IS NOT NULL)
);

CREATE TABLE mod_switches (
    mod text NOT NULL CHECK (char_length(mod) BETWEEN 1 AND 64 AND mod <> 'core'),
    -- The map's asset name, such as map/prontera, or '' for every zone.
    map text NOT NULL DEFAULT '' CHECK (char_length(map) <= 64),
    reason text NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 255),
    changed_by text NOT NULL CHECK (char_length(changed_by) BETWEEN 1 AND 24),
    changed_by_id bigint NOT NULL CHECK (changed_by_id > 0),
    changed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (mod, map)
);

-- Rights, when the roles exist (deploy/postgres/roles.sql): waserver
-- reads and writes both tables.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON event_overrides, mod_switches TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE mod_switches;
DROP TABLE event_overrides;
