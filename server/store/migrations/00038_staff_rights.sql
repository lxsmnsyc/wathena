-- Staff, accounts and database rights (phase 8 step 5, review decision
-- 51, docs/security/review-2026-10.md).
--
-- Rules:
--   - game_tickets.recent records that the ticket's login session proved
--     who its player is within the reauth window when the ticket was
--     made. GM commands that make items or sanction players need it, as
--     the admin API needs a recent login.
--   - The account role no longer inserts deletion requests itself. It
--     calls request_account_deletion, which sets both times from the
--     database clock and refuses a wait shorter than 14 days.
--   - delete_account deletes only a request that is due on the database
--     clock, whose wait was at least 14 days, whatever time the caller
--     gives.
--   - delete_old_staff_audit runs as the game role, not the account
--     role. The account role writes the reviews, so it must not also
--     delete the rows they release. The cutoff stays at least 30 days
--     ago. A row that needed review goes only once its review is 30 days
--     old too, so a new review never frees a row at once.
--   - A review always gets the database's time. It must be of a row
--     marked for review, and its reviewer must not be the row's actor.

-- +goose Up

ALTER TABLE game_tickets ADD COLUMN recent boolean NOT NULL DEFAULT false;

-- +goose StatementBegin
CREATE FUNCTION request_account_deletion(target bigint, wait interval)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF wait < interval '14 days' THEN
        RAISE EXCEPTION 'a deletion waits at least 14 days, not %', wait;
    END IF;
    INSERT INTO account_deletions (account_id, requested_at, delete_after)
    VALUES (target, now(), now() + wait)
    ON CONFLICT (account_id) DO NOTHING;
    RETURN FOUND;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION request_account_deletion(bigint, interval) FROM PUBLIC;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_account(target bigint, at timestamptz, account_storages integer[])
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    chars bigint[];
    led_parties bigint[];
    joined_guilds bigint[];
BEGIN
    -- The database clock decides, not the caller's: at only makes the
    -- call stricter.
    PERFORM 1 FROM account_deletions
    WHERE account_id = target AND delete_after <= least(at, now())
        AND delete_after - requested_at >= interval '14 days'
    FOR UPDATE;
    IF NOT FOUND THEN
        RETURN false;
    END IF;
    PERFORM 1 FROM accounts WHERE id = target FOR UPDATE;
    SELECT coalesce(array_agg(id), '{}') INTO chars FROM characters WHERE account_id = target;
    SELECT coalesce(array_agg(id), '{}') INTO led_parties FROM parties WHERE leader_id = ANY(chars);
    SELECT coalesce(array_agg(guild_id), '{}') INTO joined_guilds FROM guild_members WHERE character_id = ANY(chars);

    DELETE FROM chat_log WHERE character_id = ANY(chars) AND character_id <> 0;

    UPDATE item_log SET from_id = 0
    WHERE from_kind = 'storage' AND from_id = target AND (from_storage = 0 OR from_storage = ANY(account_storages));
    UPDATE item_log SET to_id = 0
    WHERE to_kind = 'storage' AND to_id = target AND (to_storage = 0 OR to_storage = ANY(account_storages));
    UPDATE zeny_log SET from_id = 0 WHERE from_kind = 'storage' AND from_id = target;
    UPDATE zeny_log SET to_id = 0 WHERE to_kind = 'storage' AND to_id = target;
    UPDATE guild_emblems SET uploaded_by = 0 WHERE uploaded_by = target;

    DELETE FROM characters WHERE account_id = target;
    DELETE FROM parties p
    WHERE p.id = ANY(led_parties) AND NOT EXISTS (SELECT 1 FROM party_members m WHERE m.party_id = p.id);
    DELETE FROM guilds g
    WHERE g.id = ANY(joined_guilds) AND NOT EXISTS (SELECT 1 FROM guild_members m WHERE m.guild_id = g.id);

    DELETE FROM accounts WHERE id = target;
    RETURN true;
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
        AND (NOT a.needs_review OR EXISTS (SELECT 1 FROM staff_audit_reviews r
            WHERE r.audit_id = a.id AND r.reviewed_at <= now() - interval '720 hours'));
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION check_staff_audit_review() RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    row staff_audit%ROWTYPE;
BEGIN
    SELECT * INTO row FROM staff_audit WHERE id = NEW.audit_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'there is no audit row %', NEW.audit_id;
    END IF;
    IF NOT row.needs_review THEN
        RAISE EXCEPTION 'the audit row % is not marked for review', NEW.audit_id;
    END IF;
    IF row.account_id = NEW.reviewer_id THEN
        RAISE EXCEPTION 'no one reviews their own action';
    END IF;
    NEW.reviewed_at := now();
    RETURN NEW;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION check_staff_audit_review() FROM PUBLIC;

