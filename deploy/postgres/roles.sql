-- Makes the wAthena database and its four roles. Run it once as a
-- PostgreSQL superuser, before `wactl world new` or `wactl migrate up`:
--
--   psql -U postgres -f deploy/postgres/roles.sql
--
-- Roles (strategy section 7.8):
--   - wathena_migrator owns the database and applies migrations with
--     `wactl migrate up`. Only use it for migrations.
--   - wathena_game is what the game side of waserver connects as. It
--     can read and write rows in the tables it needs, and cannot change
--     the schema. It cannot read password hashes or login sessions. It
--     only reads and deletes game tickets. It deletes accounts only
--     through delete_account, for a deletion whose time has come on the
--     database clock after a wait of at least 14 days (decision 0032,
--     review decision 51). It deletes GM audit rows only through
--     delete_old_staff_audit, for rows older than 30 days whose review,
--     if they need one, is older than 30 days too.
--   - wathena_account is what the account service connects as. It
--     reads and writes passwords, login sessions, game tickets, TOTP
--     secrets, recovery codes and passkeys, and makes accounts and
--     characters. It cannot change or delete characters, or change the
--     schema. It asks for account deletions only through
--     request_account_deletion, which sets the times itself, and it
--     cannot delete GM audit rows. The game role has no rights on TOTP
--     secrets, recovery codes or passkeys.
--   - wathena_backup is what `wactl world backup` connects as. It is a
--     member of pg_read_all_data, so it reads every table and changes
--     nothing. The backup holds what it reads, password hashes included.
--
-- The migrations grant the table rights of the game and account roles.
--
-- No role has a password yet. Set them in psql, which sends only a hash
-- to the server:
--
--   \password wathena_migrator
--   \password wathena_game
--   \password wathena_account
--   \password wathena_backup
--
-- Then give the URLs to the tools through environment variables, so the
-- passwords do not show in process lists:
--
--   WATHENA_MIGRATE_DATABASE_URL=postgres://wathena_migrator:...@host/wathena wactl migrate up
--   WATHENA_BACKUP_DATABASE_URL=postgres://wathena_backup:...@host/wathena wactl world backup ...
--   WATHENA_DATABASE_URL=postgres://wathena_game:...@host/wathena \
--   WATHENA_ACCOUNT_DATABASE_URL=postgres://wathena_account:...@host/wathena \
--   WATHENA_ACCOUNT_SECRET_KEY=<32 random bytes in base64> waserver ...

\set ON_ERROR_STOP on

CREATE ROLE wathena_migrator LOGIN;
CREATE ROLE wathena_game LOGIN;
CREATE ROLE wathena_account LOGIN;
CREATE ROLE wathena_backup LOGIN IN ROLE pg_read_all_data;
CREATE DATABASE wathena OWNER wathena_migrator;

\connect wathena

-- Only these roles may connect.
REVOKE ALL ON DATABASE wathena FROM PUBLIC;
GRANT CONNECT ON DATABASE wathena TO wathena_game, wathena_account, wathena_backup;

-- The public schema belongs to the database owner, wathena_migrator.
-- The other roles may use its tables but not create objects in it.
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO wathena_game, wathena_account;
