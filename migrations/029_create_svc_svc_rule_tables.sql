-- +goose Up

-- +goose StatementBegin

CREATE TABLE svc_svc_rules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    namespace VARCHAR(253) NOT NULL,
    name VARCHAR(253) NOT NULL,
    service_from_ref JSONB NOT NULL,
    service_to_ref   JSONB NOT NULL,
    action VARCHAR(50) NOT NULL CHECK (action IN ('ACCEPT', 'DROP')),
    priority INT NOT NULL DEFAULT 0 CHECK (priority >= 0 AND priority <= 1000),
    logs BOOLEAN NOT NULL DEFAULT false,
    trace BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resource_version BIGINT
);

-- +goose StatementEnd
-- +goose StatementBegin

CREATE TABLE service_rule_refs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    service_ref VARCHAR(510) NOT NULL,
    rule_id UUID NOT NULL REFERENCES svc_svc_rules(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL CHECK (role IN ('SERVICE_FROM', 'SERVICE_TO')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_service_rule_refs_service ON service_rule_refs(service_ref);
CREATE INDEX idx_service_rule_refs_rule_id ON service_rule_refs(rule_id);

-- +goose StatementEnd
-- +goose StatementBegin

CREATE UNIQUE INDEX idx_svc_svc_rules_unique_pair
ON svc_svc_rules(
    namespace,
    (service_from_ref->>'namespace'),
    (service_from_ref->>'name'),
    (service_to_ref->>'namespace'),
    (service_to_ref->>'name')
);

-- +goose StatementEnd
-- +goose StatementBegin

CREATE INDEX idx_svc_svc_rules_service_from_name
    ON svc_svc_rules USING btree ((service_from_ref->>'name'));

CREATE INDEX idx_svc_svc_rules_service_from_namespace
    ON svc_svc_rules USING btree ((service_from_ref->>'namespace'));

CREATE INDEX idx_svc_svc_rules_service_to_name
    ON svc_svc_rules USING btree ((service_to_ref->>'name'));

CREATE INDEX idx_svc_svc_rules_service_to_namespace
    ON svc_svc_rules USING btree ((service_to_ref->>'namespace'));

DROP INDEX IF EXISTS idx_svc_svc_rules_namespace_name;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'uk_svc_svc_rules_namespace_name'
          AND conrelid = 'svc_svc_rules'::regclass
    ) THEN
        ALTER TABLE svc_svc_rules
            ADD CONSTRAINT uk_svc_svc_rules_namespace_name UNIQUE (namespace, name);
        RAISE NOTICE 'Created constraint uk_svc_svc_rules_namespace_name';
    ELSE
        RAISE NOTICE 'Constraint uk_svc_svc_rules_namespace_name already exists, skipping';
    END IF;
END$$;

CREATE INDEX idx_svc_svc_rules_action
    ON svc_svc_rules(action);

-- +goose StatementEnd
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION sync_service_rule_refs()
RETURNS TRIGGER AS $$
DECLARE
    v_service_from_key TEXT;
    v_service_to_key TEXT;
    v_from_namespace TEXT;
    v_from_name TEXT;
    v_to_namespace TEXT;
    v_to_name TEXT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_from_namespace := NEW.service_from_ref->>'namespace';
        v_from_name := NEW.service_from_ref->>'name';
        v_service_from_key := v_from_namespace || '/' || v_from_name;

        v_to_namespace := NEW.service_to_ref->>'namespace';
        v_to_name := NEW.service_to_ref->>'name';
        v_service_to_key := v_to_namespace || '/' || v_to_name;

        INSERT INTO service_rule_refs (service_ref, rule_id, role)
        VALUES
            (v_service_from_key, NEW.id, 'SERVICE_FROM'),
            (v_service_to_key, NEW.id, 'SERVICE_TO');

        RETURN NEW;

    ELSIF TG_OP = 'UPDATE' THEN
        v_from_namespace := NEW.service_from_ref->>'namespace';
        v_from_name := NEW.service_from_ref->>'name';
        v_service_from_key := v_from_namespace || '/' || v_from_name;

        v_to_namespace := NEW.service_to_ref->>'namespace';
        v_to_name := NEW.service_to_ref->>'name';
        v_service_to_key := v_to_namespace || '/' || v_to_name;

        IF OLD.service_from_ref::text != NEW.service_from_ref::text OR
           OLD.service_to_ref::text != NEW.service_to_ref::text THEN
            RAISE EXCEPTION 'serviceFrom and serviceTo are immutable fields (validation #10, #11)';
        END IF;

        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_service_rule_refs
    AFTER INSERT OR UPDATE OR DELETE ON svc_svc_rules
    FOR EACH ROW
    EXECUTE FUNCTION sync_service_rule_refs();

-- +goose StatementEnd
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION trigger_svcsvc_rule_upsert_outbox()
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
        'SvcSvcRule:' || NEW.namespace || '/' || NEW.name
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
        'SvcSvcRule',
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'service_from', NEW.service_from_ref,
            'service_to', NEW.service_to_ref,
            'action', NEW.action,
            'priority', NEW.priority,
            'logs', NEW.logs,
            'trace', NEW.trace
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

CREATE TRIGGER trg_svcsvc_rule_upsert_outbox
    AFTER INSERT OR UPDATE ON svc_svc_rules
    FOR EACH ROW
    EXECUTE FUNCTION trigger_svcsvc_rule_upsert_outbox();

-- +goose StatementEnd
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION trigger_svcsvc_rule_before_delete()
RETURNS TRIGGER AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
BEGIN
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
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
        'SvcSvcRule',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'service_from_ref', OLD.service_from_ref,
            'service_to_ref', OLD.service_to_ref
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

CREATE TRIGGER svcsvc_rule_before_delete
    BEFORE DELETE ON svc_svc_rules
    FOR EACH ROW
    EXECUTE FUNCTION trigger_svcsvc_rule_before_delete();

-- +goose StatementEnd
-- +goose StatementBegin

ALTER TABLE services
ADD COLUMN xsvcsvc_rules_as_from JSONB DEFAULT '[]'::jsonb NOT NULL,
ADD COLUMN xsvcsvc_rules_as_to JSONB DEFAULT '[]'::jsonb NOT NULL;

CREATE INDEX idx_services_xsvcsvc_rules_as_from
    ON services USING GIN(xsvcsvc_rules_as_from);

CREATE INDEX idx_services_xsvcsvc_rules_as_to
    ON services USING GIN(xsvcsvc_rules_as_to);

-- +goose StatementEnd
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION update_service_xsvcsvc_rules()
RETURNS TRIGGER AS $$
DECLARE
    v_rule_ref JSONB;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'SvcSvcRule',
            'name', svc_svc_rules.name,
            'namespace', svc_svc_rules.namespace
        ) INTO v_rule_ref
        FROM svc_svc_rules
        WHERE id = NEW.rule_id;

        IF NEW.role = 'SERVICE_FROM' THEN
            UPDATE services
            SET xsvcsvc_rules_as_from = xsvcsvc_rules_as_from || jsonb_build_array(v_rule_ref)
            WHERE services.namespace || '/' || services.name = NEW.service_ref;

        ELSIF NEW.role = 'SERVICE_TO' THEN
            UPDATE services
            SET xsvcsvc_rules_as_to = xsvcsvc_rules_as_to || jsonb_build_array(v_rule_ref)
            WHERE services.namespace || '/' || services.name = NEW.service_ref;
        END IF;

        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        SELECT jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'SvcSvcRule',
            'name', svc_svc_rules.name,
            'namespace', svc_svc_rules.namespace
        ) INTO v_rule_ref
        FROM svc_svc_rules
        WHERE id = OLD.rule_id;

        IF OLD.role = 'SERVICE_FROM' THEN
            UPDATE services
            SET xsvcsvc_rules_as_from = (
                SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
                FROM jsonb_array_elements(xsvcsvc_rules_as_from) AS elem
                WHERE elem != v_rule_ref
            )
            WHERE services.namespace || '/' || services.name = OLD.service_ref;

        ELSIF OLD.role = 'SERVICE_TO' THEN
            UPDATE services
            SET xsvcsvc_rules_as_to = (
                SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
                FROM jsonb_array_elements(xsvcsvc_rules_as_to) AS elem
                WHERE elem != v_rule_ref
            )
            WHERE services.namespace || '/' || services.name = OLD.service_ref;
        END IF;

        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_service_xsvcsvc_rules
    AFTER INSERT OR DELETE ON service_rule_refs
    FOR EACH ROW
    EXECUTE FUNCTION update_service_xsvcsvc_rules();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS trg_update_service_xsvcsvc_rules ON service_rule_refs;
