-- Retention jobs (phase 8 step 3, proposal 0002 review decision 49,
-- decision 0049, strategy section 7.10).
--
-- Rules:
--   - drop_old_log_partitions drops the month partitions of item_log
--     and zeny_log whose whole month ends at or before cutoff, and
--     deletes the rows of their default partitions from before cutoff.
--     It returns, per log, the partitions it dropped and the rows that
--     went. A month partition is one that ensure_log_partitions made:
--     its name is the log's name and _YYYY_MM. Other tables are left.
--   - delete_old_staff_audit deletes the rows of the GM audit log from
--     before cutoff, with their reviews. A row that needs review and has
--     no review yet stays until it is reviewed. It returns how many rows
--     went.
--   - Both run with the rights of the schema's owner, as
--     ensure_log_partitions does. So the game role drops partitions
--     without the right to drop tables, and the account role deletes
--     audit rows without the right to delete them. Both refuse a cutoff
--     later than a day ago, the shortest period of the retention
--     config.
--   - The game role runs drop_old_log_partitions. The account role runs
--     delete_old_staff_audit. Neither gets new table rights.

-- +goose Up

-- +goose StatementBegin
CREATE FUNCTION drop_old_log_partitions(cutoff timestamptz)
RETURNS TABLE (kind text, partitions integer, deleted bigint)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
-- A drop waits for the commits that write the log. It gives up after
-- this long, and the next run tries again.
SET lock_timeout = '10s'
AS $$
DECLARE
    parent text;
    part text;
    -- month_end is the end of a partition's month, as a UTC time.
    month_end timestamptz;
    n bigint;
BEGIN
    IF cutoff > now() - interval '1 day' THEN
        RAISE EXCEPTION 'the cutoff % is less than a day ago', cutoff;
    END IF;
    FOREACH parent IN ARRAY ARRAY['item_log', 'zeny_log'] LOOP
        kind := parent;
        partitions := 0;
        deleted := 0;
        FOR part IN
            SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
            WHERE i.inhparent = ('public.' || parent)::regclass
                AND c.relname ~ ('^' || parent || '_[0-9]{4}_[0-9]{2}$')
            ORDER BY c.relname
        LOOP
            month_end := (to_date(right(part, 7), 'YYYY_MM') + interval '1 month') AT TIME ZONE 'UTC';
            IF month_end <= cutoff THEN
                EXECUTE format('SELECT count(*) FROM public.%I', part) INTO n;
                EXECUTE format('DROP TABLE public.%I', part);
                partitions := partitions + 1;
                deleted := deleted + n;
            END IF;
        END LOOP;
        EXECUTE format('DELETE FROM public.%I WHERE at < $1', parent || '_default') USING cutoff;
        GET DIAGNOSTICS n = ROW_COUNT;
        deleted := deleted + n;
        RETURN NEXT;
    END LOOP;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION drop_old_log_partitions(timestamptz) FROM PUBLIC;

-- +goose StatementBegin
CREATE FUNCTION delete_old_staff_audit(cutoff timestamptz) RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    n bigint;
BEGIN
    IF cutoff > now() - interval '1 day' THEN
        RAISE EXCEPTION 'the cutoff % is less than a day ago', cutoff;
    END IF;
    -- The reviews go with their rows: staff_audit_reviews cascades.
    DELETE FROM staff_audit a
    WHERE a.at < cutoff
        AND (NOT a.needs_review OR EXISTS (SELECT 1 FROM staff_audit_reviews r WHERE r.audit_id = a.id));
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION delete_old_staff_audit(timestamptz) FROM PUBLIC;

-- Rights, when the roles exist (deploy/postgres/roles.sql): the game
-- role drops old log partitions, and the account role deletes old audit
-- rows.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT EXECUTE ON FUNCTION drop_old_log_partitions(timestamptz) TO wathena_game;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_account') THEN
        GRANT EXECUTE ON FUNCTION delete_old_staff_audit(timestamptz) TO wathena_account;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP FUNCTION delete_old_staff_audit(timestamptz);
DROP FUNCTION drop_old_log_partitions(timestamptz);
