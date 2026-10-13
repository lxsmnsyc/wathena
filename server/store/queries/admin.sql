-- Queries of the admin API's player search and audit reviews (decision
-- 0029). The admin API runs them as the wathena_account role, which
-- reads accounts, characters and login records.

-- name: SearchAccountsByName :many
-- SearchAccountsByName finds accounts whose name, or the name of one of
-- whose characters, starts with a prefix, in any case. prefix is lower
-- case, with LIKE's wildcards escaped, and ends with %. Pages go by
-- account ID.
SELECT a.id, a.name, a.created_at FROM accounts a
WHERE a.id IN (
    SELECT x.id FROM accounts x WHERE lower(x.name) LIKE @prefix::text
    UNION
    SELECT c.account_id FROM characters c WHERE lower(c.name) LIKE @prefix::text
) AND a.id > @after_id
ORDER BY a.id
LIMIT @max_rows;

-- name: SearchAccountsByID :many
-- SearchAccountsByID finds the account of an ID and the account of the
-- character of that ID.
SELECT a.id, a.name, a.created_at FROM accounts a
WHERE (a.id = @id OR a.id IN (SELECT c.account_id FROM characters c WHERE c.id = @id)) AND a.id > @after_id
ORDER BY a.id
LIMIT @max_rows;

-- name: AccountsByIDs :many
SELECT id, name, created_at FROM accounts WHERE id = ANY(@ids::bigint[]) ORDER BY id;

-- name: SearchAccountsByIP :many
-- SearchAccountsByIP finds the accounts with a login from a CIDR range
-- since a time. Pages go by account ID.
SELECT DISTINCT l.account_id FROM account_logins l
WHERE l.ip <<= @cidr::cidr AND l.at >= @since AND l.account_id > @after_id
ORDER BY l.account_id
LIMIT @max_rows;

-- name: LoginsByIP :many
-- LoginsByIP lists the logins of some accounts from a CIDR range since a
-- time, newest first, at most 20 per account.
SELECT account_id, at, ip, user_agent FROM (
    SELECT l.account_id, l.at, l.ip, l.user_agent,
        row_number() OVER (PARTITION BY l.account_id ORDER BY l.at DESC, l.id DESC) AS n
    FROM account_logins l
    WHERE l.account_id = ANY(@ids::bigint[]) AND l.ip <<= @cidr::cidr AND l.at >= @since
) x WHERE x.n <= 20
ORDER BY account_id, at DESC;

-- name: LastLogins :many
-- LastLogins is the last login of each of some accounts.
SELECT account_id, max(at)::timestamptz AS at FROM account_logins
WHERE account_id = ANY(@ids::bigint[])
GROUP BY account_id;

-- name: CharactersOfAccounts :many
-- CharactersOfAccounts lists the characters of some accounts, with where
-- each was saved.
SELECT id, account_id, slot, name, job, base_level, job_level, map, x, y
FROM characters
WHERE account_id = ANY(@ids::bigint[])
ORDER BY account_id, slot;

-- name: CharacterWithAccount :one
-- CharacterWithAccount finds a character and its account's name.
SELECT c.id, c.name, c.account_id, a.name AS account_name
FROM characters c JOIN accounts a ON a.id = c.account_id
WHERE c.id = @id;

-- name: AuditRowForReview :one
-- AuditRowForReview is what a review needs to know of an audit row, and
-- whether it has a review.
SELECT a.id, a.account_id, a.needs_review, (r.audit_id IS NOT NULL)::boolean AS reviewed
FROM staff_audit a LEFT JOIN staff_audit_reviews r ON r.audit_id = a.id
WHERE a.id = @id;

-- name: AddStaffAuditReview :execrows
-- AddStaffAuditReview records the review of an audit row. A row has at
-- most one review, so a second one adds nothing.
INSERT INTO staff_audit_reviews (audit_id, reviewer_id, reviewer_name, verdict, note)
VALUES (@audit_id, @reviewer_id, @reviewer_name, @verdict, @note)
ON CONFLICT (audit_id) DO NOTHING;

-- name: StaffAuditEntry :one
-- StaffAuditEntry is one audit row with its review.
SELECT a.id, a.at, a.account_id, a.account_name, a.role, a.action, a.source, a.target,
    a.arguments, a.reason, a.needs_review, r.reviewed_at, coalesce(r.reviewer_name, '')::text AS reviewer_name,
    coalesce(r.verdict, '')::text AS verdict, coalesce(r.note, '')::text AS review_note
FROM staff_audit a
LEFT JOIN staff_audit_reviews r ON r.audit_id = a.id
WHERE a.id = @id;
