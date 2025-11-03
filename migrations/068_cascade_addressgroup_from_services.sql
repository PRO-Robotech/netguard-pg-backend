-- +goose Up
-- +goose StatementBegin

-- =====================================================
-- Migration 068: CASCADE AddressGroup deletion from Service.spec.addressGroups
-- =====================================================
-- Purpose: Remove deleted AddressGroup from Service.spec.addressGroups (source="spec" case)
-- Pattern: Same as Migration 049 (cascade_host_deletion for Host/AddressGroup)
--
-- Problem (Migration 067 incomplete):
--   Migration 067 only handles AddressGroupBindings (source="binding")
--   Does NOT handle Service.spec.addressGroups (source="spec")
--   When AddressGroup deleted:
--     ❌ AG remains in Service.spec.addressGroups (stale reference)
--     ❌ Service.aggregated_address_groups not updated
--
-- Solution (apply Host/AddressGroup pattern):
--   1. CASCADE function: Remove AG from Service.spec.addressGroups on AG deletion
--   2. AFTER DELETE trigger: Automatically fire cascade on AG deletion
--   3. Existing trigger (Migration 018): trigger_update_aggregated_ags_on_spec_change()
--      automatically recalculates aggregated_address_groups when spec.addressGroups changes
--
-- Flow:
--   AddressGroup deleted
--     ↓
--   cascade_addressgroup_deletion() fires (AFTER DELETE)
--     ↓
--   Remove AG from Service.spec.addressGroups (UPDATE services)
--     ↓
--   trigger_update_aggregated_ags_on_spec_change() fires (AFTER UPDATE OF address_groups)
--     ↓
--   Service.aggregated_address_groups recalculated (deleted AG removed)
--
-- Related Migrations:
--   - Migration 018: aggregate_service_address_groups() and trigger_update_aggregated_ags_on_spec_change()
--   - Migration 022: cascade_host_deletion() pattern (Host → AddressGroup.spec.hosts)
--   - Migration 049: COALESCE fix for NULL-safe array removal
--   - Migration 067: AddressGroupBinding cascade (incomplete, only for bindings)
--
-- Date: 2025-10-31
-- =====================================================

-- ═══════════════════════════════════════════════════════════════════
-- CASCADE Function: Remove AG from Service.spec.addressGroups
-- ═══════════════════════════════════════════════════════════════════

CREATE OR REPLACE FUNCTION cascade_addressgroup_deletion()
RETURNS TRIGGER AS $$
DECLARE
    ag_obj jsonb;
BEGIN
    -- Build AddressGroup object reference (matches Service.spec.addressGroups format)
    ag_obj := jsonb_build_object(
        'apiVersion', 'netguard.sgroups.io/v1beta1',
        'kind', 'AddressGroup',
        'name', OLD.name,
        'namespace', OLD.namespace
    );

    RAISE NOTICE '[Migration 068] AddressGroup %.% deleted, removing from Services',
        OLD.namespace, OLD.name;

    -- Remove AG from Service.spec.addressGroups
    -- Use COALESCE to prevent NULL when removing last AG (Migration 049 pattern)
    UPDATE services
    SET address_groups = COALESCE(
        (
            SELECT jsonb_agg(ag_item)
            FROM jsonb_array_elements(address_groups) AS ag_item
            WHERE ag_item->>'name' != OLD.name
               OR ag_item->>'namespace' != OLD.namespace
        ),
        '[]'::jsonb  -- ✅ NULL-safe: return empty array instead of NULL
    )
    WHERE address_groups @> jsonb_build_array(ag_obj)
       OR EXISTS (
           SELECT 1
           FROM jsonb_array_elements(address_groups) AS ag_item
           WHERE ag_item->>'name' = OLD.name
             AND ag_item->>'namespace' = OLD.namespace
       );

    -- Log affected services
    IF FOUND THEN
        RAISE NOTICE '[Migration 068] Removed AG %.% from % Service(s)',
            OLD.namespace, OLD.name, (SELECT COUNT(*) FROM services WHERE address_groups @> jsonb_build_array(ag_obj));
    END IF;

    -- trigger_update_aggregated_ags_on_spec_change() (Migration 018) will fire automatically
    -- and recalculate aggregated_address_groups for affected Services

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cascade_addressgroup_deletion() IS
'Removes deleted AddressGroup from Service.spec.addressGroups.
Fires AFTER DELETE on address_groups.
Automatically triggers aggregated_address_groups recalculation via trigger_update_aggregated_ags_on_spec_change().
Pattern: Same as cascade_host_deletion() (Migration 049).';

-- ═══════════════════════════════════════════════════════════════════
-- AFTER DELETE Trigger: Cascade AG deletion to Services
-- ═══════════════════════════════════════════════════════════════════

CREATE TRIGGER cascade_addressgroup_from_services
    AFTER DELETE ON address_groups
    FOR EACH ROW
    EXECUTE FUNCTION cascade_addressgroup_deletion();

COMMENT ON TRIGGER cascade_addressgroup_from_services ON address_groups IS
'Cascades AddressGroup deletion to Service.spec.addressGroups.
Pattern: Same as cascade_host_from_address_groups (Migration 022).';

-- ═══════════════════════════════════════════════════════════════════
-- Data Fix: Recalculate aggregated_address_groups for all Services
-- ═══════════════════════════════════════════════════════════════════
-- This cleans up any existing stale AG references in aggregated_address_groups

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

-- Revert: Drop trigger and function
DROP TRIGGER IF EXISTS cascade_addressgroup_from_services ON address_groups;
DROP FUNCTION IF EXISTS cascade_addressgroup_deletion();

RAISE NOTICE '[Migration 068 Rollback] Cascade trigger removed. Manual cleanup may be required.';

-- +goose StatementEnd
