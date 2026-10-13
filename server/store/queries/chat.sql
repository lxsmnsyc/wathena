-- Queries for block lists, sanctions, the chat log and reports
-- (decision 0018).

-- name: CharacterBlocks :many
SELECT name FROM character_blocks
WHERE character_id = @character_id
ORDER BY position;

-- name: DeleteCharacterBlocks :exec
DELETE FROM character_blocks WHERE character_id = @character_id;

-- name: InsertCharacterBlocks :exec
INSERT INTO character_blocks (character_id, position, name)
SELECT @character_id, unnest(@positions::smallint[]), unnest(@names::text[]);

-- name: ActiveSanctions :many
-- ActiveSanctions are a character's mutes and jails that have not
-- ended.
SELECT kind, reason, started_at, ends_at, issued_by, return_map, return_x, return_y
FROM sanctions
WHERE character_id = @character_id::bigint AND kind IN ('mute', 'jail') AND lifted_at IS NULL AND ends_at > now()
ORDER BY started_at, id;

-- name: UpsertSanction :exec
INSERT INTO sanctions (character_id, account_id, kind, reason, started_at, ends_at, issued_by, return_map, return_x, return_y)
SELECT @character_id::bigint, c.account_id, @kind, @reason, @started_at, @ends_at, @issued_by, @return_map, @return_x, @return_y
FROM characters c WHERE c.id = @character_id::bigint
ON CONFLICT (character_id, kind, started_at) DO UPDATE SET
    reason = EXCLUDED.reason, ends_at = EXCLUDED.ends_at, issued_by = EXCLUDED.issued_by,
    return_map = EXCLUDED.return_map, return_x = EXCLUDED.return_x, return_y = EXCLUDED.return_y;

-- name: LiftSanction :exec
-- LiftSanction records that a sanction ended before its time.
UPDATE sanctions SET lifted_at = @at
WHERE character_id = @character_id::bigint AND kind = @kind AND started_at = @started_at
    AND lifted_at IS NULL AND ends_at > @at;

-- name: InsertChatLines :exec
INSERT INTO chat_log (id, at, kind, character_id, name, target, map, text)
SELECT unnest(@ids::uuid[]), unnest(@ats::timestamptz[]), unnest(@kinds::text[]),
    unnest(@character_ids::bigint[]), unnest(@names::text[]), unnest(@targets::text[]),
    unnest(@maps::text[]), unnest(@texts::text[])
ON CONFLICT (id) DO NOTHING;

-- name: InsertReport :one
INSERT INTO reports (created_at, reporter_id, reporter_name, target_id, target_name, line_id, reason, map,
    guild_id, emblem_version, emblem_png)
VALUES (@created_at, @reporter_id, @reporter_name, @target_id, @target_name, sqlc.narg('line_id'), @reason, @map,
    @guild_id, @emblem_version, sqlc.narg('emblem_png'))
RETURNING id;

-- name: InsertReportLines :exec
-- InsertReportLines ties a report to the logged lines of its context.
-- A line the log dropped is left out.
INSERT INTO report_lines (report_id, line_id)
SELECT @report_id, id FROM chat_log WHERE id = ANY(@line_ids::uuid[])
ON CONFLICT DO NOTHING;

-- name: DeleteOldChatLines :execrows
-- DeleteOldChatLines deletes up to 10,000 lines from before a time,
-- except the lines of open reports.
DELETE FROM chat_log
WHERE id IN (
    SELECT c.id FROM chat_log c
    WHERE c.at < @before
        AND NOT EXISTS (
            SELECT 1 FROM report_lines rl JOIN reports r ON r.id = rl.report_id
            WHERE rl.line_id = c.id AND r.status = 'open'
        )
    LIMIT 10000
);

-- name: Reports :many
-- Reports lists the newest reports, for the staff tools and tests.
SELECT id, created_at, reporter_id, reporter_name, target_id, target_name, line_id, reason, map, status, guild_id, emblem_version
FROM reports
ORDER BY id DESC
LIMIT @max_rows;

-- name: ReportLines :many
-- ReportLines are the chat lines of a report, oldest first.
SELECT c.id, c.at, c.kind, c.character_id, c.name, c.target, c.map, c.text
FROM report_lines rl JOIN chat_log c ON c.id = rl.line_id
WHERE rl.report_id = @report_id
ORDER BY c.at, c.id;

-- name: CloseReport :execrows
UPDATE reports SET status = 'closed', closed_at = @at
WHERE id = @id AND status = 'open';
