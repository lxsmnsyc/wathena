-- Queries of the staff's moderation on the game role: sanctions, bans
-- and the report queue (decision 0030).

-- name: AddCharacterSanction :one
-- AddCharacterSanction records a staff's mute, jail or kick of a
-- character. A mute or a jail that the character's zone saved already
-- is the same row, by its start, so the row is merged.
INSERT INTO sanctions (character_id, account_id, kind, reason, started_at, ends_at, issued_by, return_map, return_x, return_y)
SELECT @character_id::bigint, c.account_id, @kind, @reason, @started_at, @ends_at, @issued_by, @return_map, @return_x, @return_y
FROM characters c WHERE c.id = @character_id::bigint
ON CONFLICT (character_id, kind, started_at) DO UPDATE SET
    reason = EXCLUDED.reason, ends_at = EXCLUDED.ends_at, issued_by = EXCLUDED.issued_by
RETURNING id;

-- name: AddBan :one
-- AddBan bans an account. ends_at is 'infinity' for a permanent ban.
INSERT INTO sanctions (account_id, kind, reason, started_at, ends_at, issued_by)
VALUES (@account_id, 'ban', @reason, @started_at, @ends_at, @issued_by)
RETURNING id;

-- name: LiftSanctionByID :execrows
-- LiftSanctionByID ends a mute, a jail or a ban before its time.
UPDATE sanctions SET lifted_at = @at, lifted_by = @lifted_by, lift_reason = @lift_reason
WHERE id = @id AND kind <> 'kick' AND lifted_at IS NULL AND ends_at > @at;

-- name: SanctionByID :one
SELECT s.id, s.character_id, s.account_id, s.kind, s.reason, s.started_at, s.ends_at, s.issued_by,
    s.lifted_at, s.lifted_by, s.lift_reason, coalesce(c.name, '')::text AS character_name, a.name AS account_name
FROM sanctions s
JOIN accounts a ON a.id = s.account_id
LEFT JOIN characters c ON c.id = s.character_id
WHERE s.id = @id;

-- name: SearchSanctions :many
-- SearchSanctions lists sanctions newest first. character finds the
-- character's sanctions and its account's bans. account finds every
-- sanction of the account. active keeps the ones in effect at now.
SELECT s.id, s.character_id, s.account_id, s.kind, s.reason, s.started_at, s.ends_at, s.issued_by,
    s.lifted_at, s.lifted_by, s.lift_reason, coalesce(c.name, '')::text AS character_name, a.name AS account_name
FROM sanctions s
JOIN accounts a ON a.id = s.account_id
LEFT JOIN characters c ON c.id = s.character_id
WHERE (sqlc.narg('character')::bigint IS NULL OR s.character_id = sqlc.narg('character')
        OR s.kind = 'ban' AND s.account_id = (SELECT account_id FROM characters WHERE id = sqlc.narg('character')))
    AND (sqlc.narg('account')::bigint IS NULL OR s.account_id = sqlc.narg('account'))
    AND (sqlc.narg('kind')::text IS NULL OR s.kind = sqlc.narg('kind'))
    AND (NOT @active::boolean OR s.lifted_at IS NULL AND s.ends_at > @now)
    AND (sqlc.narg('before_id')::bigint IS NULL OR s.id < sqlc.narg('before_id'))
ORDER BY s.id DESC
LIMIT @max_rows;

-- name: ActiveBanOfAccount :one
-- ActiveBanOfAccount is the account's ban in effect that ends last.
SELECT id, reason, started_at, ends_at, issued_by
FROM sanctions
WHERE account_id = @account_id AND kind = 'ban' AND lifted_at IS NULL AND ends_at > now()
ORDER BY ends_at DESC, id DESC
LIMIT 1;

