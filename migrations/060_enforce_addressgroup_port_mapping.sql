-- +goose Up
-- +goose StatementBegin

-- =====================================================
-- Migration 060: Enforce AddressGroupPortMapping (Port Conflict Detection)
-- =====================================================
-- Purpose: Detect port conflicts when multiple Services bind to same AddressGroup
-- Pattern: Similar to CIDR overlap validation (Migration 024)
--
-- Problem:
--   Multiple Services can now bind to same AddressGroup (1:N after Migration 058).
--   AddressGroupPortMapping resource exists to track port allocations but NOT enforced.
--   Without validation, Services using same AG could have conflicting ports.
--
-- Solution:
--   Add validation function to check port conflicts:
--   1. When Service binds to AddressGroup, check all OTHER Services using same AG
--   2. For each other Service: Compare Service.spec.ingressPorts
--   3. Detect overlapping port ranges (TCP/UDP are separate namespaces)
--   4. If conflict detected: RAISE EXCEPTION with details
--   5. Use AddressGroupPortMapping for conflict resolution if exists
--
-- Port Conflict Logic:
--   - Same protocol (TCP or UDP) + overlapping port range = CONFLICT
--   - Example conflicts:
--     * Service A: TCP 8080, Service B: TCP 8080 ❌
--     * Service A: TCP 8080-9090, Service B: TCP 8085 ❌
--     * Service A: TCP 8080, Service B: UDP 8080 ✓ (different protocols)
--
-- AddressGroupPortMapping Format (from types.go):
--   access_ports: map[ServiceRef]ServicePortsRef
--   - If mapping exists for AG: Use it to allow specific port assignments
--   - If no mapping exists: Use default conflict detection
--
-- Related Migrations:
--   - Migration 024: CIDR overlap validation (similar pattern)
--   - Migration 058: Enabled 1:N Service -> AG (prerequisite)
--   - Migration 059: Cross-namespace policy (previous)
--
-- Date: 2025-10-31
-- =====================================================

-- Helper function: Parse port range string to integers
CREATE OR REPLACE FUNCTION parse_port_range(port_str TEXT, OUT port_start INTEGER, OUT port_end INTEGER) AS $$
BEGIN
    -- Handle single port (e.g., "8080")
    IF position('-' in port_str) = 0 THEN
        port_start := port_str::INTEGER;
        port_end := port_str::INTEGER;
    ELSE
        -- Handle port range (e.g., "8080-9090")
        port_start := split_part(port_str, '-', 1)::INTEGER;
        port_end := split_part(port_str, '-', 2)::INTEGER;
    END IF;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Helper function: Check if two port ranges overlap
CREATE OR REPLACE FUNCTION ports_overlap(
    range1_start INTEGER, range1_end INTEGER,
    range2_start INTEGER, range2_end INTEGER
) RETURNS BOOLEAN AS $$
BEGIN
    -- Ranges overlap if: range1.start <= range2.end AND range2.start <= range1.end
    RETURN (range1_start <= range2_end) AND (range2_start <= range1.end);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Update validation function: Add port conflict detection
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

    -- Check 3: Port conflict detection (NEW in Migration 060)
    -- Get ingress ports for the new Service
    SELECT spec->'ingressPorts' INTO new_service_ports
    FROM services
    WHERE namespace = NEW.service_namespace AND name = NEW.service_name;

    IF new_service_ports IS NOT NULL AND jsonb_array_length(new_service_ports) > 0 THEN
        -- Check all OTHER Services bound to the SAME AddressGroup
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

        RAISE NOTICE '[Migration 060] Port conflict check passed for Service % -> AddressGroup %',
            NEW.service_namespace || '/' || NEW.service_name,
            NEW.address_group_namespace || '/' || NEW.address_group_name;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop helper functions
DROP FUNCTION IF EXISTS ports_overlap(INTEGER, INTEGER, INTEGER, INTEGER);
DROP FUNCTION IF EXISTS parse_port_range(TEXT);

-- Revert to Migration 059 validation (no port conflict detection)
CREATE OR REPLACE FUNCTION validate_service_ag_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    duplicate_binding BOOLEAN := false;
    requires_policy BOOLEAN := false;
    policy_exists BOOLEAN := false;
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

        RAISE NOTICE '[Migration 059] Cross-namespace binding allowed by policy: Service % -> AddressGroup %',
            NEW.service_namespace || '/' || NEW.service_name,
            NEW.address_group_namespace || '/' || NEW.address_group_name;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
