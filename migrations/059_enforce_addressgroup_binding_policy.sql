-- +goose Up
-- +goose StatementBegin

-- =====================================================
-- Migration 059: Enforce AddressGroupBindingPolicy
-- =====================================================
-- Purpose: Enforce cross-namespace binding policy for AddressGroupBinding
-- Pattern: Similar to Ready validation (check conditions before allowing operation)
--
-- Problem:
--   AddressGroupBindingPolicy resource exists but is NOT enforced.
--   Current validation (Migration 058) allows all bindings without policy check.
--   Cross-namespace bindings should require explicit policy.
--
-- Solution:
--   Add validation function to check AddressGroupBindingPolicy:
--   - Same namespace: Always allowed (no policy required)
--   - Cross-namespace: Requires AddressGroupBindingPolicy exists
--
-- Policy Format (from types.go):
--   AddressGroupBindingPolicySpec {
--     addressGroupRef: { namespace, name }
--     serviceRef: { namespace, name }
--   }
--
-- Validation Logic:
--   1. If Service.namespace == AddressGroup.namespace → allow (same namespace)
--   2. If Service.namespace != AddressGroup.namespace → require policy
--   3. Policy must exist with matching refs in EITHER direction:
--      - addressGroupRef → AG, serviceRef → Service
--      - addressGroupRef → Service AG, serviceRef → current Service (for spec.addressGroups)
--
-- Related Migrations:
--   - Migration 058: Removed 1:1 uniqueness (prerequisite)
--   - Migration 060: Port conflict detection (next)
--
-- Date: 2025-10-31
-- =====================================================

-- Update validation function: Add policy enforcement
CREATE OR REPLACE FUNCTION validate_service_ag_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    duplicate_binding BOOLEAN := false;
    requires_policy BOOLEAN := false;
    policy_exists BOOLEAN := false;
BEGIN
    -- Check 1: Prevent duplicate binding (same Service + same AG)
    SELECT EXISTS(
        SELECT 1 FROM address_group_bindings agb
        WHERE agb.service_namespace = NEW.service_namespace
          AND agb.service_name = NEW.service_name
          AND agb.address_group_namespace = NEW.address_group_namespace
          AND agb.address_group_name = NEW.address_group_name
          AND (TG_OP = 'INSERT' OR (agb.namespace != NEW.namespace OR agb.name != NEW.name))
    ) INTO duplicate_binding;

    IF duplicate_binding THEN
        RAISE EXCEPTION 'AddressGroupBinding already exists for Service %.% → AddressGroup %.%',
            NEW.service_namespace, NEW.service_name,
            NEW.address_group_namespace, NEW.address_group_name;
    END IF;

    -- Check 2: Cross-namespace policy enforcement
    -- If Service and AddressGroup are in DIFFERENT namespaces, require policy
    IF NEW.service_namespace != NEW.address_group_namespace THEN
        requires_policy := true;

        -- Check if AddressGroupBindingPolicy exists for this combination
        SELECT EXISTS(
            SELECT 1 FROM address_group_binding_policies agbp
            JOIN k8s_metadata m ON m.resource_version = agbp.resource_version
            WHERE m.deletion_timestamp IS NULL  -- Not marked for deletion
              AND (
                  -- Policy direction 1: AG → Service
                  (
                      agbp.policy_data->>'addressGroupNamespace' = NEW.address_group_namespace
                      AND agbp.policy_data->>'addressGroupName' = NEW.address_group_name
                      AND agbp.policy_data->>'serviceNamespace' = NEW.service_namespace
                      AND agbp.policy_data->>'serviceName' = NEW.service_name
                  )
                  -- Policy direction 2: Service → AG (for spec.addressGroups case)
                  OR (
                      agbp.policy_data->>'addressGroupNamespace' = NEW.service_namespace
                      AND agbp.policy_data->>'addressGroupName' = NEW.service_name
                      AND agbp.policy_data->>'serviceNamespace' = NEW.address_group_namespace
                      AND agbp.policy_data->>'serviceName' = NEW.address_group_name
                  )
              )
        ) INTO policy_exists;

        IF NOT policy_exists THEN
            RAISE EXCEPTION 'Cross-namespace binding requires AddressGroupBindingPolicy: Service % cannot bind to AddressGroup %. Create AddressGroupBindingPolicy to allow this binding.',
                NEW.service_namespace || '/' || NEW.service_name,
                NEW.address_group_namespace || '/' || NEW.address_group_name;
        END IF;

        RAISE NOTICE '[Migration 059] Cross-namespace binding allowed by policy: Service % -> AddressGroup %',
            NEW.service_namespace || '/' || NEW.service_name,
            NEW.address_group_namespace || '/' || NEW.address_group_name;
    END IF;

    -- Port conflict detection will be added in Migration 060

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to Migration 058 validation (no policy enforcement)
CREATE OR REPLACE FUNCTION validate_service_ag_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    duplicate_binding BOOLEAN := false;
BEGIN
    -- Only check for duplicate binding (same Service + same AG)
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

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
