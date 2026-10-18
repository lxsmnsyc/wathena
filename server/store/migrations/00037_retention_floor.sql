-- Retention follow-ups (decision 0049, "Later changes").
--
-- Rules:
--   - drop_old_log_partitions and delete_old_staff_audit refuse a
--     cutoff later than 30 days ago. 00036 refused one later than a day
--     ago. The config's minimum of retention.item_log_days and
--     retention.audit_days is 30 days too. 30 days is 720 hours, as the
--     server counts a day as 24 hours.
--   - retention_status holds what the last retention run did, in one
--     row: when it started, when the last run without errors started,
--     the rows it deleted by kind, the month partitions it dropped, and
--     its errors. The job writes it as the game role, which holds the
--     job's lock, and reads it for GET /admin/api/v1/retention. A run
--     that finds the lock taken writes nothing.
--   - The game role reads, inserts and updates retention_status. No
--     other role gets rights on it.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION drop_old_log_partitions(cutoff timestamptz)
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
    IF cutoff > now() - interval '720 hours' THEN
        RAISE EXCEPTION 'the cutoff % is less than 30 days ago', cutoff;
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

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_old_staff_audit(cutoff timestamptz) RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    n bigint;
BEGIN
    IF cutoff > now() - interval '720 hours' THEN
        RAISE EXCEPTION 'the cutoff % is less than 30 days ago', cutoff;
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

CREATE TABLE retention_status (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    -- last_run is when the last run started. last_success is when the
    -- last run without errors started, or NULL before one.
    last_run timestamptz NOT NULL,
    last_success timestamptz,
    -- The rows the last run deleted, by kind.
    login_ips bigint NOT NULL DEFAULT 0 CHECK (login_ips >= 0),
    chat bigint NOT NULL DEFAULT 0 CHECK (chat >= 0),
    item_log bigint NOT NULL DEFAULT 0 CHECK (item_log >= 0),
    zeny_log bigint NOT NULL DEFAULT 0 CHECK (zeny_log >= 0),
    audit bigint NOT NULL DEFAULT 0 CHECK (audit >= 0),
    partitions integer NOT NULL DEFAULT 0 CHECK (partitions >= 0),
    -- last_error is the last run's errors, or '' without one.
    last_error text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 4000)
);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT, UPDATE ON retention_status TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE retention_status;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_old_staff_audit(cutoff timestamptz) RETURNS bigint
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

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION drop_old_log_partitions(cutoff timestamptz)
RETURNS TABLE (kind text, partitions integer, deleted bigint)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
SET lock_timeout = '10s'
AS $$
DECLARE
    parent text;
    part text;
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
