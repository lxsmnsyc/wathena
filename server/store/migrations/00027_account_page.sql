-- The account page: data exports, account deletion and its job (phase
-- 6 step 5, proposal 0002 review decision 37, decision 0032).
--
-- Rules:
--   - account_exports keeps the time of each account's last data
--     export, so a player may export once a day.
--   - account_deletions keeps the accounts whose players asked for
--     deletion. The characters cannot play until delete_after, 14 days
--     after the request. A cancel deletes the row.
--   - delete_account deletes one account whose time has come: its
--     personal data, its characters and their chat lines. It runs with
--     the rights of the schema's owner, so the game role may call it
--     without the right to delete accounts or characters. It deletes
--     nothing for an account without a request that is due at the
--     time given. docs/decisions/0032-account-page.md lists what each
--     table keeps.
--   - The logs other players need keep the character IDs. The lines of
--     the account's storage name storage 0 in place of the account, so
--     no row links them to the account. Rows of the GM audit log keep
--     their IDs for their 2 years.
--   - The account role writes both tables. The game role reads the
--     requests, which refuse game logins, and runs delete_account from
--     the deletion job.

-- +goose Up

CREATE TABLE account_exports (
    account_id bigint PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    exported_at timestamptz NOT NULL
);

CREATE TABLE account_deletions (
    account_id bigint PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    requested_at timestamptz NOT NULL,
    delete_after timestamptz NOT NULL,
    CHECK (delete_after > requested_at)
);

CREATE INDEX account_deletions_due ON account_deletions (delete_after);

-- delete_account deletes an account whose deletion is due at the time
-- at. account_storages are the record IDs of the extra storages that
-- accounts own (server/storage's Record.Account), whose log lines name
-- the account. It returns false, and deletes nothing, for an account
-- without a due request.
-- +goose StatementBegin
CREATE FUNCTION delete_account(target bigint, at timestamptz, account_storages integer[])
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

    -- The characters' own chat lines. Lines of other players, such as
    -- whispers to the characters, stay.
    DELETE FROM chat_log WHERE character_id = ANY(chars) AND character_id <> 0;

    -- The logs keep the character IDs, but no row names the account.
    UPDATE item_log SET from_id = 0
    WHERE from_kind = 'storage' AND from_id = target AND (from_storage = 0 OR from_storage = ANY(account_storages));
    UPDATE item_log SET to_id = 0
    WHERE to_kind = 'storage' AND to_id = target AND (to_storage = 0 OR to_storage = ANY(account_storages));
    UPDATE zeny_log SET from_id = 0 WHERE from_kind = 'storage' AND from_id = target;
    UPDATE zeny_log SET to_id = 0 WHERE to_kind = 'storage' AND to_id = target;
    UPDATE guild_emblems SET uploaded_by = 0 WHERE uploaded_by = target;

    -- The characters, with what references them: inventories, skills,
    -- statuses, variables, block and friend lists, sanctions, party and
    -- guild memberships, carts and storages, currencies, stores, and the
    -- mails to them. Mails from them keep the sender's name without its
    -- ID.
    DELETE FROM characters WHERE account_id = target;
    -- The server moved the parties and guilds in memory first. A party
    -- or guild that has no member left goes too.
    DELETE FROM parties p
    WHERE p.id = ANY(led_parties) AND NOT EXISTS (SELECT 1 FROM party_members m WHERE m.party_id = p.id);
    DELETE FROM guilds g
    WHERE g.id = ANY(joined_guilds) AND NOT EXISTS (SELECT 1 FROM guild_members m WHERE m.guild_id = g.id);

    -- The account, with its password, two-factor settings, passkeys,
    -- sessions, tickets, login records, storages, currencies,
    -- variables, staff role, bans, export time and the request.
    DELETE FROM accounts WHERE id = target;
    RETURN true;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION delete_account(bigint, timestamptz, integer[]) FROM PUBLIC;

-- Rights, when the roles exist (deploy/postgres/roles.sql):
--   - wathena_account records exports, and adds and deletes requests.
--   - wathena_game reads requests and runs delete_account.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_account') THEN
        GRANT SELECT, INSERT, UPDATE ON account_exports TO wathena_account;
        GRANT SELECT, INSERT, DELETE ON account_deletions TO wathena_account;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT ON account_deletions TO wathena_game;
        GRANT EXECUTE ON FUNCTION delete_account(bigint, timestamptz, integer[]) TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP FUNCTION delete_account(bigint, timestamptz, integer[]);
DROP TABLE account_deletions;
DROP TABLE account_exports;
