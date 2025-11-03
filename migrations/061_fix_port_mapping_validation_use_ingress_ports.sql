-- +goose Up
-- +goose StatementBegin

-- =====================================================
-- Migration 061: Fix Port Mapping Validation - Use ingress_ports Column
-- =====================================================
-- Purpose: Fix SQL error in Migration 060 port conflict detection
-- Problem: Function references non-existent "spec" column instead of "ingress_ports"
-- Error: "ERROR: column "spec" does not exist (SQLSTATE 42703)"
--
-- Root Cause:
--   Migration 060 line 134, 141, 147 incorrectly reference:
--     SELECT spec->'ingressPorts' FROM services
--     s.spec->'ingressPorts'
--
--   But services table has column "ingress_ports" (JSONB), NOT "spec"
--
-- Solution:
--   Replace all "spec->'ingressPorts'" with "ingress_ports" (direct JSONB column access)
--
-- Date: 2025-10-31
-- =====================================================

-- Update validation function: Fix spec -> ingress_ports
CREATE OR REPLACE FUNCTION validate_service_ag_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    duplicate_binding BOOLEAN := false;
    requires_policy BOOLEAN := false;
    policy_exists BOOLEAN := false;
    conflicting_service RECORD;
    new_service_ports JSONB;
    other_service_ports JSONB;
    new_port JSONB;
    other_port JSONB;
    new_port_start INTEGER;
    new_port_end INTEGER;
    other_port_start INTEGER;
    other_port_end INTEGER;
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
        RAISE EXCEPTION 'AddressGroupBinding already exists for Service % -> AddressGroup %',
            NEW.service_namespace || '/' || NEW.service_name,
            NEW.address_group_namespace || '/' || NEW.address_group_name;
    END IF;

    -- Check 2: Cross-namespace policy enforcement (from Migration 059)
    IF NEW.service_namespace != NEW.address_group_namespace THEN
        requires_policy := true;

        SELECT EXISTS(
            SELECT 1 FROM address_group_binding_policies agbp
            JOIN k8s_metadata m ON m.resource_version = agbp.resource_version
            WHERE m.deletion_timestamp IS NULL
              AND (
                  (
                      agbp.policy_data->>'addressGroupNamespace' = NEW.address_group_namespace
                      AND agbp.policy_data->>'addressGroupName' = NEW.address_group_name
                      AND agbp.policy_data->>'serviceNamespace' = NEW.service_namespace
                      AND agbp.policy_data->>'serviceName' = NEW.service_name
                  )
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
    END IF;

    -- Check 3: Port conflict detection (FIXED: spec -> ingress_ports)
    -- Get ingress ports for the new Service (FIXED: Use ingress_ports column directly)
    SELECT ingress_ports INTO new_service_ports
    FROM services
    WHERE namespace = NEW.service_namespace AND name = NEW.service_name;

    IF new_service_ports IS NOT NULL AND jsonb_array_length(new_service_ports) > 0 THEN
        -- Check all OTHER Services bound to the SAME AddressGroup (FIXED: Use s.ingress_ports)
        FOR conflicting_service IN
            SELECT s.namespace, s.name, s.ingress_ports AS ingress_ports
            FROM address_group_bindings agb
            JOIN services s ON s.namespace = agb.service_namespace AND s.name = agb.service_name
            WHERE agb.address_group_namespace = NEW.address_group_namespace
              AND agb.address_group_name = NEW.address_group_name
              AND (agb.service_namespace != NEW.service_namespace OR agb.service_name != NEW.service_name)
              AND s.ingress_ports IS NOT NULL
        LOOP
            other_service_ports := conflicting_service.ingress_ports;

            -- Compare each port in new Service with each port in existing Service
            FOR new_port IN SELECT * FROM jsonb_array_elements(new_service_ports)
            LOOP
                FOR other_port IN SELECT * FROM jsonb_array_elements(other_service_ports)
                LOOP
                    -- Only check if protocols match (TCP/UDP are separate namespaces)
                    IF new_port->>'protocol' = other_port->>'protocol' THEN
                        -- Parse port ranges
                        SELECT * INTO new_port_start, new_port_end
                        FROM parse_port_range(new_port->>'port');

                        SELECT * INTO other_port_start, other_port_end
                        FROM parse_port_range(other_port->>'port');

                        -- Check for overlap
                        IF ports_overlap(new_port_start, new_port_end, other_port_start, other_port_end) THEN
                            RAISE EXCEPTION 'Port conflict detected: Service % port %/% overlaps with Service % port %/% in AddressGroup %. Multiple Services using the same AddressGroup cannot have overlapping ports on the same protocol.',
                                NEW.service_namespace || '/' || NEW.service_name,
                                new_port->>'protocol', new_port->>'port',
                                conflicting_service.namespace || '/' || conflicting_service.name,
                                other_port->>'protocol', other_port->>'port',
                                NEW.address_group_namespace || '/' || NEW.address_group_name;
                        END IF;
                    END IF;
                END LOOP;
            END LOOP;
        END LOOP;

        RAISE NOTICE '[Migration 061] Port conflict check passed for Service % -> AddressGroup %',
            NEW.service_namespace || '/' || NEW.service_name,
            NEW.address_group_namespace || '/' || NEW.address_group_name;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to Migration 060 version (with bug)
CREATE OR REPLACE FUNCTION validate_service_ag_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    duplicate_binding BOOLEAN := false;
    requires_policy BOOLEAN := false;
    policy_exists BOOLEAN := false;
    conflicting_service RECORD;
    new_service_ports JSONB;
    other_service_ports JSONB;
    new_port JSONB;
    other_port JSONB;
    new_port_start INTEGER;
    new_port_end INTEGER;
    other_port_start INTEGER;
    other_port_end INTEGER;
BEGIN
    -- Check 1: Prevent duplicate binding
    SELECT EXISTS(
        SELECT 1 FROM address_group_bindings agb
        WHERE agb.service_namespace = NEW.service_namespace
          AND agb.service_name = NEW.service_name
          AND agb.address_group_namespace = NEW.address_group_namespace
          AND agb.address_group_name = NEW.address_group_name
          AND (TG_OP = 'INSERT' OR (agb.namespace != NEW.namespace OR agb.name != NEW.name))
    ) INTO duplicate_binding;

    IF duplicate_binding THEN
        RAISE EXCEPTION 'AddressGroupBinding already exists for Service % -> AddressGroup %',
            NEW.service_namespace || '/' || NEW.service_name,
            NEW.address_group_namespace || '/' || NEW.address_group_name;
    END IF;

    -- Check 2: Cross-namespace policy enforcement
    IF NEW.service_namespace != NEW.address_group_namespace THEN
        requires_policy := true;

        SELECT EXISTS(
            SELECT 1 FROM address_group_binding_policies agbp
            JOIN k8s_metadata m ON m.resource_version = agbp.resource_version
            WHERE m.deletion_timestamp IS NULL
              AND (
                  (
                      agbp.policy_data->>'addressGroupNamespace' = NEW.address_group_namespace
                      AND agbp.policy_data->>'addressGroupName' = NEW.address_group_name
                      AND agbp.policy_data->>'serviceNamespace' = NEW.service_namespace
                      AND agbp.policy_data->>'serviceName' = NEW.service_name
                  )
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
    END IF;

    -- Check 3: Port conflict detection (with bug: spec instead of ingress_ports)
    SELECT spec->'ingressPorts' INTO new_service_ports
    FROM services
    WHERE namespace = NEW.service_namespace AND name = NEW.service_name;

    IF new_service_ports IS NOT NULL AND jsonb_array_length(new_service_ports) > 0 THEN
        FOR conflicting_service IN
            SELECT s.namespace, s.name, s.spec->'ingressPorts' AS ingress_ports
            FROM address_group_bindings agb
            JOIN services s ON s.namespace = agb.service_namespace AND s.name = agb.service_name
            WHERE agb.address_group_namespace = NEW.address_group_namespace
              AND agb.address_group_name = NEW.address_group_name
              AND (agb.service_namespace != NEW.service_namespace OR agb.service_name != NEW.service_name)
              AND s.spec->'ingressPorts' IS NOT NULL
        LOOP
            other_service_ports := conflicting_service.ingress_ports;

            FOR new_port IN SELECT * FROM jsonb_array_elements(new_service_ports)
            LOOP
                FOR other_port IN SELECT * FROM jsonb_array_elements(other_service_ports)
                LOOP
                    IF new_port->>'protocol' = other_port->>'protocol' THEN
                        SELECT * INTO new_port_start, new_port_end
                        FROM parse_port_range(new_port->>'port');

                        SELECT * INTO other_port_start, other_port_end
                        FROM parse_port_range(other_port->>'port');

                        IF ports_overlap(new_port_start, new_port_end, other_port_start, other_port_end) THEN
                            RAISE EXCEPTION 'Port conflict detected: Service % port %/% overlaps with Service % port %/% in AddressGroup %',
                                NEW.service_namespace || '/' || NEW.service_name,
                                new_port->>'protocol', new_port->>'port',
                                conflicting_service.namespace || '/' || conflicting_service.name,
                                other_port->>'protocol', other_port->>'port',
                                NEW.address_group_namespace || '/' || NEW.address_group_name;
                        END IF;
                    END IF;
                END LOOP;
            END LOOP;
        END LOOP;

        RAISE NOTICE '[Migration 060] Port conflict check passed for Service % -> AddressGroup %',
            NEW.service_namespace || '/' || NEW.service_name,
            NEW.address_group_namespace || '/' || NEW.address_group_name;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
