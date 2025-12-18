-- +goose Up

-- +goose StatementBegin
CREATE TABLE ie_cidr_svc_rules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    namespace namespace_name NOT NULL,
    name resource_name NOT NULL,
    transport transport_protocol NOT NULL,
    cidr TEXT NOT NULL,
    service_ref JSONB NOT NULL,
    traffic traffic_direction NOT NULL,
    ports JSONB NOT NULL DEFAULT '[]',
    logs BOOLEAN NOT NULL DEFAULT false,
    trace BOOLEAN NOT NULL DEFAULT false,
    action rule_action NOT NULL,
    priority INT NOT NULL DEFAULT 0 CHECK (priority >= 0 AND priority <= 1000),
    description TEXT NOT NULL DEFAULT '',
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resource_version BIGINT NOT NULL,
    UNIQUE (namespace, name)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE service_ie_cidr_svc_rule_refs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    service_ref TEXT NOT NULL,
    rule_id UUID NOT NULL REFERENCES ie_cidr_svc_rules(id) ON DELETE CASCADE,
    rule_namespace TEXT NOT NULL,
    rule_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_service_ie_cidr_svc_rule_refs_service ON service_ie_cidr_svc_rule_refs(service_ref);
CREATE INDEX idx_service_ie_cidr_svc_rule_refs_rule_id ON service_ie_cidr_svc_rule_refs(rule_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX idx_ie_cidr_svc_rules_unique_key
    ON ie_cidr_svc_rules(
        namespace,
        cidr,
        (service_ref->>'namespace'),
        (service_ref->>'name'),
        traffic
    );

CREATE INDEX idx_ie_cidr_svc_rules_service_namespace ON ie_cidr_svc_rules USING btree ((service_ref->>'namespace'));
CREATE INDEX idx_ie_cidr_svc_rules_service_name ON ie_cidr_svc_rules USING btree ((service_ref->>'name'));
CREATE INDEX idx_ie_cidr_svc_rules_cidr ON ie_cidr_svc_rules(cidr);
CREATE INDEX idx_ie_cidr_svc_rules_traffic ON ie_cidr_svc_rules(traffic);
CREATE INDEX idx_ie_cidr_svc_rules_transport ON ie_cidr_svc_rules(transport);
CREATE INDEX idx_ie_cidr_svc_rules_name ON ie_cidr_svc_rules(name);
CREATE INDEX idx_ie_cidr_svc_rules_namespace ON ie_cidr_svc_rules(namespace);
CREATE INDEX idx_ie_cidr_svc_rules_namespace_name ON ie_cidr_svc_rules(namespace, name);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE ie_cidr_svc_rules
    ADD CONSTRAINT ie_cidr_svc_rules_resource_version_fkey
    FOREIGN KEY (resource_version)
    REFERENCES k8s_metadata(resource_version)
    ON UPDATE CASCADE
    ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION sync_ie_cidr_svc_rule_refs()
RETURNS TRIGGER AS $$
DECLARE
    v_service_namespace TEXT;
    v_service_name TEXT;
    v_service_key TEXT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_service_namespace := NEW.service_ref->>'namespace';
        v_service_name := NEW.service_ref->>'name';
        v_service_key := v_service_namespace || '/' || v_service_name;

        INSERT INTO service_ie_cidr_svc_rule_refs (service_ref, rule_id, rule_namespace, rule_name)
        VALUES (v_service_key, NEW.id, NEW.namespace, NEW.name);

        RETURN NEW;

    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.cidr IS DISTINCT FROM NEW.cidr
           OR OLD.service_ref::text != NEW.service_ref::text
           OR OLD.traffic IS DISTINCT FROM NEW.traffic THEN
            RAISE EXCEPTION 'cidr, serviceRef, and traffic fields are immutable';
        END IF;

        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_ie_cidr_svc_rule_refs
    AFTER INSERT OR UPDATE OR DELETE ON ie_cidr_svc_rules
    FOR EACH ROW
    EXECUTE FUNCTION sync_ie_cidr_svc_rule_refs();
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_ie_cidr_svc_rule_upsert_outbox()
RETURNS TRIGGER AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.cidr IS NOT DISTINCT FROM OLD.cidr
           AND NEW.service_ref::text IS NOT DISTINCT FROM OLD.service_ref::text
           AND NEW.traffic IS NOT DISTINCT FROM OLD.traffic
           AND NEW.transport IS NOT DISTINCT FROM OLD.transport
           AND NEW.ports::text IS NOT DISTINCT FROM OLD.ports::text
           AND NEW.logs IS NOT DISTINCT FROM OLD.logs
           AND NEW.trace IS NOT DISTINCT FROM OLD.trace
           AND NEW.action IS NOT DISTINCT FROM OLD.action
           AND NEW.priority IS NOT DISTINCT FROM OLD.priority
           AND NEW.description IS NOT DISTINCT FROM OLD.description
           AND NEW.comment IS NOT DISTINCT FROM OLD.comment THEN
            RETURN NEW;
        END IF;
        v_operation_type := 'UPDATE'::sync_operation;
    ELSE
        v_operation_type := 'CREATE'::sync_operation;
    END IF;

    v_resource_id := uuid_generate_v5(
        uuid_ns_dns(),
        'IECidrSvcRule:' || NEW.namespace || '/' || NEW.name
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
        'IECidrSvcRule',
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'transport', NEW.transport,
            'cidr', NEW.cidr,
            'service_ref', NEW.service_ref,
            'traffic', NEW.traffic,
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

CREATE TRIGGER trg_ie_cidr_svc_rule_upsert_outbox
    AFTER INSERT OR UPDATE ON ie_cidr_svc_rules
    FOR EACH ROW
    EXECUTE FUNCTION trigger_ie_cidr_svc_rule_upsert_outbox();
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_ie_cidr_svc_rule_before_delete()
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
        'IECidrSvcRule',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'transport', OLD.transport,
            'cidr', OLD.cidr,
            'service_ref', OLD.service_ref,
            'traffic', OLD.traffic
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

CREATE TRIGGER trg_ie_cidr_svc_rule_before_delete
    BEFORE DELETE ON ie_cidr_svc_rules
    FOR EACH ROW
    EXECUTE FUNCTION trigger_ie_cidr_svc_rule_before_delete();
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE services
    ADD COLUMN xie_cidr_svc_rules JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX idx_services_xie_cidr_svc_rules ON services USING GIN(xie_cidr_svc_rules);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_service_xie_cidr_svc_rules()
RETURNS TRIGGER AS $$
DECLARE
    v_rule_ref JSONB;
    v_service_key TEXT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_rule_ref := jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'IECidrSvcRule',
            'name', NEW.rule_name,
            'namespace', NEW.rule_namespace
        );

        SELECT NEW.service_ref INTO v_service_key;

        UPDATE services
        SET xie_cidr_svc_rules = xie_cidr_svc_rules || jsonb_build_array(v_rule_ref)
        WHERE services.namespace || '/' || services.name = v_service_key;

        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        v_rule_ref := jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'IECidrSvcRule',
            'name', OLD.rule_name,
            'namespace', OLD.rule_namespace
        );

        SELECT OLD.service_ref INTO v_service_key;

        UPDATE services
        SET xie_cidr_svc_rules = (
            SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
            FROM jsonb_array_elements(xie_cidr_svc_rules) AS elem
            WHERE elem != v_rule_ref
        )
        WHERE services.namespace || '/' || services.name = v_service_key;

        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_service_xie_cidr_svc_rules
    AFTER INSERT OR DELETE ON service_ie_cidr_svc_rule_refs
    FOR EACH ROW
    EXECUTE FUNCTION update_service_xie_cidr_svc_rules();
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_ie_cidr_svc_rules_change()
RETURNS TRIGGER AS $$
DECLARE
    notification JSON;
    operation TEXT;
    ns TEXT;
    n TEXT;
    rv BIGINT;
    data_payload JSON;
