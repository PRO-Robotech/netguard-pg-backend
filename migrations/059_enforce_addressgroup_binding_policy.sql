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
          AND (TG_OP = 'INSERT' OR (agb.namespace != NEW.namespace OR agb.name != NEW.name))
    ) INTO duplicate_binding;

    IF duplicate_binding THEN
        RAISE EXCEPTION 'AddressGroupBinding already exists for Service %.% → AddressGroup %.%',
            NEW.service_namespace, NEW.service_name,
            NEW.address_group_namespace, NEW.address_group_name;
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

-- +goose Down
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION validate_service_ag_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    duplicate_binding BOOLEAN := false;
BEGIN
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
