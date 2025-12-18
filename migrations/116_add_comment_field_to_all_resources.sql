-- +goose Up

ALTER TABLE services ADD COLUMN comment TEXT NOT NULL DEFAULT '';
ALTER TABLE address_groups ADD COLUMN comment TEXT NOT NULL DEFAULT '';
ALTER TABLE networks ADD COLUMN comment TEXT NOT NULL DEFAULT '';
ALTER TABLE hosts ADD COLUMN comment TEXT NOT NULL DEFAULT '';
ALTER TABLE svc_svc_rules ADD COLUMN comment TEXT NOT NULL DEFAULT '';
ALTER TABLE svc_fqdn_rules ADD COLUMN comment TEXT NOT NULL DEFAULT '';
ALTER TABLE address_group_bindings ADD COLUMN comment TEXT NOT NULL DEFAULT '';
ALTER TABLE network_bindings ADD COLUMN comment TEXT NOT NULL DEFAULT '';
ALTER TABLE host_bindings ADD COLUMN comment TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE services DROP COLUMN IF EXISTS comment;
ALTER TABLE address_groups DROP COLUMN IF EXISTS comment;
ALTER TABLE networks DROP COLUMN IF EXISTS comment;
ALTER TABLE hosts DROP COLUMN IF EXISTS comment;
ALTER TABLE svc_svc_rules DROP COLUMN IF EXISTS comment;
ALTER TABLE svc_fqdn_rules DROP COLUMN IF EXISTS comment;
ALTER TABLE address_group_bindings DROP COLUMN IF EXISTS comment;
ALTER TABLE network_bindings DROP COLUMN IF EXISTS comment;
ALTER TABLE host_bindings DROP COLUMN IF EXISTS comment;
