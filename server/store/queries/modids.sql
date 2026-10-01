-- Queries for the IDs of mod records (strategy section 6.5). waserver
-- runs them as wathena_game when it loads mods.

-- name: ModIDs :many
-- ModIDs returns every ID given so far.
SELECT kind, name, id, mod_id FROM mod_ids ORDER BY kind, id;

-- name: AssignModID :one
-- AssignModID gives a name the next ID of its kind: one more than the
-- highest given, and at least the start of the mod range. It returns no
-- row when the name has an ID already. Two servers that assign at once
-- can pick the same ID: one insert then fails on the unique ID, and the
-- caller tries again.
INSERT INTO mod_ids (kind, name, id, mod_id)
SELECT @kind::text, @name::text, GREATEST(COALESCE(MAX(m.id) + 1, 0), @start::bigint), @mod_id::text
FROM mod_ids m
WHERE m.kind = @kind::text
ON CONFLICT (kind, name) DO NOTHING
RETURNING id;

-- name: ModID :one
-- ModID returns the ID a name has.
SELECT id FROM mod_ids WHERE kind = @kind AND name = @name;
