-- +goose Up

-- +goose StatementBegin
ALTER TABLE service_fqdn_rule_refs
    ADD COLUMN rule_namespace TEXT NOT NULL DEFAULT '',
    ADD COLUMN rule_name TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE service_fqdn_rule_refs AS refs
SET rule_namespace = rules.namespace,
    rule_name = rules.name
FROM svc_fqdn_rules AS rules
WHERE rules.id = refs.rule_id;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE service_fqdn_rule_refs
    ALTER COLUMN rule_namespace DROP DEFAULT,
    ALTER COLUMN rule_name DROP DEFAULT;
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

        INSERT INTO service_fqdn_rule_refs (service_ref, rule_id, rule_namespace, rule_name)
        VALUES (v_service_key, NEW.id, NEW.namespace, NEW.name);

        RETURN NEW;

    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.service_from_ref::text != NEW.service_from_ref::text
           OR OLD.fqdn IS DISTINCT FROM NEW.fqdn
           OR OLD.transport IS DISTINCT FROM NEW.transport THEN
            RAISE EXCEPTION 'serviceFrom, FQDN and transport fields are immutable';
        END IF;

        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        DELETE FROM service_fqdn_rule_refs
        WHERE rule_id = OLD.id;

        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_service_xsvc_fqdn_rules()
RETURNS TRIGGER AS $$
DECLARE
    v_rule_ref JSONB;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_rule_ref := jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'SvcFqdnRule',
            'name', NEW.rule_name,
            'namespace', NEW.rule_namespace
        );

        UPDATE services
        SET xsvc_fqdn_rules = xsvc_fqdn_rules || jsonb_build_array(v_rule_ref)
        WHERE services.namespace || '/' || services.name = NEW.service_ref;

        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        v_rule_ref := jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'SvcFqdnRule',
            'name', OLD.rule_name,
            'namespace', OLD.rule_namespace
        );

        UPDATE services
        SET xsvc_fqdn_rules = (
            SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
            FROM jsonb_array_elements(xsvc_fqdn_rules) AS elem
            WHERE elem != v_rule_ref
        )
        WHERE services.namespace || '/' || services.name = OLD.service_ref;

        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE service_fqdn_rule_refs
    DROP COLUMN IF EXISTS rule_namespace,
    DROP COLUMN IF EXISTS rule_name;
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
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_service_xsvc_fqdn_rules()
RETURNS TRIGGER AS $$
DECLARE
    v_rule_ref JSONB;
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

        UPDATE services
        SET xsvc_fqdn_rules = xsvc_fqdn_rules || jsonb_build_array(v_rule_ref)
        WHERE services.namespace || '/' || services.name = NEW.service_ref;

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

        UPDATE services
        SET xsvc_fqdn_rules = (
            SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
            FROM jsonb_array_elements(xsvc_fqdn_rules) AS elem
            WHERE elem != v_rule_ref
        )
        WHERE services.namespace || '/' || services.name = OLD.service_ref;

        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

