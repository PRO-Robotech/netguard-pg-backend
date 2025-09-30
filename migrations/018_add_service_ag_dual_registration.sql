-- +goose Up
-- Add dual registration support for AddressGroups in Service
-- Following the pattern from Host → AddressGroup dual registration (migrations 012, 014)

-- +goose StatementBegin

-- =============================================================================
-- STEP 1: Add fields to services table
-- =============================================================================

-- Add address_groups JSONB field to services table for direct AddressGroup registration
ALTER TABLE services ADD COLUMN address_groups JSONB NOT NULL DEFAULT '[]';

-- Add aggregated_address_groups JSONB field for combined view (spec + bindings)
ALTER TABLE services ADD COLUMN aggregated_address_groups JSONB NOT NULL DEFAULT '[]';

-- Create indexes for efficient searching
CREATE INDEX idx_services_address_groups ON services USING GIN(address_groups);
CREATE INDEX idx_services_aggregated_address_groups ON services USING GIN(aggregated_address_groups);

-- Add comments
COMMENT ON COLUMN services.address_groups IS 'NamespacedObjectReference[] - List of AddressGroups that belong exclusively to this Service (direct registration)';
COMMENT ON COLUMN services.aggregated_address_groups IS 'AddressGroupReference[] - Aggregated list of all AddressGroups from both spec.addressGroups and AddressGroupBindings with source tracking';

-- =============================================================================
-- STEP 2: Aggregation function
-- =============================================================================

-- Function to aggregate AddressGroups from both spec.address_groups and AddressGroupBindings
-- Returns JSONB array with AddressGroupReference objects
CREATE OR REPLACE FUNCTION aggregate_service_address_groups(
    svc_namespace TEXT,
    svc_name TEXT
) RETURNS JSONB AS $$
DECLARE
    aggregated_ags_json JSONB := '[]'::jsonb;
    ag_ref JSONB;
    ag_record RECORD;
    address_groups_field JSONB;
BEGIN
    -- Get address_groups field, handling null case
    SELECT COALESCE(address_groups, '[]'::jsonb) INTO address_groups_field
    FROM services
    WHERE namespace = svc_namespace AND name = svc_name;

    -- Source 1: Collect AddressGroups from spec.address_groups (source = 'spec')
    IF address_groups_field IS NOT NULL AND address_groups_field != 'null'::jsonb
       AND jsonb_typeof(address_groups_field) = 'array' AND jsonb_array_length(address_groups_field) > 0 THEN
        FOR ag_ref IN
            SELECT jsonb_array_elements(address_groups_field) as ag_obj
        LOOP
            -- Add AddressGroup reference with source information
            aggregated_ags_json := aggregated_ags_json || jsonb_build_array(
                jsonb_build_object(
                    'ref', ag_ref,
                    'source', 'spec'
                )
            );
        END LOOP;
    END IF;

    -- Source 2: Collect AddressGroups from AddressGroupBindings (source = 'binding')
    FOR ag_record IN
        SELECT ag.namespace, ag.name
        FROM address_group_bindings agb
        JOIN address_groups ag ON ag.namespace = agb.address_group_namespace
                              AND ag.name = agb.address_group_name
        WHERE agb.service_namespace = svc_namespace::namespace_name
        AND agb.service_name = svc_name::resource_name
    LOOP
        -- Add AddressGroup reference with source information
        aggregated_ags_json := aggregated_ags_json || jsonb_build_array(
            jsonb_build_object(
                'ref', jsonb_build_object(
                    'apiVersion', 'netguard.sgroups.io/v1beta1',
                    'kind', 'AddressGroup',
                    'name', ag_record.name,
                    'namespace', ag_record.namespace
                ),
                'source', 'binding'
            )
        );
    END LOOP;

    RETURN aggregated_ags_json;
END;
$$ LANGUAGE plpgsql;

-- Helper function to update aggregated_address_groups for specific Service
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

-- =============================================================================
-- STEP 3: Validation function to prevent AddressGroup conflicts
-- =============================================================================

-- Function to validate AddressGroupBinding doesn't conflict with existing bindings or spec.address_groups
CREATE OR REPLACE FUNCTION validate_service_ag_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    conflicting_service RECORD;
    ag_in_spec BOOLEAN := false;