BEGIN
    IF (TG_OP = 'DELETE') THEN
        operation := 'DELETE';
        ns := OLD.namespace;
        n := OLD.name;
        rv := nextval('k8s_metadata_resource_version_seq');
        data_payload := row_to_json(OLD);
    ELSIF (TG_OP = 'INSERT') THEN
        operation := 'INSERT';
        ns := NEW.namespace;
        n := NEW.name;
        rv := NEW.resource_version;
        data_payload := row_to_json(NEW);
    ELSIF (TG_OP = 'UPDATE') THEN
        IF OLD.cidr IS NOT DISTINCT FROM NEW.cidr
           AND OLD.service_ref::text IS NOT DISTINCT FROM NEW.service_ref::text
           AND OLD.traffic IS NOT DISTINCT FROM NEW.traffic
           AND OLD.transport IS NOT DISTINCT FROM NEW.transport
           AND OLD.ports::text IS NOT DISTINCT FROM NEW.ports::text
           AND OLD.logs IS NOT DISTINCT FROM NEW.logs
           AND OLD.trace IS NOT DISTINCT FROM NEW.trace
           AND OLD.action IS NOT DISTINCT FROM NEW.action
           AND OLD.priority IS NOT DISTINCT FROM NEW.priority
           AND OLD.description IS NOT DISTINCT FROM NEW.description
           AND OLD.comment IS NOT DISTINCT FROM NEW.comment
           AND OLD.namespace IS NOT DISTINCT FROM NEW.namespace
           AND OLD.name IS NOT DISTINCT FROM NEW.name THEN
            RETURN NEW;
        END IF;

        operation := 'UPDATE';
        ns := NEW.namespace;
        n := NEW.name;
        rv := NEW.resource_version;
        data_payload := row_to_json(NEW);
    END IF;

    notification := json_build_object(
        'operation', operation,
        'resource_type', 'ie_cidr_svc_rules',
        'namespace', ns,
        'name', n,
        'resource_version', rv,
        'data', data_payload,
        'timestamp', NOW()
    );

    PERFORM pg_notify('k8s_resource_changes', notification::TEXT);

    IF (TG_OP = 'DELETE') THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER notify_ie_cidr_svc_rules_change
    AFTER INSERT OR UPDATE OR DELETE ON ie_cidr_svc_rules
    FOR EACH ROW
    EXECUTE FUNCTION notify_ie_cidr_svc_rules_change();
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TRIGGER IF EXISTS notify_ie_cidr_svc_rules_change ON ie_cidr_svc_rules;
DROP FUNCTION IF EXISTS notify_ie_cidr_svc_rules_change();

