#!/bin/sh
# Makes the wathena database and its roles on the first run, from
# deploy/postgres/roles.sql. Every run makes the wathena_verify role of
# `wactl world verify-backup` when it is missing (decision 0048), and
# sets the roles' passwords from the Docker secrets, so a changed secret
# takes effect on the next `up`.
#
# Passwords are read from files inside psql, so they never appear in
# process arguments. Statement logging is off for this session, so the
# ALTER ROLE statements are not written to the server log.
set -eu

PGPASSWORD=$(cat /run/secrets/postgres_password)
export PGPASSWORD

exists=$(psql -X -tAc "SELECT 1 FROM pg_database WHERE datname = 'wathena'")
if [ "$exists" != 1 ]; then
	echo "db-init: making the wathena database and roles"
	psql -X -q -v ON_ERROR_STOP=1 -f /deploy/roles.sql
fi

psql -X -q -v ON_ERROR_STOP=1 <<'SQL'
\set migrator `cat /run/secrets/migrator_password`
\set game `cat /run/secrets/game_password`
\set account `cat /run/secrets/account_password`
\set verify `cat /run/secrets/verify_password`
SET log_statement = 'none';
SET log_min_error_statement = 'panic';
SET log_min_duration_statement = -1;
SET password_encryption = 'scram-sha-256';
ALTER ROLE wathena_migrator PASSWORD :'migrator';
ALTER ROLE wathena_game PASSWORD :'game';
ALTER ROLE wathena_account PASSWORD :'account';
-- wathena_verify may make databases and nothing else. It cannot connect
-- to wathena, which grants CONNECT to the game and account roles only.
SELECT 'CREATE ROLE wathena_verify' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_verify') \gexec
ALTER ROLE wathena_verify LOGIN CREATEDB NOSUPERUSER NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD :'verify';
SQL
echo "db-init: roles ready"
