-- +goose Up
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

-- +goose Down
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION validate_service_ag_binding_conflicts() RETURNS TRIGGER AS $$
DECLARE
    conflicting_service RECORD;
    ag_in_spec BOOLEAN := false;
BEGIN
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
