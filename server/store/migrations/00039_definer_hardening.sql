-- SECURITY DEFINER hardening (docs/security/review-2026-10.md, L15 and
-- L16).
--
-- Rules:
--   - Every SECURITY DEFINER function searches pg_catalog, then public,
--     then pg_temp. PostgreSQL searches pg_temp first unless the path
--     names it, so a role with TEMP could shadow a public table with a
--     temporary one. Listed last, pg_temp never shadows a public table.
--   - ensure_log_partitions refuses a day more than 45 days before or
--     after the database clock. waserver passes the present only, and
--     the function makes that month and the next. Before, a caller
--     could make the partitions of any month, and so make thousands of
--     them.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ensure_log_partitions(day timestamptz) RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public, pg_temp
AS $$
DECLARE
    -- m is the first moment of a month, as a UTC time without a zone.
    m timestamp;
    parent text;
    part text;
BEGIN
    IF day IS NULL OR day < now() - interval '45 days' OR day > now() + interval '45 days' THEN
        RAISE EXCEPTION 'the day % is more than 45 days from now', day;
    END IF;
    FOR i IN 0..1 LOOP
        m := date_trunc('month', day AT TIME ZONE 'UTC') + make_interval(months => i);
        FOREACH parent IN ARRAY ARRAY['item_log', 'zeny_log'] LOOP
            part := parent || '_' || to_char(m, 'YYYY_MM');
            IF to_regclass('public.' || part) IS NULL THEN
                BEGIN
                    EXECUTE format('CREATE TABLE public.%I PARTITION OF public.%I FOR VALUES FROM (%L) TO (%L)',
                        part, parent, m AT TIME ZONE 'UTC', (m + interval '1 month') AT TIME ZONE 'UTC');
                EXCEPTION WHEN check_violation THEN
                    RAISE WARNING 'the default partition of % holds rows of %; the month stays there', parent, part;
                END;
            END IF;
        END LOOP;
    END LOOP;
END
$$;
-- +goose StatementEnd

ALTER FUNCTION delete_account(bigint, timestamptz, integer[]) SET search_path = pg_catalog, public, pg_temp;
ALTER FUNCTION request_account_deletion(bigint, interval) SET search_path = pg_catalog, public, pg_temp;
ALTER FUNCTION delete_old_staff_audit(timestamptz) SET search_path = pg_catalog, public, pg_temp;
ALTER FUNCTION check_staff_audit_review() SET search_path = pg_catalog, public, pg_temp;
ALTER FUNCTION drop_old_log_partitions(timestamptz) SET search_path = pg_catalog, public, pg_temp;

-- +goose Down

ALTER FUNCTION drop_old_log_partitions(timestamptz) SET search_path = pg_catalog, public;
ALTER FUNCTION check_staff_audit_review() SET search_path = pg_catalog, public;
ALTER FUNCTION delete_old_staff_audit(timestamptz) SET search_path = pg_catalog, public;
ALTER FUNCTION request_account_deletion(bigint, interval) SET search_path = pg_catalog, public;
ALTER FUNCTION delete_account(bigint, timestamptz, integer[]) SET search_path = pg_catalog, public;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ensure_log_partitions(day timestamptz) RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    m timestamp;
    parent text;
    part text;
BEGIN
    FOR i IN 0..1 LOOP
        m := date_trunc('month', day AT TIME ZONE 'UTC') + make_interval(months => i);
        FOREACH parent IN ARRAY ARRAY['item_log', 'zeny_log'] LOOP
            part := parent || '_' || to_char(m, 'YYYY_MM');
            IF to_regclass('public.' || part) IS NULL THEN
                BEGIN
                    EXECUTE format('CREATE TABLE public.%I PARTITION OF public.%I FOR VALUES FROM (%L) TO (%L)',
                        part, parent, m AT TIME ZONE 'UTC', (m + interval '1 month') AT TIME ZONE 'UTC');
                EXCEPTION WHEN check_violation THEN
                    RAISE WARNING 'the default partition of % holds rows of %; the month stays there', parent, part;
                END;
            END IF;
        END LOOP;
    END LOOP;
END
$$;
-- +goose StatementEnd
