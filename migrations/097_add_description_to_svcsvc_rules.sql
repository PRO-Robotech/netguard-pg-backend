-- +goose Up
ALTER TABLE svc_svc_rules
ADD COLUMN description TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN svc_svc_rules.description IS
    'Optional human-readable description (Netguard-only, not synced to SGROUPS)';

-- +goose Down
ALTER TABLE svc_svc_rules
DROP COLUMN IF EXISTS description;

