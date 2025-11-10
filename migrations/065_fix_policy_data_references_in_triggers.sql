-- +goose Up
-- +goose StatementBegin


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
          AND (NEW.namespace IS DISTINCT FROM agb.namespace OR NEW.name IS DISTINCT FROM agb.name)
    ) INTO duplicate_binding;

    IF duplicate_binding THEN
        RAISE EXCEPTION 'Duplicate binding detected: Service % already bound to AddressGroup %',
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
                      agbp.address_group_ref->>'namespace' = NEW.address_group_namespace
                      AND agbp.address_group_ref->>'name' = NEW.address_group_name
                      AND agbp.service_ref->>'namespace' = NEW.service_namespace
                      AND agbp.service_ref->>'name' = NEW.service_name
                  )
                  OR (
                      agbp.address_group_ref->>'namespace' = NEW.service_namespace
                      AND agbp.address_group_ref->>'name' = NEW.service_name
                      AND agbp.service_ref->>'namespace' = NEW.address_group_namespace
                      AND agbp.service_ref->>'name' = NEW.address_group_name
                  )
              )
        ) INTO policy_exists;

        IF NOT policy_exists THEN
            RAISE EXCEPTION 'cross-namespace binding not allowed: no AddressGroupBindingPolicy found in namespace % that references both AddressGroup % and Service %',
                NEW.address_group_namespace,
                NEW.address_group_name,
                NEW.service_name;
        END IF;

        RAISE NOTICE '[Migration 065] Cross-namespace binding allowed by policy: Service %/% -> AddressGroup %/%',
            NEW.service_namespace, NEW.service_name,
            NEW.address_group_namespace, NEW.address_group_name;
    END IF;

    DECLARE
        conflicting_binding RECORD;
        current_service_ports JSONB;
    BEGIN
        SELECT s.ingress_ports INTO current_service_ports
        FROM services s
        WHERE s.namespace = NEW.service_namespace
          AND s.name = NEW.service_name;

        FOR conflicting_binding IN
            SELECT
                agb.service_namespace,
                agb.service_name,
                s.ingress_ports AS existing_ports
            FROM address_group_bindings agb
            JOIN services s ON s.namespace = agb.service_namespace AND s.name = agb.service_name
            WHERE agb.address_group_namespace = NEW.address_group_namespace
              AND agb.address_group_name = NEW.address_group_name
              AND (agb.namespace != NEW.namespace OR agb.name != NEW.name)
        LOOP
            IF EXISTS (
                SELECT 1
                FROM jsonb_array_elements(current_service_ports) AS current_port,
                     jsonb_array_elements(conflicting_binding.existing_ports) AS existing_port
                WHERE current_port->>'protocol' = existing_port->>'protocol'
                  AND ports_overlap(
                      (current_port->>'port')::INTEGER,
                      (current_port->>'port')::INTEGER,
                      (existing_port->>'port')::INTEGER,
                      (existing_port->>'port')::INTEGER
                  )
            ) THEN
                RAISE EXCEPTION 'Port conflict detected: Service %/% and %/% have overlapping ports on the same AddressGroup %/%',
                    NEW.service_namespace, NEW.service_name,
                    conflicting_binding.service_namespace, conflicting_binding.service_name,
                    NEW.address_group_namespace, NEW.address_group_name;
            END IF;
        END LOOP;
    END;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION validate_service_ag_binding_conflicts IS '[Migration 065] Fixed policy_data references';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION validate_service_ag_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    duplicate_binding BOOLEAN := false;
    requires_policy BOOLEAN := false;
    policy_exists BOOLEAN := false;
BEGIN
    RAISE EXCEPTION 'Migration 065 rollback not supported: Migration 005 removed policy_data column';
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
