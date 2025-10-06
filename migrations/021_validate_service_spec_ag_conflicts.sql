-- +goose Up

-- +goose StatementBegin

CREATE OR REPLACE FUNCTION validate_service_spec_ag_conflicts() RETURNS TRIGGER AS $$
DECLARE
    conflicting_ag RECORD;
    ag_ref JSONB;
BEGIN
    -- Only check if address_groups field is being modified
    IF TG_OP = 'UPDATE' AND NEW.address_groups = OLD.address_groups THEN
        RETURN NEW;
    END IF;

    IF NEW.address_groups IS NOT NULL AND jsonb_typeof(NEW.address_groups) = 'array' THEN
        FOR ag_ref IN SELECT jsonb_array_elements(NEW.address_groups) AS ag_obj
        LOOP
            SELECT agb.namespace, agb.name INTO conflicting_ag
            FROM address_group_bindings agb
            WHERE agb.service_namespace = NEW.namespace
            AND agb.service_name = NEW.name
            AND agb.address_group_namespace = (ag_ref->'ag_obj'->>'namespace')
            AND agb.address_group_name = (ag_ref->'ag_obj'->>'name')
            LIMIT 1;

            IF FOUND THEN
                RAISE EXCEPTION 'AddressGroup %.% is already bound to Service %.% via AddressGroupBinding %.% - cannot add to spec.addressGroups',
                    ag_ref->'ag_obj'->>'namespace', ag_ref->'ag_obj'->>'name',
                    NEW.namespace, NEW.name,
                    conflicting_ag.namespace, conflicting_ag.name;
            END IF;
        END LOOP;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER validate_service_spec_ag_conflicts_trigger
    BEFORE INSERT OR UPDATE OF address_groups ON services
    FOR EACH ROW
    EXECUTE FUNCTION validate_service_spec_ag_conflicts();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS validate_service_spec_ag_conflicts_trigger ON services;
DROP FUNCTION IF EXISTS validate_service_spec_ag_conflicts();

-- +goose StatementEnd
