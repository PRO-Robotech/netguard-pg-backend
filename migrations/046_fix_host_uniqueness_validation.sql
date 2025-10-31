-- +goose Up
-- ============================================================================
-- Migration 046: Fix Host Uniqueness Validation
-- ============================================================================
--
-- Purpose: Fix Migration 045 validation logic to allow legitimate updates
--          to AddressGroup.spec.hosts while still preventing duplicates.
--
-- Problem: validate_host_uniqueness() was blocking ALL spec.hosts updates
--          because it checked if Host exists in ANY AddressGroup including
--          the CURRENT AddressGroup being updated.
--
-- Solution: Add filter to EXCLUDE current AddressGroup from spec.hosts check.
--           This allows legitimate operations while blocking actual duplicates.
--
-- ============================================================================

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_host_uniqueness(
    p_host_namespace TEXT,
    p_host_name TEXT,
    p_address_group_namespace TEXT,
    p_address_group_name TEXT,
    p_operation TEXT
)
RETURNS VOID AS $$
DECLARE
    v_existing_binding RECORD;
    v_spec_ag RECORD;
    v_host_full_name TEXT;
    v_ag_full_name TEXT;
BEGIN
    v_host_full_name := p_host_namespace || '/' || p_host_name;
    v_ag_full_name := p_address_group_namespace || '/' || p_address_group_name;

    -- Check 1: Host already has a HostBinding
    SELECT
        hb.address_group_namespace,
        hb.address_group_name
    INTO v_existing_binding
    FROM host_bindings hb
    JOIN k8s_metadata m ON m.resource_version = hb.resource_version
    WHERE hb.host_namespace = p_host_namespace::namespace_name
    AND hb.host_name = p_host_name::resource_name
    AND m.deletion_timestamp IS NULL
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'Host % already has HostBinding to AddressGroup %/%',
            v_host_full_name,
            v_existing_binding.address_group_namespace,
            v_existing_binding.address_group_name
        USING HINT = 'Each Host can only be bound to ONE AddressGroup',
              ERRCODE = '23505';
    END IF;

    -- Check 2: Host already exists in spec.hosts of ANOTHER AddressGroup
    -- FIX: Exclude CURRENT AddressGroup from check
    SELECT
        ag.namespace,
        ag.name
    INTO v_spec_ag
    FROM address_groups ag,
    LATERAL jsonb_array_elements(COALESCE(ag.hosts, '[]'::jsonb)) as host_elem
    WHERE jsonb_typeof(ag.hosts) = 'array'
    AND host_elem->>'namespace' = p_host_namespace
    AND host_elem->>'name' = p_host_name
    -- CRITICAL FIX: Exclude current AddressGroup
    AND NOT (ag.namespace = p_address_group_namespace::namespace_name
             AND ag.name = p_address_group_name::resource_name)
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'Host % already exists in spec.hosts of AddressGroup %/%',
            v_host_full_name,
            v_spec_ag.namespace,
            v_spec_ag.name
        USING HINT = 'Remove Host from spec.hosts before creating HostBinding or adding to another AddressGroup',
              ERRCODE = '23505';
    END IF;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION validate_host_uniqueness(TEXT, TEXT, TEXT, TEXT, TEXT) IS
'Validates that a Host can only be attached to ONE AddressGroup via ONE method.
Raises exception if Host already has HostBinding or exists in ANOTHER AG spec.hosts.
FIXED in Migration 046: Now excludes current AddressGroup from spec.hosts check.';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to Migration 045 version (without the fix)
CREATE OR REPLACE FUNCTION validate_host_uniqueness(
    p_host_namespace TEXT,
    p_host_name TEXT,
    p_address_group_namespace TEXT,
    p_address_group_name TEXT,
    p_operation TEXT
)
RETURNS VOID AS $$
DECLARE
    v_existing_binding RECORD;
    v_spec_ag RECORD;
    v_host_full_name TEXT;
    v_ag_full_name TEXT;
BEGIN
    v_host_full_name := p_host_namespace || '/' || p_host_name;
    v_ag_full_name := p_address_group_namespace || '/' || p_address_group_name;

    SELECT
        hb.address_group_namespace,
        hb.address_group_name
    INTO v_existing_binding
    FROM host_bindings hb
    JOIN k8s_metadata m ON m.resource_version = hb.resource_version
    WHERE hb.host_namespace = p_host_namespace::namespace_name
    AND hb.host_name = p_host_name::resource_name
    AND m.deletion_timestamp IS NULL
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'Host % already has HostBinding to AddressGroup %/%',
            v_host_full_name,
            v_existing_binding.address_group_namespace,
            v_existing_binding.address_group_name
        USING HINT = 'Each Host can only be bound to ONE AddressGroup',
              ERRCODE = '23505';
    END IF;

    SELECT
        ag.namespace,
        ag.name
    INTO v_spec_ag
    FROM address_groups ag,
    LATERAL jsonb_array_elements(COALESCE(ag.hosts, '[]'::jsonb)) as host_elem
    WHERE jsonb_typeof(ag.hosts) = 'array'
    AND host_elem->>'namespace' = p_host_namespace
    AND host_elem->>'name' = p_host_name
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'Host % already exists in spec.hosts of AddressGroup %/%',
            v_host_full_name,
            v_spec_ag.namespace,
            v_spec_ag.name
        USING HINT = 'Remove Host from spec.hosts before creating HostBinding',
              ERRCODE = '23505';
    END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
