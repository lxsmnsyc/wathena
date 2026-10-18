-- name: DatabaseRole :one
-- DatabaseRole reads the rights of the connection's role, for waserver's
-- start check (review 2026-10): whether the session's or the current
-- role is a superuser, whether the current role may make roles or
-- databases, skips row security or replicates, whether it owns an object
-- in the public schema, as the migrator does, and whether it can read
-- account_passwords, as the account role can.
SELECT current_user::text AS name,
	(s.rolsuper OR c.rolsuper)::boolean AS super,
	c.rolcreaterole::boolean AS create_role,
	c.rolcreatedb::boolean AS create_db,
	c.rolbypassrls::boolean AS bypass_rls,
	c.rolreplication::boolean AS replication,
	EXISTS (
		SELECT 1 FROM pg_catalog.pg_class k JOIN pg_catalog.pg_namespace n ON n.oid = k.relnamespace
		WHERE n.nspname = 'public' AND pg_has_role(current_user, k.relowner, 'USAGE')
	)::boolean AS owns,
	COALESCE(has_table_privilege(to_regclass('public.account_passwords'), 'SELECT'), false)::boolean AS reads_passwords
FROM pg_catalog.pg_roles c, pg_catalog.pg_roles s
WHERE c.rolname = current_user AND s.rolname = session_user;
