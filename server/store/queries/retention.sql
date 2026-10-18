-- Queries of the retention job (decision 0049).

-- name: TryRetentionLock :one
-- TryRetentionLock takes the retention job's advisory lock for the
-- session, or returns false when another session holds it. Bit 61 set
-- marks the server's jobs, as bit 62 marks the account storages of
-- LockAccountStorages, and 1 is the retention job.
SELECT pg_try_advisory_lock((1::bigint << 61) | 1)::boolean AS locked;

-- name: RetentionUnlock :one
-- RetentionUnlock lets the retention job's lock go.
SELECT pg_advisory_unlock((1::bigint << 61) | 1)::boolean AS unlocked;

-- name: DropOldLogPartitions :many
-- DropOldLogPartitions drops the month partitions of the item and zeny
-- logs that end at or before cutoff, and deletes the older rows of the
-- default partitions, with drop_old_log_partitions.
SELECT kind::text AS kind, partitions::integer AS partitions, deleted::bigint AS deleted
FROM drop_old_log_partitions(@cutoff::timestamptz);

-- name: DeleteOldStaffAudit :one
-- DeleteOldStaffAudit deletes the audit rows from before cutoff with
-- their reviews, except rows that wait for a review, with
-- delete_old_staff_audit.
SELECT delete_old_staff_audit(@cutoff::timestamptz)::bigint AS deleted;

-- name: DeleteOldLogins :execrows
-- DeleteOldLogins deletes up to 10,000 login records from before a
-- time.
DELETE FROM account_logins
WHERE id IN (SELECT l.id FROM account_logins l WHERE l.at < @before LIMIT 10000);
