-- +goose Up
-- +goose StatementBegin

ALTER TABLE services ADD COLUMN address_groups JSONB NOT NULL DEFAULT '[]';
ALTER TABLE services ADD COLUMN aggregated_address_groups JSONB NOT NULL DEFAULT '[]';

CREATE INDEX idx_services_address_groups ON services USING GIN(address_groups);
CREATE INDEX idx_services_aggregated_address_groups ON services USING GIN(aggregated_address_groups);

ALTER TABLE services ADD CONSTRAINT chk_services_address_groups_is_array
    CHECK (jsonb_typeof(address_groups) = 'array');
ALTER TABLE services ADD CONSTRAINT chk_services_aggregated_address_groups_is_array
    CHECK (jsonb_typeof(aggregated_address_groups) = 'array');

CREATE OR REPLACE FUNCTION aggregate_service_address_groups(svc_namespace TEXT, svc_name TEXT) RETURNS JSONB AS $$
DECLARE
    aggregated_groups_json JSONB := '[]'::jsonb;
    group_ref JSONB;
    binding_record RECORD;
    address_groups_field JSONB;
BEGIN
    SELECT COALESCE(address_groups, '[]'::jsonb) INTO address_groups_field
    FROM services
    WHERE namespace = svc_namespace AND name = svc_name;

    IF address_groups_field IS NOT NULL AND address_groups_field != 'null'::jsonb
       AND jsonb_typeof(address_groups_field) = 'array' AND jsonb_array_length(address_groups_field) > 0 THEN
        FOR group_ref IN
            SELECT value FROM jsonb_array_elements(address_groups_field) AS value
        LOOP
            aggregated_groups_json := aggregated_groups_json || jsonb_build_array(
                jsonb_build_object(
                    'ref', group_ref.value,
                    'source', 'spec'
                )
            );
        END LOOP;
    END IF;

    FOR binding_record IN
        SELECT agb.address_group_namespace, agb.address_group_name
        FROM address_group_bindings agb
        WHERE agb.service_namespace = svc_namespace::namespace_name
        AND agb.service_name = svc_name::resource_name
    LOOP
        aggregated_groups_json := aggregated_groups_json || jsonb_build_array(
            jsonb_build_object(
                'ref', jsonb_build_object(
                    'apiVersion', 'netguard.sgroups.io/v1beta1',
                    'kind', 'AddressGroup',
                    'name', binding_record.address_group_name,
                    'namespace', binding_record.address_group_namespace
                ),
                'source', 'binding'
            )
        );
    END LOOP;

    RETURN aggregated_groups_json;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_aggregated_address_groups_for_service(svc_namespace TEXT, svc_name TEXT) RETURNS VOID AS $$
BEGIN
    UPDATE services
    SET aggregated_address_groups = aggregate_service_address_groups(svc_namespace, svc_name)
    WHERE namespace = svc_namespace::namespace_name AND name = svc_name::resource_name;

    RAISE NOTICE 'Updated aggregated_address_groups for Service %.%', svc_namespace, svc_name;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION validate_service_address_group_conflicts() RETURNS TRIGGER AS $$
DECLARE
    conflicting_count INTEGER;
    spec_groups JSONB;
    binding_groups JSONB;
    spec_group JSONB;
    binding_group JSONB;
    service_record RECORD;
BEGIN
    IF TG_TABLE_NAME = 'address_group_bindings' THEN
        SELECT namespace, name, address_groups INTO service_record
        FROM services
        WHERE namespace = NEW.service_namespace AND name = NEW.service_name;

        IF NOT FOUND THEN
            RAISE EXCEPTION 'Service %.% does not exist', NEW.service_namespace, NEW.service_name;
        END IF;

        spec_groups := COALESCE(service_record.address_groups, '[]'::jsonb);

        FOR spec_group IN
            SELECT value FROM jsonb_array_elements(spec_groups) AS value
        LOOP
            IF (spec_group.value->>'name') = NEW.address_group_name
               AND COALESCE(spec_group.value->>'namespace', NEW.service_namespace) = NEW.address_group_namespace THEN
                RAISE EXCEPTION 'AddressGroup %.% is already referenced by Service %.% via spec.addressGroups - cannot create AddressGroupBinding for dual registration conflict',
                    NEW.address_group_namespace, NEW.address_group_name,
                    NEW.service_namespace, NEW.service_name;
            END IF;
        END LOOP;
    END IF;

    IF TG_TABLE_NAME = 'services' THEN
        spec_groups := COALESCE(NEW.address_groups, '[]'::jsonb);

        FOR spec_group IN
            SELECT value FROM jsonb_array_elements(spec_groups) AS value
        LOOP
            SELECT COUNT(*) INTO conflicting_count
            FROM address_group_bindings agb
            WHERE agb.service_namespace = NEW.namespace
            AND agb.service_name = NEW.name
            AND agb.address_group_name = (spec_group.value->>'name')::resource_name
            AND agb.address_group_namespace = COALESCE((spec_group.value->>'namespace')::namespace_name, NEW.namespace);

            IF conflicting_count > 0 THEN
                RAISE EXCEPTION 'AddressGroup %.% is already bound to Service %.% via AddressGroupBinding - cannot add to spec.addressGroups for dual registration conflict',
                    COALESCE(spec_group.value->>'namespace', NEW.namespace), spec_group.value->>'name',
                    NEW.namespace, NEW.name;
            END IF;
        END LOOP;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION trigger_update_aggregated_address_groups_on_spec_change() RETURNS TRIGGER AS $$
