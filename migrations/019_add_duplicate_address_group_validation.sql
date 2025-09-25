-- +goose Up
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION validate_service_address_group_conflicts() RETURNS TRIGGER AS $$
DECLARE
    conflicting_count INTEGER;
    spec_groups JSONB;
    binding_groups JSONB;
    spec_group JSONB;
    binding_group JSONB;
    service_record RECORD;
    duplicate_count INTEGER;
    current_group_name TEXT;
    current_group_namespace TEXT;
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
            IF (spec_group->>'name') = NEW.address_group_name
               AND COALESCE(spec_group->>'namespace', NEW.service_namespace) = NEW.address_group_namespace THEN
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
            current_group_name := spec_group->>'name';
            current_group_namespace := COALESCE(spec_group->>'namespace', NEW.namespace);

            SELECT COUNT(*) INTO duplicate_count
            FROM jsonb_array_elements(spec_groups) AS elem
            WHERE elem->>'name' = current_group_name
            AND COALESCE(elem->>'namespace', NEW.namespace) = current_group_namespace;

            IF duplicate_count > 1 THEN
                RAISE EXCEPTION 'Duplicate AddressGroup %.% found in Service %.% spec.addressGroups - each AddressGroup can only be referenced once per Service',
                    current_group_namespace, current_group_name,
                    NEW.namespace, NEW.name;
            END IF;
        END LOOP;

        FOR spec_group IN
            SELECT value FROM jsonb_array_elements(spec_groups) AS value
        LOOP
            DECLARE
                group_name resource_name;
                group_namespace namespace_name;
            BEGIN
                group_name := (spec_group->>'name')::resource_name;
                group_namespace := COALESCE((spec_group->>'namespace')::namespace_name, NEW.namespace);

                SELECT COUNT(*) INTO conflicting_count
                FROM address_group_bindings agb
                WHERE agb.service_namespace = NEW.namespace
                AND agb.service_name = NEW.name
                AND agb.address_group_name = group_name
                AND agb.address_group_namespace = group_namespace;

                IF conflicting_count > 0 THEN
                    RAISE EXCEPTION 'AddressGroup %.% is already bound to Service %.% via AddressGroupBinding - cannot add to spec.addressGroups for dual registration conflict',
                        group_namespace, group_name,
                        NEW.namespace, NEW.name;
                END IF;
            END;
        END LOOP;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION validate_service_address_group_conflicts() IS 'Enhanced validation: Prevents both dual registration conflicts (spec vs bindings) and duplicate entries within spec.addressGroups';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

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
            IF (spec_group->>'name') = NEW.address_group_name
               AND COALESCE(spec_group->>'namespace', NEW.service_namespace) = NEW.address_group_namespace THEN
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
            DECLARE
                group_name resource_name;
                group_namespace namespace_name;
            BEGIN
                group_name := (spec_group->>'name')::resource_name;
                group_namespace := COALESCE((spec_group->>'namespace')::namespace_name, NEW.namespace);

                SELECT COUNT(*) INTO conflicting_count
                FROM address_group_bindings agb
                WHERE agb.service_namespace = NEW.namespace
                AND agb.service_name = NEW.name
                AND agb.address_group_name = group_name
                AND agb.address_group_namespace = group_namespace;

                IF conflicting_count > 0 THEN
                    RAISE EXCEPTION 'AddressGroup %.% is already bound to Service %.% via AddressGroupBinding - cannot add to spec.addressGroups for dual registration conflict',
                        group_namespace, group_name,
                        NEW.namespace, NEW.name;
                END IF;
            END;
        END LOOP;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd