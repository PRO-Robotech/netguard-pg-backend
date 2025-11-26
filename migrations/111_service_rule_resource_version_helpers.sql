-- +goose Up

-- +goose StatementBegin
-- Helper that bumps Service resourceVersion and returns the new value.
CREATE OR REPLACE FUNCTION bump_service_resource_version(svc_namespace TEXT, svc_name TEXT)
RETURNS BIGINT AS $$
DECLARE
    current_rv BIGINT;
    new_rv BIGINT;
BEGIN
    SELECT resource_version
    INTO current_rv
    FROM services
    WHERE namespace = svc_namespace::namespace_name
      AND name = svc_name::resource_name
    FOR UPDATE;

    IF current_rv IS NULL THEN
        RETURN NULL;
    END IF;

    UPDATE k8s_metadata
    SET updated_at = NOW()
    WHERE resource_version = current_rv
    RETURNING resource_version INTO new_rv;

    RETURN new_rv;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- Ensure aggregated address groups always bump Service resourceVersion.
CREATE OR REPLACE FUNCTION update_aggregated_ags_for_service(
    svc_namespace TEXT,
    svc_name TEXT
) RETURNS VOID AS $$
DECLARE
    new_rv BIGINT;
BEGIN
    new_rv := bump_service_resource_version(svc_namespace, svc_name);

    IF new_rv IS NULL THEN
        RETURN;
    END IF;

    UPDATE services
    SET aggregated_address_groups = aggregate_service_address_groups(svc_namespace, svc_name),
        resource_version = new_rv
    WHERE namespace = svc_namespace::namespace_name
      AND name = svc_name::resource_name;

    RAISE NOTICE 'Updated aggregated_address_groups for Service %.%', svc_namespace, svc_name;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_update_aggregated_ags_on_spec_change()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM update_aggregated_ags_for_service(NEW.namespace, NEW.name);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_update_aggregated_ags_on_binding_change()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM update_aggregated_ags_for_service(OLD.service_namespace, OLD.service_name);
        RETURN OLD;
    END IF;

    PERFORM update_aggregated_ags_for_service(NEW.service_namespace, NEW.service_name);

    IF TG_OP = 'UPDATE' AND (OLD.service_namespace != NEW.service_namespace OR OLD.service_name != NEW.service_name) THEN
        PERFORM update_aggregated_ags_for_service(OLD.service_namespace, OLD.service_name);
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- Keep Service resourceVersion in sync when service_rule_refs change.
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
-- Keep Service resourceVersion in sync when service_fqdn_rule_refs change.
CREATE OR REPLACE FUNCTION update_service_xsvc_fqdn_rules()
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
            'kind', 'SvcFqdnRule',
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

        UPDATE services
        SET xsvc_fqdn_rules = xsvc_fqdn_rules || jsonb_build_array(v_rule_ref),
            resource_version = new_rv
        WHERE namespace = svc_namespace::namespace_name
          AND name = svc_name::resource_name;

        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        v_rule_ref := jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'SvcFqdnRule',
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

        UPDATE services
        SET xsvc_fqdn_rules = (
                SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
                FROM jsonb_array_elements(xsvc_fqdn_rules) AS elem
                WHERE elem != v_rule_ref
            ),
            resource_version = new_rv
        WHERE namespace = svc_namespace::namespace_name
          AND name = svc_name::resource_name;

        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down

DROP FUNCTION IF EXISTS bump_service_resource_version(TEXT, TEXT);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_aggregated_ags_for_service(
    svc_namespace TEXT,
    svc_name TEXT
) RETURNS VOID AS $$
BEGIN
    UPDATE services
    SET aggregated_address_groups = aggregate_service_address_groups(svc_namespace, svc_name)
    WHERE namespace = svc_namespace::namespace_name AND name = svc_name::resource_name;

    RAISE NOTICE 'Updated aggregated_address_groups for Service %.%', svc_namespace, svc_name;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_update_aggregated_ags_on_spec_change() RETURNS TRIGGER AS $$
BEGIN
    UPDATE services
    SET aggregated_address_groups = aggregate_service_address_groups(NEW.namespace, NEW.name)
    WHERE namespace = NEW.namespace AND name = NEW.name;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_update_aggregated_ags_on_binding_change() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        UPDATE services
        SET aggregated_address_groups = aggregate_service_address_groups(OLD.service_namespace, OLD.service_name)
        WHERE namespace = OLD.service_namespace AND name = OLD.service_name;
        RETURN OLD;
    ELSE
        UPDATE services
        SET aggregated_address_groups = aggregate_service_address_groups(NEW.service_namespace, NEW.service_name)
        WHERE namespace = NEW.service_namespace AND name = NEW.service_name;

        IF TG_OP = 'UPDATE' AND (OLD.service_namespace != NEW.service_namespace OR OLD.service_name != NEW.service_name) THEN
            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(OLD.service_namespace, OLD.service_name)
            WHERE namespace = OLD.service_namespace AND name = OLD.service_name;
        END IF;

        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;
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

