-- +goose Up
-- +goose StatementBegin



CREATE OR REPLACE FUNCTION cascade_addressgroup_deletion()
RETURNS TRIGGER AS $$
DECLARE
    ag_obj jsonb;
BEGIN
    ag_obj := jsonb_build_object(
        'apiVersion', 'netguard.sgroups.io/v1beta1',
        'kind', 'AddressGroup',
        'name', OLD.name,
        'namespace', OLD.namespace
    );

    RAISE NOTICE '[Migration 068] AddressGroup %.% deleted, removing from Services',
        OLD.namespace, OLD.name;

    UPDATE services
    SET address_groups = COALESCE(
        (
            SELECT jsonb_agg(ag_item)
            FROM jsonb_array_elements(address_groups) AS ag_item
            WHERE ag_item->>'name' != OLD.name
               OR ag_item->>'namespace' != OLD.namespace
        ),
        '[]'::jsonb
    )
    WHERE address_groups @> jsonb_build_array(ag_obj)
       OR EXISTS (
           SELECT 1
           FROM jsonb_array_elements(address_groups) AS ag_item
           WHERE ag_item->>'name' = OLD.name
             AND ag_item->>'namespace' = OLD.namespace
       );

    IF FOUND THEN
        RAISE NOTICE '[Migration 068] Removed AG %.% from % Service(s)',
            OLD.namespace, OLD.name, (SELECT COUNT(*) FROM services WHERE address_groups @> jsonb_build_array(ag_obj));
    END IF;


    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cascade_addressgroup_deletion() IS
'Removes deleted AddressGroup from Service.spec.addressGroups.
Fires AFTER DELETE on address_groups.
Automatically triggers aggregated_address_groups recalculation via trigger_update_aggregated_ags_on_spec_change().
Pattern: Same as cascade_host_deletion() (Migration 049).';


CREATE TRIGGER cascade_addressgroup_from_services
    AFTER DELETE ON address_groups
    FOR EACH ROW
    EXECUTE FUNCTION cascade_addressgroup_deletion();

COMMENT ON TRIGGER cascade_addressgroup_from_services ON address_groups IS
'Cascades AddressGroup deletion to Service.spec.addressGroups.
Pattern: Same as cascade_host_from_address_groups (Migration 022).';


DO $$
BEGIN
    RAISE NOTICE '[Migration 068] Recalculating aggregated_address_groups for all Services...';

    UPDATE services
    SET aggregated_address_groups = aggregate_service_address_groups(namespace::text, name::text);

    RAISE NOTICE '[Migration 068] Migration complete. AddressGroup CASCADE now works for both spec.addressGroups and AddressGroupBindings.';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS cascade_addressgroup_from_services ON address_groups;
DROP FUNCTION IF EXISTS cascade_addressgroup_deletion();

RAISE NOTICE '[Migration 068 Rollback] Cascade trigger removed. Manual cleanup may be required.';

-- +goose StatementEnd
