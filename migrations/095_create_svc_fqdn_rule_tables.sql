-- +goose Up

-- +goose StatementBegin

CREATE TABLE svc_fqdn_rules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    service_from_ref JSONB NOT NULL,
    fqdn TEXT NOT NULL,
    transport transport_protocol NOT NULL,
    ports JSONB NOT NULL DEFAULT '[]',
    logs BOOLEAN NOT NULL DEFAULT false,
    trace BOOLEAN NOT NULL DEFAULT false,
    action rule_action NOT NULL,
    priority INT NOT NULL DEFAULT 0 CHECK (priority >= 0 AND priority <= 1000),
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resource_version BIGINT NOT NULL,
    UNIQUE (namespace, name)
);

-- +goose StatementEnd
-- +goose StatementBegin

CREATE TABLE service_fqdn_rule_refs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    service_ref TEXT NOT NULL,
    rule_id UUID NOT NULL REFERENCES svc_fqdn_rules(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_service_fqdn_rule_refs_service ON service_fqdn_rule_refs(service_ref);
CREATE INDEX idx_service_fqdn_rule_refs_rule_id ON service_fqdn_rule_refs(rule_id);

-- +goose StatementEnd
-- +goose StatementBegin

CREATE UNIQUE INDEX idx_svc_fqdn_rules_unique_key
    ON svc_fqdn_rules(
        namespace,
        (service_from_ref->>'namespace'),
        (service_from_ref->>'name'),
        fqdn,
        transport
    );

CREATE INDEX idx_svc_fqdn_rules_service_from_namespace
    ON svc_fqdn_rules USING btree ((service_from_ref->>'namespace'));

CREATE INDEX idx_svc_fqdn_rules_service_from_name
    ON svc_fqdn_rules USING btree ((service_from_ref->>'name'));

CREATE INDEX idx_svc_fqdn_rules_fqdn
    ON svc_fqdn_rules(fqdn);

CREATE INDEX idx_svc_fqdn_rules_transport
    ON svc_fqdn_rules(transport);

-- +goose StatementEnd
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION sync_service_fqdn_rule_refs()
RETURNS TRIGGER AS $$
DECLARE
    v_service_namespace TEXT;
    v_service_name TEXT;
    v_service_key TEXT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_service_namespace := NEW.service_from_ref->>'namespace';
        v_service_name := NEW.service_from_ref->>'name';
        v_service_key := v_service_namespace || '/' || v_service_name;

        INSERT INTO service_fqdn_rule_refs (service_ref, rule_id)
        VALUES (v_service_key, NEW.id);

        RETURN NEW;

    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.service_from_ref::text != NEW.service_from_ref::text
           OR OLD.fqdn IS DISTINCT FROM NEW.fqdn
           OR OLD.transport IS DISTINCT FROM NEW.transport THEN
            RAISE EXCEPTION 'serviceFrom, FQDN and transport fields are immutable';
        END IF;

        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_service_fqdn_rule_refs
    AFTER INSERT OR UPDATE OR DELETE ON svc_fqdn_rules
    FOR EACH ROW
    EXECUTE FUNCTION sync_service_fqdn_rule_refs();

-- +goose StatementEnd
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION trigger_svcfqdn_rule_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
    END IF;

    v_resource_id := uuid_generate_v5(
        uuid_ns_dns(),
        'SvcFqdnRule:' || NEW.namespace || '/' || NEW.name
    );

    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'SvcFqdnRule',
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'service_from_ref', NEW.service_from_ref,
            'fqdn', NEW.fqdn,
            'transport', NEW.transport,
            'ports', NEW.ports,
            'logs', NEW.logs,
            'trace', NEW.trace,
            'action', NEW.action,
            'priority', NEW.priority,
            'description', NEW.description
        ),
        NEW.namespace,
        NEW.name,
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW(),
        payload = EXCLUDED.payload;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_svcfqdn_rule_upsert_outbox
    AFTER INSERT OR UPDATE ON svc_fqdn_rules
    FOR EACH ROW
    EXECUTE FUNCTION trigger_svcfqdn_rule_upsert_outbox();

-- +goose StatementEnd
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION trigger_svcfqdn_rule_before_delete()
RETURNS TRIGGER AS $$
DECLARE
    v_uid UUID;
    v_already_marked BOOLEAN;
