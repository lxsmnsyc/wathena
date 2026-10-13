-- Queries of the world, events and mods tools on the game role
-- (decision 0031).

-- name: EventOverrides :many
SELECT event, enabled, trigger, duration_minutes, changed_by, changed_by_id, changed_at
FROM event_overrides
ORDER BY event;

-- name: SetEventOverride :exec
-- SetEventOverride saves a staff change of an event, over the config.
INSERT INTO event_overrides (event, enabled, trigger, duration_minutes, changed_by, changed_by_id, changed_at)
VALUES (@event, @enabled, @trigger, @duration_minutes, @changed_by, @changed_by_id, now())
ON CONFLICT (event) DO UPDATE SET
    enabled = EXCLUDED.enabled, trigger = EXCLUDED.trigger, duration_minutes = EXCLUDED.duration_minutes,
    changed_by = EXCLUDED.changed_by, changed_by_id = EXCLUDED.changed_by_id, changed_at = EXCLUDED.changed_at;

-- name: DeleteEventOverride :execrows
DELETE FROM event_overrides WHERE event = @event;

-- name: ModSwitches :many
SELECT mod, map, reason, changed_by, changed_by_id, changed_at
FROM mod_switches
ORDER BY mod, map;

-- name: AddModSwitch :exec
-- AddModSwitch turns a mod off in a map's zone, or in every zone for
-- map ''.
INSERT INTO mod_switches (mod, map, reason, changed_by, changed_by_id, changed_at)
VALUES (@mod, @map, @reason, @changed_by, @changed_by_id, now())
ON CONFLICT (mod, map) DO UPDATE SET
    reason = EXCLUDED.reason, changed_by = EXCLUDED.changed_by, changed_by_id = EXCLUDED.changed_by_id,
    changed_at = EXCLUDED.changed_at;

-- name: DeleteModSwitch :execrows
DELETE FROM mod_switches WHERE mod = @mod AND map = @map;

-- name: DeleteModSwitches :execrows
-- DeleteModSwitches turns a mod on in every zone.
DELETE FROM mod_switches WHERE mod = @mod;
