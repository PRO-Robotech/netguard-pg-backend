-- +goose Up
-- +goose StatementBegin


CREATE OR REPLACE FUNCTION parse_port_range(port_str TEXT, OUT port_start INTEGER, OUT port_end INTEGER) AS $$
BEGIN
    IF position('-' in port_str) = 0 THEN
        port_start := port_str::INTEGER;
        port_end := port_str::INTEGER;
    ELSE
        port_start := split_part(port_str, '-', 1)::INTEGER;
        port_end := split_part(port_str, '-', 2)::INTEGER;
    END IF;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION ports_overlap(
    range1_start INTEGER, range1_end INTEGER,
    range2_start INTEGER, range2_end INTEGER
) RETURNS BOOLEAN AS $$
BEGIN
    RETURN (range1_start <= range2_end) AND (range2_start <= range1.end);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

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

DROP FUNCTION IF EXISTS ports_overlap(INTEGER, INTEGER, INTEGER, INTEGER);
DROP FUNCTION IF EXISTS parse_port_range(TEXT);

CREATE OR REPLACE FUNCTION validate_service_ag_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    duplicate_binding BOOLEAN := false;
    requires_policy BOOLEAN := false;
    policy_exists BOOLEAN := false;
BEGIN
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