CREATE TRIGGER staff_audit_reviews_check BEFORE INSERT ON staff_audit_reviews
FOR EACH ROW EXECUTE FUNCTION check_staff_audit_review();

-- Rights, when the roles exist (deploy/postgres/roles.sql):
--   - wathena_account asks for deletions through the function, and
--     reads and cancels them. It cannot insert them or delete audit rows.
--   - wathena_game deletes old audit rows.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_account') THEN
        REVOKE INSERT ON account_deletions FROM wathena_account;
        GRANT EXECUTE ON FUNCTION request_account_deletion(bigint, interval) TO wathena_account;
        REVOKE EXECUTE ON FUNCTION delete_old_staff_audit(timestamptz) FROM wathena_account;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT EXECUTE ON FUNCTION delete_old_staff_audit(timestamptz) TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        REVOKE EXECUTE ON FUNCTION delete_old_staff_audit(timestamptz) FROM wathena_game;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_account') THEN
        GRANT EXECUTE ON FUNCTION delete_old_staff_audit(timestamptz) TO wathena_account;
        GRANT INSERT ON account_deletions TO wathena_account;
    END IF;
END
$$;
-- +goose StatementEnd

DROP TRIGGER staff_audit_reviews_check ON staff_audit_reviews;
DROP FUNCTION check_staff_audit_review();

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
    DELETE FROM staff_audit a
    WHERE a.at < cutoff
        AND (NOT a.needs_review OR EXISTS (SELECT 1 FROM staff_audit_reviews r WHERE r.audit_id = a.id));
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_account(target bigint, at timestamptz, account_storages integer[])
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    chars bigint[];
    led_parties bigint[];
    joined_guilds bigint[];
BEGIN
    PERFORM 1 FROM account_deletions WHERE account_id = target AND delete_after <= at FOR UPDATE;
    IF NOT FOUND THEN
        RETURN false;
    END IF;
    PERFORM 1 FROM accounts WHERE id = target FOR UPDATE;
    SELECT coalesce(array_agg(id), '{}') INTO chars FROM characters WHERE account_id = target;
    SELECT coalesce(array_agg(id), '{}') INTO led_parties FROM parties WHERE leader_id = ANY(chars);
    SELECT coalesce(array_agg(guild_id), '{}') INTO joined_guilds FROM guild_members WHERE character_id = ANY(chars);
    DELETE FROM chat_log WHERE character_id = ANY(chars) AND character_id <> 0;
    UPDATE item_log SET from_id = 0
    WHERE from_kind = 'storage' AND from_id = target AND (from_storage = 0 OR from_storage = ANY(account_storages));
    UPDATE item_log SET to_id = 0
    WHERE to_kind = 'storage' AND to_id = target AND (to_storage = 0 OR to_storage = ANY(account_storages));
    UPDATE zeny_log SET from_id = 0 WHERE from_kind = 'storage' AND from_id = target;
    UPDATE zeny_log SET to_id = 0 WHERE to_kind = 'storage' AND to_id = target;
    UPDATE guild_emblems SET uploaded_by = 0 WHERE uploaded_by = target;
    DELETE FROM characters WHERE account_id = target;
    DELETE FROM parties p
    WHERE p.id = ANY(led_parties) AND NOT EXISTS (SELECT 1 FROM party_members m WHERE m.party_id = p.id);
    DELETE FROM guilds g
    WHERE g.id = ANY(joined_guilds) AND NOT EXISTS (SELECT 1 FROM guild_members m WHERE m.guild_id = g.id);
    DELETE FROM accounts WHERE id = target;
    RETURN true;
END
$$;
-- +goose StatementEnd

DROP FUNCTION request_account_deletion(bigint, interval);
ALTER TABLE game_tickets DROP COLUMN recent;