BEGIN
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked THEN
        RETURN OLD;
    END IF;

    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'SvcFqdnRule',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'service_from_ref', OLD.service_from_ref,
            'fqdn', OLD.fqdn,
            'transport', OLD.transport
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_svcfqdn_rule_before_delete
    BEFORE DELETE ON svc_fqdn_rules
    FOR EACH ROW
    EXECUTE FUNCTION trigger_svcfqdn_rule_before_delete();

-- +goose StatementEnd
-- +goose StatementBegin

ALTER TABLE services
    ADD COLUMN xsvc_fqdn_rules JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX idx_services_xsvc_fqdn_rules
    ON services USING GIN(xsvc_fqdn_rules);

-- +goose StatementEnd
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION update_service_xsvc_fqdn_rules()
RETURNS TRIGGER AS $$
DECLARE
    v_rule_ref JSONB;
    v_service_key TEXT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'SvcFqdnRule',
            'name', svc_fqdn_rules.name,
            'namespace', svc_fqdn_rules.namespace
        ) INTO v_rule_ref
        FROM svc_fqdn_rules
        WHERE id = NEW.rule_id;

        IF v_rule_ref IS NULL THEN
            RETURN NEW;
        END IF;

        SELECT NEW.service_ref INTO v_service_key;

        UPDATE services
        SET xsvc_fqdn_rules = xsvc_fqdn_rules || jsonb_build_array(v_rule_ref)
        WHERE services.namespace || '/' || services.name = v_service_key;

        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        SELECT jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'SvcFqdnRule',
            'name', svc_fqdn_rules.name,
            'namespace', svc_fqdn_rules.namespace
        ) INTO v_rule_ref
        FROM svc_fqdn_rules
        WHERE id = OLD.rule_id;

        IF v_rule_ref IS NULL THEN
            RETURN OLD;
        END IF;

        SELECT OLD.service_ref INTO v_service_key;

        UPDATE services
        SET xsvc_fqdn_rules = (
            SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
            FROM jsonb_array_elements(xsvc_fqdn_rules) AS elem
            WHERE elem != v_rule_ref
        )
        WHERE services.namespace || '/' || services.name = v_service_key;

        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_service_xsvc_fqdn_rules
    AFTER INSERT OR DELETE ON service_fqdn_rule_refs
    FOR EACH ROW
    EXECUTE FUNCTION update_service_xsvc_fqdn_rules();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS trg_update_service_xsvc_fqdn_rules ON service_fqdn_rule_refs;
DROP FUNCTION IF EXISTS update_service_xsvc_fqdn_rules();

DROP TRIGGER IF EXISTS trg_svcfqdn_rule_before_delete ON svc_fqdn_rules;
DROP FUNCTION IF EXISTS trigger_svcfqdn_rule_before_delete();

DROP TRIGGER IF EXISTS trg_svcfqdn_rule_upsert_outbox ON svc_fqdn_rules;
DROP FUNCTION IF EXISTS trigger_svcfqdn_rule_upsert_outbox();

DROP TRIGGER IF EXISTS trg_sync_service_fqdn_rule_refs ON svc_fqdn_rules;
DROP FUNCTION IF EXISTS sync_service_fqdn_rule_refs();

DROP INDEX IF EXISTS idx_services_xsvc_fqdn_rules;
ALTER TABLE services DROP COLUMN IF EXISTS xsvc_fqdn_rules;

DROP INDEX IF EXISTS idx_svc_fqdn_rules_transport;
DROP INDEX IF EXISTS idx_svc_fqdn_rules_fqdn;
DROP INDEX IF EXISTS idx_svc_fqdn_rules_service_from_name;
DROP INDEX IF EXISTS idx_svc_fqdn_rules_service_from_namespace;
DROP INDEX IF EXISTS idx_svc_fqdn_rules_unique_key;

DROP INDEX IF EXISTS idx_service_fqdn_rule_refs_rule_id;
DROP INDEX IF EXISTS idx_service_fqdn_rule_refs_service;
DROP TABLE IF EXISTS service_fqdn_rule_refs;

DROP TABLE IF EXISTS svc_fqdn_rules;

-- +goose StatementEnd

