-- Makes the wAthena database and its two roles. Run it once as a
-- PostgreSQL superuser, before the first `wactl migrate up`:
--
--   psql -U postgres -f deploy/postgres/roles.sql
--
-- Roles (strategy section 7.8):
--   - wathena_migrator owns the database and applies migrations with
--     `wactl migrate up`. Only use it for migrations.
--   - wathena_game is what waserver connects as. It can read and write
--     rows in the tables it needs, and cannot change the schema. The
--     migrations grant its table rights.
--
-- Neither role has a password yet. Set them in psql, which sends only a
-- hash to the server:
--
--   \password wathena_migrator
--   \password wathena_game
--
-- Then give the URLs to the tools through environment variables, so the
-- passwords do not show in process lists:
--
--   WATHENA_MIGRATE_DATABASE_URL=postgres://wathena_migrator:...@host/wathena wactl migrate up
--   WATHENA_DATABASE_URL=postgres://wathena_game:...@host/wathena waserver ...

\set ON_ERROR_STOP on

CREATE ROLE wathena_migrator LOGIN;
CREATE ROLE wathena_game LOGIN;
CREATE DATABASE wathena OWNER wathena_migrator;

\connect wathena

-- Only the two roles may connect.
REVOKE ALL ON DATABASE wathena FROM PUBLIC;
GRANT CONNECT ON DATABASE wathena TO wathena_game;

-- The public schema belongs to the database owner, wathena_migrator.
-- The game role may use its tables but not create objects in it.
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO wathena_game;
