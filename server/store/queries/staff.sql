-- Queries for staff roles and the GM audit log (decision 0028). The
-- admin API runs them as the wathena_account role. AllStaffRoles and
-- WriteStaffAudit run as wathena_game.

-- name: AllStaffRoles :many
-- AllStaffRoles lists every role given from the admin screens, for the
-- world's staff registry.
SELECT account_id, role FROM staff_roles ORDER BY account_id;

-- name: StaffAccountByName :one
-- StaffAccountByName finds an account by name, in any case. Dev
-- accounts are left out: they have no password and no second factor.
SELECT id, name FROM accounts WHERE lower(name) = lower(@name) AND NOT dev;

-- name: StaffAccountsByNames :many
-- StaffAccountsByNames finds the accounts of a list of lower-case names,
-- such as the config's owners.
SELECT id, name FROM accounts WHERE lower(name) = ANY(@names::text[]) AND NOT dev;

-- name: StaffRoleOf :one
SELECT role FROM staff_roles WHERE account_id = @account_id;

-- name: TwoFactorOn :one
-- TwoFactorOn reports whether an account has TOTP or a passkey on.
SELECT (EXISTS (SELECT FROM account_totp t WHERE t.account_id = @account_id AND t.enabled_at IS NOT NULL)
    OR EXISTS (SELECT FROM account_passkeys k WHERE k.account_id = @account_id))::boolean AS two_factor;

-- name: StaffMembers :many
-- StaffMembers lists the given roles with their accounts, who gave
-- them, and whether each account has TOTP or a passkey on.
SELECT s.account_id, a.name, s.role, s.given_at, coalesce(g.name, '')::text AS given_by,
    (EXISTS (SELECT FROM account_totp t WHERE t.account_id = s.account_id AND t.enabled_at IS NOT NULL)
        OR EXISTS (SELECT FROM account_passkeys k WHERE k.account_id = s.account_id))::boolean AS two_factor
FROM staff_roles s
JOIN accounts a ON a.id = s.account_id
LEFT JOIN accounts g ON g.id = s.given_by
ORDER BY lower(a.name);

-- name: AccountsTwoFactor :many
-- AccountsTwoFactor reports, for a list of accounts, whether each has
-- TOTP or a passkey on.
SELECT a.id, (EXISTS (SELECT FROM account_totp t WHERE t.account_id = a.id AND t.enabled_at IS NOT NULL)
        OR EXISTS (SELECT FROM account_passkeys k WHERE k.account_id = a.id))::boolean AS two_factor
FROM accounts a WHERE a.id = ANY(@ids::bigint[]);

-- name: GiveStaffRole :exec
-- GiveStaffRole gives an account a role, in place of the one it had.
INSERT INTO staff_roles (account_id, role, given_by)
VALUES (@account_id, @role, @given_by)
ON CONFLICT (account_id) DO UPDATE SET
    role = EXCLUDED.role, given_by = EXCLUDED.given_by, given_at = now();

-- name: RemoveStaffRole :execrows
DELETE FROM staff_roles WHERE account_id = @account_id;

-- name: AddStaffAudit :one
-- AddStaffAudit writes a row of the GM audit log and returns its ID.
INSERT INTO staff_audit (account_id, account_name, role, action, source, target, target_account_id,
    arguments, reason, needs_review)
VALUES (@account_id, @account_name, @role, @action, @source, @target, sqlc.narg('target_account_id'),
    @arguments, @reason, @needs_review)
RETURNING id;

-- name: WriteStaffAudit :exec
-- WriteStaffAudit writes a row of the GM audit log, for a role that may
-- add rows but not read them, such as wathena_game.
INSERT INTO staff_audit (account_id, account_name, role, action, source, target, target_account_id,
    arguments, reason, needs_review)
VALUES (@account_id, @account_name, @role, @action, @source, @target, sqlc.narg('target_account_id'),
    @arguments, @reason, @needs_review);

-- name: SearchStaffAudit :many
-- SearchStaffAudit lists audit rows, newest first, before a row ID.
-- Each filter applies when it is not NULL. review is pending for rows
-- that wait for a review, reviewed for rows that have one.
SELECT a.id, a.at, a.account_id, a.account_name, a.role, a.action, a.source, a.target,
    a.arguments, a.reason, a.needs_review, r.reviewed_at, coalesce(r.reviewer_name, '')::text AS reviewer_name
FROM staff_audit a
LEFT JOIN staff_audit_reviews r ON r.audit_id = a.id
WHERE (sqlc.narg('before_id')::bigint IS NULL OR a.id < sqlc.narg('before_id')::bigint)
    AND (sqlc.narg('actor')::text IS NULL OR lower(a.account_name) = lower(sqlc.narg('actor')::text))
    AND (sqlc.narg('action')::text IS NULL OR a.action = sqlc.narg('action')::text)
    AND (sqlc.narg('target')::text IS NULL OR lower(a.target) = lower(sqlc.narg('target')::text))
    AND (sqlc.narg('since')::timestamptz IS NULL OR a.at >= sqlc.narg('since')::timestamptz)
    AND (sqlc.narg('until')::timestamptz IS NULL OR a.at < sqlc.narg('until')::timestamptz)
    AND (sqlc.narg('review')::text IS NULL
        OR sqlc.narg('review')::text = 'pending' AND a.needs_review AND r.audit_id IS NULL
        OR sqlc.narg('review')::text = 'reviewed' AND r.audit_id IS NOT NULL)
ORDER BY a.id DESC
LIMIT @max_rows;
