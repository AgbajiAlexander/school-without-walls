-- 000003_auth_hardening.down.sql
DROP TABLE IF EXISTS auth_rate_limits;
DROP TABLE IF EXISTS platform_admins;

DELETE FROM schema_migrations WHERE version = '000003_auth_hardening';