DROP FUNCTION IF EXISTS update_service_xsvcsvc_rules();

DROP TRIGGER IF EXISTS svcsvc_rule_before_delete ON svc_svc_rules;
DROP FUNCTION IF EXISTS trigger_svcsvc_rule_before_delete();

DROP TRIGGER IF EXISTS trg_svcsvc_rule_upsert_outbox ON svc_svc_rules;
DROP FUNCTION IF EXISTS trigger_svcsvc_rule_upsert_outbox();

DROP TRIGGER IF EXISTS trg_sync_service_rule_refs ON svc_svc_rules;
DROP FUNCTION IF EXISTS sync_service_rule_refs();

DROP INDEX IF EXISTS idx_services_xsvcsvc_rules_as_to;
DROP INDEX IF EXISTS idx_services_xsvcsvc_rules_as_from;
DROP INDEX IF EXISTS idx_svc_svc_rules_action;
DROP INDEX IF EXISTS idx_svc_svc_rules_namespace_name;
DROP INDEX IF EXISTS idx_svc_svc_rules_service_to_namespace;
DROP INDEX IF EXISTS idx_svc_svc_rules_service_to_name;
DROP INDEX IF EXISTS idx_svc_svc_rules_service_from_namespace;
DROP INDEX IF EXISTS idx_svc_svc_rules_service_from_name;
DROP INDEX IF EXISTS idx_svc_svc_rules_unique_pair;
DROP INDEX IF EXISTS idx_service_rule_refs_rule_id;
DROP INDEX IF EXISTS idx_service_rule_refs_service;

ALTER TABLE services
DROP COLUMN IF EXISTS xsvcsvc_rules_as_to,
DROP COLUMN IF EXISTS xsvcsvc_rules_as_from;

DROP TABLE IF EXISTS service_rule_refs CASCADE;
DROP TABLE IF EXISTS svc_svc_rules CASCADE;

-- +goose StatementEnd
