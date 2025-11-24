-- +goose Up
WITH missing_null_rv AS (
    SELECT id, COALESCE(created_at, NOW()) AS created_at, COALESCE(updated_at, NOW()) AS updated_at
    FROM svc_svc_rules
    WHERE resource_version IS NULL
),
updated_rules AS (
    UPDATE svc_svc_rules sr
    SET resource_version = nextval('k8s_metadata_resource_version_seq')
    FROM missing_null_rv mr
    WHERE sr.id = mr.id
    RETURNING sr.resource_version, mr.created_at, mr.updated_at
)
INSERT INTO k8s_metadata (
    resource_version,
    labels,
    annotations,
    finalizers,
    conditions,
    created_at,
    updated_at,
    uid,
    generation
)
SELECT
    resource_version,
    '{}'::jsonb,
    '{}'::jsonb,
    '{}'::text[],
    '[{"type":"Ready","status":"False","reason":"PendingSGROUPSync","message":"Awaiting synchronization with SGROUP before marking as ready"}]'::jsonb,
    created_at,
    updated_at,
    uuid_generate_v4(),
    1
FROM updated_rules;

WITH missing_metadata AS (
    SELECT sr.resource_version, COALESCE(sr.created_at, NOW()) AS created_at, COALESCE(sr.updated_at, NOW()) AS updated_at
    FROM svc_svc_rules sr
    LEFT JOIN k8s_metadata km ON km.resource_version = sr.resource_version
    WHERE sr.resource_version IS NOT NULL
      AND km.resource_version IS NULL
)
INSERT INTO k8s_metadata (
    resource_version,
    labels,
    annotations,
    finalizers,
    conditions,
    created_at,
    updated_at,
    uid,
    generation
)
SELECT
    resource_version,
    '{}'::jsonb,
    '{}'::jsonb,
    '{}'::text[],
    '[{"type":"Ready","status":"False","reason":"PendingSGROUPSync","message":"Awaiting synchronization with SGROUP before marking as ready"}]'::jsonb,
    created_at,
    updated_at,
    uuid_generate_v4(),
    1
FROM missing_metadata;

ALTER TABLE svc_svc_rules
    ALTER COLUMN resource_version SET NOT NULL;

ALTER TABLE svc_svc_rules
    DROP CONSTRAINT IF EXISTS svc_svc_rules_resource_version_fkey;

ALTER TABLE svc_svc_rules
    ADD CONSTRAINT svc_svc_rules_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE svc_fqdn_rules
    ALTER COLUMN resource_version SET NOT NULL;

ALTER TABLE svc_fqdn_rules
    DROP CONSTRAINT IF EXISTS svc_fqdn_rules_resource_version_fkey;

ALTER TABLE svc_fqdn_rules
    ADD CONSTRAINT svc_fqdn_rules_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;

-- +goose Down

ALTER TABLE svc_fqdn_rules
    DROP CONSTRAINT IF EXISTS svc_fqdn_rules_resource_version_fkey;

ALTER TABLE svc_svc_rules
    DROP CONSTRAINT IF EXISTS svc_svc_rules_resource_version_fkey;

ALTER TABLE svc_svc_rules
    ALTER COLUMN resource_version DROP NOT NULL;