BEGIN
    UPDATE services
    SET aggregated_address_groups = aggregate_service_address_groups(NEW.namespace, NEW.name)
    WHERE namespace = NEW.namespace AND name = NEW.name;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_aggregated_address_groups_on_spec_change
    AFTER INSERT OR UPDATE OF address_groups ON services
    FOR EACH ROW
    EXECUTE FUNCTION trigger_update_aggregated_address_groups_on_spec_change();

CREATE TRIGGER validate_service_address_group_conflicts_trigger
    BEFORE INSERT OR UPDATE OF address_groups ON services
    FOR EACH ROW
    EXECUTE FUNCTION validate_service_address_group_conflicts();

CREATE TRIGGER validate_address_group_binding_conflicts_trigger
    BEFORE INSERT OR UPDATE ON address_group_bindings
    FOR EACH ROW
    EXECUTE FUNCTION validate_service_address_group_conflicts();

CREATE OR REPLACE FUNCTION trigger_update_aggregated_address_groups_on_binding_change() RETURNS TRIGGER AS $$
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

        -- If UPDATE changed the Service reference, also update the old one
        IF TG_OP = 'UPDATE' AND (OLD.service_namespace != NEW.service_namespace OR OLD.service_name != NEW.service_name) THEN
            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(OLD.service_namespace, OLD.service_name)
            WHERE namespace = OLD.service_namespace AND name = OLD.service_name;
        END IF;

        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_aggregated_address_groups_on_binding_change_trigger
    AFTER INSERT OR UPDATE OR DELETE ON address_group_bindings
    FOR EACH ROW
    EXECUTE FUNCTION trigger_update_aggregated_address_groups_on_binding_change();

UPDATE services
SET aggregated_address_groups = aggregate_service_address_groups(namespace, name);

COMMENT ON COLUMN services.address_groups IS 'NamespacedObjectReference[] - AddressGroups directly referenced in Service.spec.addressGroups';
COMMENT ON COLUMN services.aggregated_address_groups IS 'AddressGroupReference[] - Aggregated list of all AddressGroups from both spec.addressGroups and AddressGroupBindings with source tracking';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop triggers
DROP TRIGGER IF EXISTS update_aggregated_address_groups_on_spec_change ON services;
DROP TRIGGER IF EXISTS validate_service_address_group_conflicts_trigger ON services;
DROP TRIGGER IF EXISTS validate_address_group_binding_conflicts_trigger ON address_group_bindings;
DROP TRIGGER IF EXISTS update_aggregated_address_groups_on_binding_change_trigger ON address_group_bindings;

-- Drop functions
DROP FUNCTION IF EXISTS trigger_update_aggregated_address_groups_on_binding_change();
DROP FUNCTION IF EXISTS trigger_update_aggregated_address_groups_on_spec_change();
DROP FUNCTION IF EXISTS validate_service_address_group_conflicts();
DROP FUNCTION IF EXISTS update_aggregated_address_groups_for_service(TEXT, TEXT);
DROP FUNCTION IF EXISTS aggregate_service_address_groups(TEXT, TEXT);

-- Drop constraints
ALTER TABLE services DROP CONSTRAINT IF EXISTS chk_services_address_groups_is_array;
ALTER TABLE services DROP CONSTRAINT IF EXISTS chk_services_aggregated_address_groups_is_array;

-- Drop indexes
DROP INDEX IF EXISTS idx_services_address_groups;
DROP INDEX IF EXISTS idx_services_aggregated_address_groups;

-- Remove new columns
ALTER TABLE services DROP COLUMN IF EXISTS address_groups;
ALTER TABLE services DROP COLUMN IF EXISTS aggregated_address_groups;

-- +goose StatementEnd