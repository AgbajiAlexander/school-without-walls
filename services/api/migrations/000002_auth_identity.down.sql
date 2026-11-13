-- 000002_auth_identity.down.sql
-- Safe rollback for auth and identity schema

DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS password_resets;
DROP TABLE IF EXISTS invitations;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;

DROP TYPE IF EXISTS invitation_status;
DROP TYPE IF EXISTS user_status;

DELETE FROM schema_migrations WHERE version = '000002_auth_identity';
