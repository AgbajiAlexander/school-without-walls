-- 000004_tenants.down.sql
DROP INDEX IF EXISTS idx_audit_events_tenant_created;
ALTER TABLE audit_events DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE invitations DROP CONSTRAINT IF EXISTS invitations_tenant_fk;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS tenants;
DROP TYPE IF EXISTS membership_status;
DROP TYPE IF EXISTS tenant_status;
DELETE FROM schema_migrations WHERE version = '000004_tenants';