DROP TRIGGER IF EXISTS trg_update_service_xie_cidr_svc_rules ON service_ie_cidr_svc_rule_refs;
DROP FUNCTION IF EXISTS update_service_xie_cidr_svc_rules();

DROP TRIGGER IF EXISTS trg_ie_cidr_svc_rule_before_delete ON ie_cidr_svc_rules;
DROP FUNCTION IF EXISTS trigger_ie_cidr_svc_rule_before_delete();

DROP TRIGGER IF EXISTS trg_ie_cidr_svc_rule_upsert_outbox ON ie_cidr_svc_rules;
DROP FUNCTION IF EXISTS trigger_ie_cidr_svc_rule_upsert_outbox();

DROP TRIGGER IF EXISTS trg_sync_ie_cidr_svc_rule_refs ON ie_cidr_svc_rules;
DROP FUNCTION IF EXISTS sync_ie_cidr_svc_rule_refs();

DROP INDEX IF EXISTS idx_services_xie_cidr_svc_rules;
ALTER TABLE services DROP COLUMN IF EXISTS xie_cidr_svc_rules;

ALTER TABLE ie_cidr_svc_rules DROP CONSTRAINT IF EXISTS ie_cidr_svc_rules_resource_version_fkey;

DROP INDEX IF EXISTS idx_ie_cidr_svc_rules_namespace_name;
DROP INDEX IF EXISTS idx_ie_cidr_svc_rules_namespace;
DROP INDEX IF EXISTS idx_ie_cidr_svc_rules_name;
DROP INDEX IF EXISTS idx_ie_cidr_svc_rules_transport;
DROP INDEX IF EXISTS idx_ie_cidr_svc_rules_traffic;
DROP INDEX IF EXISTS idx_ie_cidr_svc_rules_cidr;
DROP INDEX IF EXISTS idx_ie_cidr_svc_rules_service_name;
DROP INDEX IF EXISTS idx_ie_cidr_svc_rules_service_namespace;
DROP INDEX IF EXISTS idx_ie_cidr_svc_rules_unique_key;

DROP INDEX IF EXISTS idx_service_ie_cidr_svc_rule_refs_rule_id;
DROP INDEX IF EXISTS idx_service_ie_cidr_svc_rule_refs_service;
DROP TABLE IF EXISTS service_ie_cidr_svc_rule_refs;

DROP TABLE IF EXISTS ie_cidr_svc_rules;
-- +goose StatementEnd
