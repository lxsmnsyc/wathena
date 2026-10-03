-- Item and zeny logs of transfers (phase 5 step 1, decision 0017,
-- strategy sections 3.5 and 7.10).
--
-- Rules:
--   - One row per move. A transfer writes its rows in the same
--     transaction as the characters it changed.
--   - from_kind and to_kind name an owner: a holder (inventory, storage,
--     cart, guild_storage, mail, vending) or an end (ground, monster,
--     shop, script, skill, used). from_id and to_id are its ID, as
--     server/transfer documents, or 0.
--   - serial is the stack the amount ended in, or the stack it left for
--     an end. source is the serial it came from when that differs: the
--     stack of a split or a merge, or the stack a ground item was
--     dropped from.
--   - Rows have no foreign keys, so they outlive the characters they
--     name.
--   - Both tables are partitioned by month on at. ensure_log_partitions
--     makes the partitions of a month and the next one. waserver calls
--     it at start and once a day. Rows outside every month partition go
--     to the default partition. Old months are dropped whole, so the
--     180 days of section 7.10 need no slow deletes.

-- +goose Up

CREATE TABLE item_log (
    id bigint GENERATED ALWAYS AS IDENTITY,
    at timestamptz NOT NULL,
    action text NOT NULL CHECK (char_length(action) BETWEEN 1 AND 32),
    from_kind text NOT NULL CHECK (char_length(from_kind) BETWEEN 1 AND 32),
    from_id bigint NOT NULL,
    to_kind text NOT NULL CHECK (char_length(to_kind) BETWEEN 1 AND 32),
    to_id bigint NOT NULL,
    serial uuid NOT NULL,
    source uuid,
    item_id integer NOT NULL CHECK (item_id > 0),
    amount integer NOT NULL CHECK (amount BETWEEN 1 AND 30000),
    refine smallint NOT NULL DEFAULT 0 CHECK (refine >= 0),
    cards integer[] NOT NULL DEFAULT '{}',
    rolls jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(rolls) = 'object'),
    card_rolls jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(card_rolls) = 'array'),
    random_options jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(random_options) = 'array'),
    identified boolean NOT NULL DEFAULT true,
    PRIMARY KEY (at, id)
) PARTITION BY RANGE (at);

CREATE TABLE item_log_default PARTITION OF item_log DEFAULT;

-- Finding where a serial went, and where it came from.
CREATE INDEX item_log_serial ON item_log (serial, at);
CREATE INDEX item_log_source ON item_log (source, at) WHERE source IS NOT NULL;
CREATE INDEX item_log_from ON item_log (from_kind, from_id, at);
CREATE INDEX item_log_to ON item_log (to_kind, to_id, at);

CREATE TABLE zeny_log (
    id bigint GENERATED ALWAYS AS IDENTITY,
    at timestamptz NOT NULL,
    action text NOT NULL CHECK (char_length(action) BETWEEN 1 AND 32),
    from_kind text NOT NULL CHECK (char_length(from_kind) BETWEEN 1 AND 32),
    from_id bigint NOT NULL,
    to_kind text NOT NULL CHECK (char_length(to_kind) BETWEEN 1 AND 32),
    to_id bigint NOT NULL,
    -- MAX_ZENY.
    amount bigint NOT NULL CHECK (amount BETWEEN 1 AND 2147483647),
    PRIMARY KEY (at, id)
) PARTITION BY RANGE (at);

CREATE TABLE zeny_log_default PARTITION OF zeny_log DEFAULT;

CREATE INDEX zeny_log_from ON zeny_log (from_kind, from_id, at);
CREATE INDEX zeny_log_to ON zeny_log (to_kind, to_id, at);

-- ensure_log_partitions makes the month partitions of both logs for the
-- month of day and the month after, when they do not exist. It runs with
-- the rights of the schema's owner, so the game role can call it
-- without the right to change the schema. A month whose rows already
-- went to the default partition is left there: the function then
-- raises a warning and goes on.
-- +goose StatementBegin
CREATE FUNCTION ensure_log_partitions(day timestamptz) RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    -- m is the first moment of a month, as a UTC time without a zone.
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

REVOKE ALL ON FUNCTION ensure_log_partitions(timestamptz) FROM PUBLIC;

-- Rights, when the roles exist (deploy/postgres/roles.sql): waserver
-- writes and reads the logs, and makes their partitions.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, INSERT ON item_log, zeny_log TO wathena_game;
        GRANT EXECUTE ON FUNCTION ensure_log_partitions(timestamptz) TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

SELECT ensure_log_partitions(now());

-- +goose Down

DROP FUNCTION ensure_log_partitions(timestamptz);
DROP TABLE zeny_log;
DROP TABLE item_log;
