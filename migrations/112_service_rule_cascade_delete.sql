-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION cascade_service_rules_on_delete()
RETURNS TRIGGER AS $$
DECLARE
    v_service_key TEXT := OLD.namespace || '/' || OLD.name;
BEGIN
    DELETE FROM svc_svc_rules
    WHERE id IN (
        SELECT rule_id FROM service_rule_refs WHERE service_ref = v_service_key
    );

    DELETE FROM svc_fqdn_rules
    WHERE id IN (
        SELECT rule_id FROM service_fqdn_rule_refs WHERE service_ref = v_service_key
    );

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_service_rules_on_delete ON services;
CREATE TRIGGER trg_service_rules_on_delete
    AFTER DELETE ON services
    FOR EACH ROW
    EXECUTE FUNCTION cascade_service_rules_on_delete();
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_service_rules_on_delete ON services;
DROP FUNCTION IF EXISTS cascade_service_rules_on_delete();
-- +goose StatementEnd


