-- name: HasMigrationTable :one
-- HasMigrationTable reports whether goose made its table of applied
-- migrations.
SELECT EXISTS (
    SELECT FROM pg_catalog.pg_tables
    WHERE schemaname = current_schema() AND tablename = 'goose_db_version'
);