-- name: ActiveBanOfCharacter :one
-- ActiveBanOfCharacter is the ban in effect of a character's account,
-- which a game login checks.
SELECT s.id, s.reason, s.ends_at
FROM sanctions s JOIN characters c ON c.account_id = s.account_id
WHERE c.id = @character_id AND s.kind = 'ban' AND s.lifted_at IS NULL AND s.ends_at > now()
ORDER BY s.ends_at DESC, s.id DESC
LIMIT 1;

-- name: ActiveBansOfAccounts :many
-- ActiveBansOfAccounts are the bans in effect of some accounts, for the
-- player search.
SELECT DISTINCT ON (s.account_id) s.id, s.account_id, s.reason, s.started_at, s.ends_at, s.issued_by
FROM sanctions s
WHERE s.account_id = ANY(@account_ids::bigint[]) AND s.kind = 'ban' AND s.lifted_at IS NULL AND s.ends_at > now()
ORDER BY s.account_id, s.ends_at DESC, s.id DESC;

-- name: CharacterPlace :one
-- CharacterPlace is a character's name, account and saved position.
SELECT c.id, c.name, c.account_id, c.map, c.x, c.y, a.name AS account_name
FROM characters c JOIN accounts a ON a.id = c.account_id
WHERE c.id = @id;

-- name: AccountName :one
SELECT name FROM accounts WHERE id = @id;

-- name: SearchReports :many
-- SearchReports lists reports newest first, with filters.
SELECT r.id, r.created_at, r.reporter_id, r.reporter_name, r.target_id, r.target_name, r.line_id, r.reason, r.map,
    r.status, r.closed_at, coalesce(r.outcome, '')::text AS outcome, r.note, r.closed_by, r.sanction_id,
    r.guild_id, r.emblem_version, coalesce(c.account_id, 0)::bigint AS target_account_id
FROM reports r
LEFT JOIN characters c ON c.id = r.target_id AND r.target_id > 0
WHERE (sqlc.narg('status')::text IS NULL OR r.status = sqlc.narg('status'))
    AND (sqlc.narg('target')::text IS NULL OR lower(r.target_name) = lower(sqlc.narg('target')))
    AND (sqlc.narg('kind')::text IS NULL
        OR sqlc.narg('kind') = 'player' AND r.guild_id = 0 AND r.line_id IS NULL
        OR sqlc.narg('kind') = 'line' AND r.guild_id = 0 AND r.line_id IS NOT NULL
        OR sqlc.narg('kind') = 'guild_name' AND r.guild_id <> 0 AND r.emblem_version = 0
        OR sqlc.narg('kind') = 'guild_emblem' AND r.guild_id <> 0 AND r.emblem_version <> 0)
    AND (sqlc.narg('before_id')::bigint IS NULL OR r.id < sqlc.narg('before_id'))
ORDER BY r.id DESC
LIMIT @max_rows;

-- name: ReportByID :one
SELECT r.id, r.created_at, r.reporter_id, r.reporter_name, r.target_id, r.target_name, r.line_id, r.reason, r.map,
    r.status, r.closed_at, coalesce(r.outcome, '')::text AS outcome, r.note, r.closed_by, r.sanction_id,
    r.guild_id, r.emblem_version, coalesce(c.account_id, 0)::bigint AS target_account_id, r.emblem_png
FROM reports r
LEFT JOIN characters c ON c.id = r.target_id AND r.target_id > 0
WHERE r.id = @id;

-- name: ChatAround :many
-- ChatAround are a character's own lines in a time window, for the
-- chat around a report.
SELECT id, at, kind, character_id, name, target, map, text
FROM chat_log
WHERE character_id = @character_id AND at BETWEEN @since AND @until
ORDER BY at, id
LIMIT @max_rows;

-- name: CloseReportWithOutcome :execrows
UPDATE reports SET status = 'closed', closed_at = @at, outcome = @outcome::text, note = @note, closed_by = @closed_by,
    sanction_id = sqlc.narg('sanction_id')
WHERE id = @id AND status = 'open';
