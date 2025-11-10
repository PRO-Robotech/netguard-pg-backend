-- +goose Up
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION cascade_service_svcsvc_rules()
RETURNS TRIGGER AS $$
DECLARE
    rule RECORD;
BEGIN
    FOR rule IN
        SELECT sr.namespace, sr.name
        FROM svc_svc_rules sr
        JOIN k8s_metadata km ON sr.resource_version = km.resource_version
        WHERE km.deletion_timestamp IS NULL
          AND (
            (sr.service_from_ref->>'namespace' = OLD.namespace AND sr.service_from_ref->>'name' = OLD.name) OR
            (sr.service_to_ref->>'namespace' = OLD.namespace AND sr.service_to_ref->>'name' = OLD.name)
          )
    LOOP
        DELETE FROM svc_svc_rules
        WHERE namespace = rule.namespace
          AND name = rule.name;
    END LOOP;

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cascade_service_svcsvc_rules() IS
'Marks dependent SvcSvcRule resources for deletion when a Service is removed. '
'Existing SvcSvcRule triggers handle outbox entries and physical cleanup.';

CREATE TRIGGER cascade_service_svcsvc_rules_after_delete
    AFTER DELETE ON services
    FOR EACH ROW
    EXECUTE FUNCTION cascade_service_svcsvc_rules();

COMMENT ON TRIGGER cascade_service_svcsvc_rules_after_delete ON services IS
'Ensures SvcSvcRule entries referencing a deleted Service are soft-deleted.';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS cascade_service_svcsvc_rules_after_delete ON services;
DROP FUNCTION IF EXISTS cascade_service_svcsvc_rules();

-- +goose StatementEnd

