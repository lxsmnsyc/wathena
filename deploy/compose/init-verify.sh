#!/bin/sh
# Makes the wathena_verify role of `wactl world verify-backup` when it is
# missing, and sets its password from the Docker secret on every run
# (decision 0048). The verify-init service runs it, only with the verify
# profile, so a stack without backup-verify has no such role.
#
# The password is read from a file inside psql, so it never appears in
# process arguments. Statement logging is off for this session, so the
# ALTER ROLE statement is not written to the server log.
set -eu

PGPASSWORD=$(cat /run/secrets/postgres_password)
export PGPASSWORD

psql -X -q -v ON_ERROR_STOP=1 <<'SQL'
\set verify `cat /run/secrets/verify_password`
SET log_statement = 'none';
SET log_min_error_statement = 'panic';
SET log_min_duration_statement = -1;
SET password_encryption = 'scram-sha-256';
-- wathena_verify may make databases and nothing else. It cannot connect
-- to wathena, which grants CONNECT to the game and account roles only.
SELECT 'CREATE ROLE wathena_verify' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_verify') \gexec
ALTER ROLE wathena_verify LOGIN CREATEDB NOSUPERUSER NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD :'verify';
SQL
echo "verify-init: the verify role is ready"
