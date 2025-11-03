-- +goose Up
-- +goose StatementBegin

-- =====================================================
-- Migration 058: Remove 1:1 Uniqueness Validation, Allow 1:N
-- =====================================================
-- Purpose: Allow multiple Services to use the same AddressGroup
-- Pattern: Modify existing validation function from Migration 018
--
-- Problem:
--   Current validation enforces 1:1 relationship:
--   - Check if AG is in ANY Service spec.address_groups → block
--   - Check if AG is in ANY AddressGroupBinding → block
--   Result: Only ONE Service can use an AddressGroup.
--
-- User Requirement:
--   1:N relationship (multiple Services → one AddressGroup)
--
-- Solution:
--   Simplify validation to ONLY check cross-namespace policy enforcement.
--   Remove uniqueness checks entirely.
--   Cross-namespace policy will be enforced in Migration 059.
--
-- New Validation Logic:
--   1. Allow duplicate AG across multiple Services (1:N) ✓
--   2. Still allow Service.spec.addressGroups + AddressGroupBinding ✓
--   3. Policy enforcement moved to Migration 059
--   4. Port conflict detection moved to Migration 060
--
-- Related Migrations:
--   - Migration 018: Original validation (enforced 1:1)
--   - Migration 059: Cross-namespace policy enforcement (next)
--   - Migration 060: Port conflict detection (next)
--
-- Date: 2025-10-31
-- =====================================================

-- Replace validation function: Remove ALL uniqueness checks
-- Now only validates that the binding itself is valid (refs exist, not duplicate)
CREATE OR REPLACE FUNCTION validate_service_ag_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    duplicate_binding BOOLEAN := false;
BEGIN
    -- REMOVED: Check if AG is in ANY Service spec.address_groups (allowed now - 1:N)
    -- REMOVED: Check if AG is bound to ANY other Service (allowed now - 1:N)

    -- NEW: Only check for duplicate binding (same Service + same AG)
    -- This is the ONLY validation we keep: prevent exact duplicate bindings
    SELECT EXISTS(
        SELECT 1 FROM address_group_bindings agb
        WHERE agb.service_namespace = NEW.service_namespace
          AND agb.service_name = NEW.service_name
          AND agb.address_group_namespace = NEW.address_group_namespace
          AND agb.address_group_name = NEW.address_group_name
          AND (TG_OP = 'INSERT' OR agb.namespace != NEW.namespace OR agb.name != NEW.name)
    ) INTO duplicate_binding;

    IF duplicate_binding THEN
        RAISE EXCEPTION 'AddressGroupBinding already exists for Service %.% → AddressGroup %.%',
            NEW.service_namespace, NEW.service_name,
            NEW.address_group_namespace, NEW.address_group_name;
    END IF;

    -- Cross-namespace policy enforcement will be added in Migration 059
    -- Port conflict detection will be added in Migration 060

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Restore original 1:1 uniqueness validation from Migration 018
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

-- +goose StatementEnd