BEGIN
    -- Check if AddressGroup is already in spec.address_groups of ANY Service (including the target one)
    SELECT EXISTS(
        SELECT 1 FROM services s
        WHERE s.address_groups @> jsonb_build_array(
            jsonb_build_object(
                'apiVersion', 'netguard.sgroups.io/v1beta1',
                'kind', 'AddressGroup',
                'name', NEW.address_group_name,
                'namespace', NEW.address_group_namespace
            )
        )
    ) INTO ag_in_spec;

    IF ag_in_spec THEN
        -- Find which Service contains this AddressGroup in spec.address_groups
        SELECT s.namespace, s.name INTO conflicting_service
        FROM services s
        WHERE s.address_groups @> jsonb_build_array(
            jsonb_build_object(
                'apiVersion', 'netguard.sgroups.io/v1beta1',
                'kind', 'AddressGroup',
                'name', NEW.address_group_name,
                'namespace', NEW.address_group_namespace
            )
        )
        LIMIT 1;

        RAISE EXCEPTION 'AddressGroup %.% already belongs to Service %.% via spec.addressGroups - cannot create AddressGroupBinding',
            NEW.address_group_namespace, NEW.address_group_name,
            conflicting_service.namespace, conflicting_service.name;
    END IF;

    -- Check if AddressGroup is already bound to a different Service via AddressGroupBinding
    SELECT s.namespace, s.name INTO conflicting_service
    FROM address_group_bindings agb
    JOIN services s ON s.namespace = agb.service_namespace AND s.name = agb.service_name
    WHERE agb.address_group_namespace = NEW.address_group_namespace
    AND agb.address_group_name = NEW.address_group_name
    AND (agb.service_namespace != NEW.service_namespace OR agb.service_name != NEW.service_name)
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'AddressGroup %.% already belongs to Service %.% via AddressGroupBinding - each AddressGroup can belong to only one Service',
            NEW.address_group_namespace, NEW.address_group_name,
            conflicting_service.namespace, conflicting_service.name;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- STEP 4: Trigger functions for automatic aggregation
-- =============================================================================

-- Trigger function to update aggregated_address_groups when spec.address_groups changes
CREATE OR REPLACE FUNCTION trigger_update_aggregated_ags_on_spec_change() RETURNS TRIGGER AS $$
BEGIN
    -- Update aggregated_address_groups with separate UPDATE statement after the main operation
    UPDATE services
    SET aggregated_address_groups = aggregate_service_address_groups(NEW.namespace, NEW.name)
    WHERE namespace = NEW.namespace AND name = NEW.name;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger function to update aggregated_address_groups when AddressGroupBinding changes
CREATE OR REPLACE FUNCTION trigger_update_aggregated_ags_on_binding_change() RETURNS TRIGGER AS $$
BEGIN
    -- Update aggregated_address_groups for the affected Service(s)
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

-- =============================================================================
-- STEP 5: Create triggers
-- =============================================================================

-- Trigger 1: Update aggregated_address_groups when spec.address_groups changes
CREATE TRIGGER update_aggregated_ags_on_spec_change
    AFTER INSERT OR UPDATE OF address_groups ON services
    FOR EACH ROW
    EXECUTE FUNCTION trigger_update_aggregated_ags_on_spec_change();

-- Trigger 2: Validate conflicts before AddressGroupBinding creation
CREATE TRIGGER validate_service_ag_binding_conflicts_trigger
    BEFORE INSERT OR UPDATE ON address_group_bindings
    FOR EACH ROW
    EXECUTE FUNCTION validate_service_ag_binding_conflicts();

-- Trigger 3: Update aggregated_address_groups when AddressGroupBinding changes
CREATE TRIGGER update_aggregated_ags_on_binding_change_trigger
    AFTER INSERT OR UPDATE OR DELETE ON address_group_bindings
    FOR EACH ROW
    EXECUTE FUNCTION trigger_update_aggregated_ags_on_binding_change();

-- =============================================================================
-- STEP 6: Initialize aggregated_address_groups for existing Services
-- =============================================================================

-- Populate aggregated_address_groups for all existing services
UPDATE services
SET aggregated_address_groups = aggregate_service_address_groups(namespace, name);

-- +goose StatementEnd

-- +goose Down
-- Remove Service → AddressGroup dual registration support

-- +goose StatementBegin

-- Drop triggers
DROP TRIGGER IF EXISTS update_aggregated_ags_on_spec_change ON services;
DROP TRIGGER IF EXISTS validate_service_ag_binding_conflicts_trigger ON address_group_bindings;
DROP TRIGGER IF EXISTS update_aggregated_ags_on_binding_change_trigger ON address_group_bindings;

-- Drop functions
DROP FUNCTION IF EXISTS trigger_update_aggregated_ags_on_binding_change();
DROP FUNCTION IF EXISTS trigger_update_aggregated_ags_on_spec_change();
DROP FUNCTION IF EXISTS validate_service_ag_binding_conflicts();
DROP FUNCTION IF EXISTS update_aggregated_ags_for_service(TEXT, TEXT);
DROP FUNCTION IF EXISTS aggregate_service_address_groups(TEXT, TEXT);

-- Drop indexes
DROP INDEX IF EXISTS idx_services_aggregated_address_groups;
DROP INDEX IF EXISTS idx_services_address_groups;

-- Remove columns
ALTER TABLE services DROP COLUMN IF EXISTS aggregated_address_groups;
ALTER TABLE services DROP COLUMN IF EXISTS address_groups;

-- +goose StatementEnd
