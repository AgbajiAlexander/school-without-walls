-- 000001_initial_schema.down.sql
-- Safe rollback for baseline migration

DELETE FROM schema_migrations WHERE version = '000001_initial_schema';
DROP TABLE IF EXISTS schema_migrations;
