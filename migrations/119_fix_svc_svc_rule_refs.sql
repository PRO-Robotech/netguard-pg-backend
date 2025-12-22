-- +goose Up

-- +goose StatementBegin
ALTER TABLE service_rule_refs
    ADD COLUMN rule_namespace TEXT NOT NULL DEFAULT '',
    ADD COLUMN rule_name TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE service_rule_refs AS refs
SET rule_namespace = rules.namespace,
    rule_name = rules.name
FROM svc_svc_rules AS rules
WHERE rules.id = refs.rule_id;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE service_rule_refs
    ALTER COLUMN rule_namespace DROP DEFAULT,
    ALTER COLUMN rule_name DROP DEFAULT;
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

        INSERT INTO service_rule_refs (service_ref, rule_id, role, rule_namespace, rule_name)
        VALUES
            (v_service_from_key, NEW.id, 'SERVICE_FROM', NEW.namespace, NEW.name),
            (v_service_to_key, NEW.id, 'SERVICE_TO', NEW.namespace, NEW.name);

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
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_service_xsvcsvc_rules()
RETURNS TRIGGER AS $$
DECLARE
    v_rule_ref JSONB;
    svc_namespace TEXT;
    svc_name TEXT;
    new_rv BIGINT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_rule_ref := jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'SvcSvcRule',
            'name', NEW.rule_name,
            'namespace', NEW.rule_namespace
        );

        svc_namespace := split_part(NEW.service_ref, '/', 1);
        svc_name := split_part(NEW.service_ref, '/', 2);

        IF svc_namespace = '' OR svc_name = '' THEN
            RETURN NEW;
        END IF;

        new_rv := bump_service_resource_version(svc_namespace, svc_name);
        IF new_rv IS NULL THEN
            RETURN NEW;
        END IF;

        IF NEW.role = 'SERVICE_FROM' THEN
            UPDATE services
            SET xsvcsvc_rules_as_from = xsvcsvc_rules_as_from || jsonb_build_array(v_rule_ref),
                resource_version = new_rv
            WHERE namespace = svc_namespace::namespace_name
              AND name = svc_name::resource_name;
        ELSIF NEW.role = 'SERVICE_TO' THEN
            UPDATE services
            SET xsvcsvc_rules_as_to = xsvcsvc_rules_as_to || jsonb_build_array(v_rule_ref),
                resource_version = new_rv
            WHERE namespace = svc_namespace::namespace_name
              AND name = svc_name::resource_name;
        END IF;

        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        v_rule_ref := jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'SvcSvcRule',
            'name', OLD.rule_name,
            'namespace', OLD.rule_namespace
        );

        svc_namespace := split_part(OLD.service_ref, '/', 1);
        svc_name := split_part(OLD.service_ref, '/', 2);

        IF svc_namespace = '' OR svc_name = '' THEN
            RETURN OLD;
        END IF;

        new_rv := bump_service_resource_version(svc_namespace, svc_name);
        IF new_rv IS NULL THEN
            RETURN OLD;
        END IF;

        IF OLD.role = 'SERVICE_FROM' THEN
            UPDATE services
            SET xsvcsvc_rules_as_from = (
                SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
                FROM jsonb_array_elements(xsvcsvc_rules_as_from) AS elem
                WHERE elem != v_rule_ref
            ),
                resource_version = new_rv
            WHERE namespace = svc_namespace::namespace_name
              AND name = svc_name::resource_name;
        ELSIF OLD.role = 'SERVICE_TO' THEN
            UPDATE services
            SET xsvcsvc_rules_as_to = (
                SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
                FROM jsonb_array_elements(xsvcsvc_rules_as_to) AS elem
                WHERE elem != v_rule_ref
            ),
                resource_version = new_rv
            WHERE namespace = svc_namespace::namespace_name
              AND name = svc_name::resource_name;
        END IF;

        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_service_xsvcsvc_rules()
RETURNS TRIGGER AS $$
DECLARE
    v_rule_ref JSONB;
    svc_namespace TEXT;
    svc_name TEXT;
    new_rv BIGINT;
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

        IF v_rule_ref IS NULL THEN
            RETURN NEW;
        END IF;

        svc_namespace := split_part(NEW.service_ref, '/', 1);
        svc_name := split_part(NEW.service_ref, '/', 2);

        IF svc_namespace = '' OR svc_name = '' THEN
            RETURN NEW;
        END IF;

        new_rv := bump_service_resource_version(svc_namespace, svc_name);
        IF new_rv IS NULL THEN
            RETURN NEW;
        END IF;

        IF NEW.role = 'SERVICE_FROM' THEN
            UPDATE services
            SET xsvcsvc_rules_as_from = xsvcsvc_rules_as_from || jsonb_build_array(v_rule_ref),
                resource_version = new_rv
            WHERE namespace = svc_namespace::namespace_name
              AND name = svc_name::resource_name;
        ELSIF NEW.role = 'SERVICE_TO' THEN
            UPDATE services
            SET xsvcsvc_rules_as_to = xsvcsvc_rules_as_to || jsonb_build_array(v_rule_ref),
                resource_version = new_rv
            WHERE namespace = svc_namespace::namespace_name
              AND name = svc_name::resource_name;
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

        IF v_rule_ref IS NULL THEN
            RETURN OLD;
        END IF;

        svc_namespace := split_part(OLD.service_ref, '/', 1);
        svc_name := split_part(OLD.service_ref, '/', 2);

        IF svc_namespace = '' OR svc_name = '' THEN
            RETURN OLD;
        END IF;

        new_rv := bump_service_resource_version(svc_namespace, svc_name);
        IF new_rv IS NULL THEN
            RETURN OLD;
        END IF;

        IF OLD.role = 'SERVICE_FROM' THEN
            UPDATE services
            SET xsvcsvc_rules_as_from = (
                SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
                FROM jsonb_array_elements(xsvcsvc_rules_as_from) AS elem
                WHERE elem != v_rule_ref
            ),
                resource_version = new_rv
            WHERE namespace = svc_namespace::namespace_name
              AND name = svc_name::resource_name;
        ELSIF OLD.role = 'SERVICE_TO' THEN
            UPDATE services
            SET xsvcsvc_rules_as_to = (
                SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
                FROM jsonb_array_elements(xsvcsvc_rules_as_to) AS elem
                WHERE elem != v_rule_ref
            ),
                resource_version = new_rv
            WHERE namespace = svc_namespace::namespace_name
              AND name = svc_name::resource_name;
        END IF;

        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
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
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE service_rule_refs
    DROP COLUMN IF EXISTS rule_namespace,
    DROP COLUMN IF EXISTS rule_name;
-- +goose StatementEnd
