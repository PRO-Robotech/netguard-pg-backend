--
-- PostgreSQL database dump
--

-- Dumped from database version 17.5
-- Dumped by pg_dump version 17.5

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: public; Type: SCHEMA; Schema: -; Owner: netguard
--

-- *not* creating schema, since initdb creates it


ALTER SCHEMA public OWNER TO netguard;

--
-- Name: uuid-ossp; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;


--
-- Name: EXTENSION "uuid-ossp"; Type: COMMENT; Schema: -; Owner: 
--

COMMENT ON EXTENSION "uuid-ossp" IS 'generate universally unique identifiers (UUIDs)';


--
-- Name: namespace_name; Type: DOMAIN; Schema: public; Owner: netguard
--

CREATE DOMAIN public.namespace_name AS text
	CONSTRAINT namespace_name_check CHECK (((char_length(VALUE) > 0) AND (char_length(VALUE) <= 253)));


ALTER DOMAIN public.namespace_name OWNER TO netguard;

--
-- Name: outbox_status; Type: TYPE; Schema: public; Owner: netguard
--

CREATE TYPE public.outbox_status AS ENUM (
    'PENDING',
    'PROCESSING',
    'SUCCESS',
    'FAILED_RETRYABLE',
    'FAILED_PERMANENT'
);


ALTER TYPE public.outbox_status OWNER TO netguard;

--
-- Name: TYPE outbox_status; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TYPE public.outbox_status IS 'Status of outbox entry lifecycle: PENDING → PROCESSING → SUCCESS/FAILED_RETRYABLE → FAILED_PERMANENT';


--
-- Name: resource_name; Type: DOMAIN; Schema: public; Owner: netguard
--

CREATE DOMAIN public.resource_name AS text
	CONSTRAINT resource_name_check CHECK (((char_length(VALUE) > 0) AND (char_length(VALUE) <= 253)));


ALTER DOMAIN public.resource_name OWNER TO netguard;

--
-- Name: rule_action; Type: DOMAIN; Schema: public; Owner: netguard
--

CREATE DOMAIN public.rule_action AS text
	CONSTRAINT rule_action_check CHECK ((VALUE = ANY (ARRAY['ACCEPT'::text, 'DROP'::text])));


ALTER DOMAIN public.rule_action OWNER TO netguard;

--
-- Name: sync_operation; Type: TYPE; Schema: public; Owner: netguard
--

CREATE TYPE public.sync_operation AS ENUM (
    'CREATE',
    'UPDATE',
    'DELETE'
);


ALTER TYPE public.sync_operation OWNER TO netguard;

--
-- Name: TYPE sync_operation; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TYPE public.sync_operation IS 'Operation type for resource synchronization: CREATE (new resource), UPDATE (modify existing), DELETE (remove resource)';


--
-- Name: target_system; Type: TYPE; Schema: public; Owner: netguard
--

CREATE TYPE public.target_system AS ENUM (
    'SGROUP',
    'INTERNAL'
);


ALTER TYPE public.target_system OWNER TO netguard;

--
-- Name: TYPE target_system; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TYPE public.target_system IS 'Target system for synchronization: SGROUP (external sync to SGROUP service), INTERNAL (internal process resources like HostBinding)';


--
-- Name: traffic_direction; Type: DOMAIN; Schema: public; Owner: netguard
--

CREATE DOMAIN public.traffic_direction AS text
	CONSTRAINT traffic_direction_check CHECK ((VALUE = ANY (ARRAY['INGRESS'::text, 'EGRESS'::text])));


ALTER DOMAIN public.traffic_direction OWNER TO netguard;

--
-- Name: transport_protocol; Type: DOMAIN; Schema: public; Owner: netguard
--

CREATE DOMAIN public.transport_protocol AS text
	CONSTRAINT transport_protocol_check CHECK ((VALUE = ANY (ARRAY['TCP'::text, 'UDP'::text, 'ICMP'::text])));


ALTER DOMAIN public.transport_protocol OWNER TO netguard;

--
-- Name: aggregate_address_group_hosts(text, text); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.aggregate_address_group_hosts(ag_namespace text, ag_name text) RETURNS jsonb
    LANGUAGE plpgsql
    AS $$
DECLARE
    aggregated_hosts_json JSONB := '[]'::jsonb;
    host_ref JSONB;
    host_record RECORD;
    hosts_field JSONB;
    seen_hosts TEXT[] := ARRAY[]::TEXT[];
BEGIN
    SELECT COALESCE(hosts, '[]'::jsonb) INTO hosts_field
    FROM address_groups
    WHERE namespace = ag_namespace AND name = ag_name;

    IF hosts_field IS NOT NULL AND hosts_field != 'null'::jsonb
       AND jsonb_typeof(hosts_field) = 'array' AND jsonb_array_length(hosts_field) > 0 THEN
        FOR host_ref IN
            SELECT jsonb_array_elements(hosts_field) as host_obj
        LOOP
            DECLARE
                host_name_text TEXT := host_ref->>'name';
            BEGIN
                IF host_name_text = ANY(seen_hosts) THEN
                    CONTINUE;
                END IF;

                SELECT h.uuid INTO host_record
                FROM hosts h
                WHERE h.namespace = ag_namespace::namespace_name
                AND h.name = host_name_text::resource_name;

                aggregated_hosts_json := aggregated_hosts_json || jsonb_build_array(
                    jsonb_build_object(
                        'ref', jsonb_build_object(
                            'apiVersion', COALESCE(host_ref->>'apiVersion', 'netguard.sgroups.io/v1beta1'),
                            'kind', COALESCE(host_ref->>'kind', 'Host'),
                            'namespace', ag_namespace,
                            'name', host_name_text
                        ),
                        'uuid', COALESCE(host_record.uuid, ''),
                        'source', 'spec'
                    )
                );

                seen_hosts := array_append(seen_hosts, host_name_text);
            END;
        END LOOP;
    END IF;

    FOR host_record IN
        SELECT h.namespace, h.name, h.uuid
        FROM host_bindings hb
        JOIN hosts h ON h.namespace = hb.host_namespace AND h.name = hb.host_name
        WHERE hb.address_group_namespace = ag_namespace::namespace_name
        AND hb.address_group_name = ag_name::resource_name
    LOOP
        IF host_record.name = ANY(seen_hosts) THEN
            CONTINUE;
        END IF;

        aggregated_hosts_json := aggregated_hosts_json || jsonb_build_array(
            jsonb_build_object(
                'ref', jsonb_build_object(
                    'apiVersion', 'netguard.sgroups.io/v1beta1',
                    'kind', 'Host',
                    'namespace', host_record.namespace,
                    'name', host_record.name
                ),
                'uuid', host_record.uuid,
                'source', 'binding'
            )
        );

        seen_hosts := array_append(seen_hosts, host_record.name);
    END LOOP;

    RETURN aggregated_hosts_json;
END;
$$;


ALTER FUNCTION public.aggregate_address_group_hosts(ag_namespace text, ag_name text) OWNER TO netguard;

--
-- Name: aggregate_service_address_groups(text, text); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.aggregate_service_address_groups(svc_namespace text, svc_name text) RETURNS jsonb
    LANGUAGE plpgsql
    AS $$
DECLARE
    aggregated_ags_json JSONB := '[]'::jsonb;
    ag_ref JSONB;
    ag_record RECORD;
    address_groups_field JSONB;
BEGIN
    -- Get address_groups field, handling null case
    SELECT COALESCE(address_groups, '[]'::jsonb) INTO address_groups_field
    FROM services
    WHERE namespace = svc_namespace AND name = svc_name;

    -- Source 1: Collect AddressGroups from spec.address_groups (source = 'spec')
    IF address_groups_field IS NOT NULL AND address_groups_field != 'null'::jsonb
       AND jsonb_typeof(address_groups_field) = 'array' AND jsonb_array_length(address_groups_field) > 0 THEN
        FOR ag_ref IN
            SELECT jsonb_array_elements(address_groups_field) as ag_obj
        LOOP
            -- Add AddressGroup reference with source information
            aggregated_ags_json := aggregated_ags_json || jsonb_build_array(
                jsonb_build_object(
                    'ref', ag_ref,
                    'source', 'spec'
                )
            );
        END LOOP;
    END IF;

    -- Source 2: Collect AddressGroups from AddressGroupBindings (source = 'binding')
    FOR ag_record IN
        SELECT ag.namespace, ag.name
        FROM address_group_bindings agb
        JOIN address_groups ag ON ag.namespace = agb.address_group_namespace
                              AND ag.name = agb.address_group_name
        WHERE agb.service_namespace = svc_namespace::namespace_name
        AND agb.service_name = svc_name::resource_name
    LOOP
        -- Add AddressGroup reference with source information
        aggregated_ags_json := aggregated_ags_json || jsonb_build_array(
            jsonb_build_object(
                'ref', jsonb_build_object(
                    'apiVersion', 'netguard.sgroups.io/v1beta1',
                    'kind', 'AddressGroup',
                    'name', ag_record.name,
                    'namespace', ag_record.namespace
                ),
                'source', 'binding'
            )
        );
    END LOOP;

    RETURN aggregated_ags_json;
END;
$$;


ALTER FUNCTION public.aggregate_service_address_groups(svc_namespace text, svc_name text) OWNER TO netguard;

--
-- Name: cascade_host_deletion(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.cascade_host_deletion() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    host_obj jsonb;
BEGIN
    host_obj := jsonb_build_object(
        'name', OLD.name,
        'apiVersion', 'netguard.sgroups.io/v1beta1',
        'kind', 'Host'
    );
    
    UPDATE address_groups
    SET hosts = COALESCE(
        (
            SELECT jsonb_agg(host_element)
            FROM jsonb_array_elements(hosts) AS host_element
            WHERE host_element != host_obj
        ), 
        '[]'::jsonb  -- Return empty array if all elements removed
    )
    WHERE hosts @> jsonb_build_array(host_obj)
    AND hosts IS NOT NULL;
    
    RAISE NOTICE 'Removed host %.% from all AddressGroups due to host deletion', OLD.namespace, OLD.name;
    
    RETURN OLD;
END;
$$;


ALTER FUNCTION public.cascade_host_deletion() OWNER TO netguard;

--
-- Name: check_host_exclusivity(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.check_host_exclusivity() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
BEGIN
    -- Check if any host in NEW.hosts already belongs to another AddressGroup
    IF EXISTS (
        SELECT 1 FROM address_groups 
        WHERE (namespace, name) != (NEW.namespace, NEW.name)
        AND hosts ?| ARRAY(
            SELECT jsonb_array_elements_text(
                jsonb_path_query_array(NEW.hosts, '$[*].name')
            )
        )
        AND hosts ?| ARRAY(
            SELECT jsonb_array_elements_text(
                jsonb_path_query_array(NEW.hosts, '$[*].namespace')
            )
        )
    ) THEN
        RAISE EXCEPTION 'Host already belongs to another AddressGroup - each host can belong to only one AddressGroup';
    END IF;
    RETURN NEW;
END;
$_$;


ALTER FUNCTION public.check_host_exclusivity() OWNER TO netguard;

--
-- Name: rebuild_address_group_networks(text, text); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.rebuild_address_group_networks(ag_namespace text, ag_name text) RETURNS jsonb
    LANGUAGE plpgsql
    AS $$
DECLARE
    networks_json JSONB;
BEGIN
    SELECT COALESCE(jsonb_agg(
        jsonb_build_object(
            'name', n.name,
            'namespace', n.namespace,
            'cidr', n.cidr
        )
    ), '[]'::jsonb)
    INTO networks_json
    FROM network_bindings nb
    INNER JOIN networks n ON nb.network_namespace = n.namespace AND nb.network_name = n.name
    WHERE nb.address_group_namespace = ag_namespace
      AND nb.address_group_name = ag_name;

    RETURN networks_json;
END;
$$;


ALTER FUNCTION public.rebuild_address_group_networks(ag_namespace text, ag_name text) OWNER TO netguard;

--
-- Name: sync_ready_from_conditions(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.sync_ready_from_conditions() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    new_ready_status BOOLEAN;
    host_updated INT := 0;
    network_updated INT := 0;
BEGIN
    -- ========================================================================
    -- Determine Ready Status from Conditions
    -- ========================================================================
    -- Ready = TRUE if conditions array contains: {"type":"Ready","status":"True"}
    -- Ready = FALSE otherwise (conditions empty, Ready=False, or condition missing)
    -- JSONB contains operator (@>) checks if left JSONB contains right JSONB

    new_ready_status := (NEW.conditions @> '[{"type":"Ready","status":"True"}]'::jsonb);

    -- ========================================================================
    -- Update hosts.ready (if this k8s_metadata belongs to a Host)
    -- ========================================================================
    -- Strategy: Try UPDATE hosts WHERE resource_version = NEW.resource_version
    -- If ROW_COUNT > 0, this was a Host resource
    -- Only update if ready value actually changed (avoid unnecessary updates)

    UPDATE hosts
    SET ready = new_ready_status
    WHERE resource_version = NEW.resource_version
      AND (ready IS DISTINCT FROM new_ready_status);  -- Only update if changed

    GET DIAGNOSTICS host_updated = ROW_COUNT;

    IF host_updated > 0 THEN
        RAISE NOTICE '[029] Updated % host(s) ready = % for resource_version %',
            host_updated, new_ready_status, NEW.resource_version;

        -- Note: If ready changed from FALSE→TRUE, migration 026 trigger will fire now
        -- and create sync_outbox entry for affected AddressGroup
    END IF;

    -- ========================================================================
    -- Update networks.ready (if this k8s_metadata belongs to a Network)
    -- ========================================================================
    -- Same strategy as hosts

    UPDATE networks
    SET ready = new_ready_status
    WHERE resource_version = NEW.resource_version
      AND (ready IS DISTINCT FROM new_ready_status);  -- Only update if changed

    GET DIAGNOSTICS network_updated = ROW_COUNT;

    IF network_updated > 0 THEN
        RAISE NOTICE '[029] Updated % network(s) ready = % for resource_version %',
            network_updated, new_ready_status, NEW.resource_version;

        -- Note: If ready changed from FALSE→TRUE, migration 027 trigger will fire now
        -- and create sync_outbox entry for affected AddressGroup
    END IF;

    -- ========================================================================
    -- Log if no resource updated
    -- ========================================================================

    IF host_updated = 0 AND network_updated = 0 THEN
        -- This k8s_metadata belongs to AddressGroup, Service, or other resource type
        -- These resources don't have .ready field yet (only hosts/networks do)
        -- OR ready value didn't change (optimization prevented update)
        -- This is NOT an error - just means no action needed
        RAISE DEBUG '[029] resource_version % is not a Host or Network (or ready unchanged), skipping',
            NEW.resource_version;
    END IF;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.sync_ready_from_conditions() OWNER TO netguard;

--
-- Name: FUNCTION sync_ready_from_conditions(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.sync_ready_from_conditions() IS 'P0 BLOCKER WORKAROUND: Auto-syncs hosts.ready/networks.ready from k8s_metadata.conditions.
Fires when k8s_metadata.conditions changes (AFTER UPDATE trigger).
Computes ready = (conditions contains Ready=True) and updates corresponding resource table.
Enables migrations 026/027 triggers to work without backend code changes.
Migration 029 - Temporary workaround until backend implements ready field updates.';


--
-- Name: trigger_address_group_before_delete(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_address_group_before_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    ag_uid UUID;
    ag_resource_version BIGINT;
    already_marked_for_deletion BOOLEAN;
BEGIN
    RAISE NOTICE '[032] BEFORE DELETE trigger fired for AddressGroup %.%',
        OLD.namespace, OLD.name;

    -- Get AddressGroup metadata
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO ag_uid, already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    -- If already marked for deletion, allow actual deletion
    IF already_marked_for_deletion THEN
        RAISE NOTICE '[032] AddressGroup %.% already marked for deletion (Worker calling DELETE after sync)',
            OLD.namespace, OLD.name;
        RAISE NOTICE '[032] Allowing actual deletion from DB...';
        RETURN OLD; -- Allow DELETE to proceed
    END IF;

    -- Otherwise, apply Sync-First strategy
    RAISE NOTICE '[032] Initial DELETE request for AddressGroup %.% - applying Sync-First strategy',
        OLD.namespace, OLD.name;

    -- Mark as pending deletion
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    RAISE NOTICE '[032] ✅ Marked AddressGroup %.% as pending deletion',
        OLD.namespace, OLD.name;

    -- Create DELETE outbox entry
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'AddressGroup',
        ag_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RAISE NOTICE '[032] ✅ Created DELETE outbox entry for AddressGroup %.%',
        OLD.namespace, OLD.name;

    -- PREVENT actual deletion
    RAISE NOTICE '[032] ⛔ Preventing DELETE from DB (resource stays until Worker processes)';

    RETURN NULL; -- Prevents DELETE
END;
$$;


ALTER FUNCTION public.trigger_address_group_before_delete() OWNER TO netguard;

--
-- Name: trigger_address_group_binding_before_delete(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_address_group_binding_before_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    agb_uid UUID;
    agb_resource_version BIGINT;
    already_marked_for_deletion BOOLEAN;
BEGIN
    RAISE NOTICE '[036] BEFORE DELETE trigger fired for AddressGroupBinding %.%',
        OLD.namespace, OLD.name;

    -- Get AddressGroupBinding metadata
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO agb_uid, already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    -- If already marked for deletion, allow actual deletion
    IF already_marked_for_deletion THEN
        RAISE NOTICE '[036] AddressGroupBinding %.% already marked for deletion (Worker calling DELETE after sync)',
            OLD.namespace, OLD.name;
        RAISE NOTICE '[036] Allowing actual deletion from DB...';
        RETURN OLD; -- Allow DELETE to proceed
    END IF;

    -- Otherwise, apply Sync-First strategy
    RAISE NOTICE '[036] Initial DELETE request for AddressGroupBinding %.% - applying Sync-First strategy',
        OLD.namespace, OLD.name;

    -- Mark as pending deletion
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting internal processing before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    RAISE NOTICE '[036] ✅ Marked AddressGroupBinding %.% as pending deletion',
        OLD.namespace, OLD.name;

    -- Create DELETE outbox entry
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'AddressGroupBinding',
        agb_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'service', jsonb_build_object(
                'namespace', OLD.service_namespace,
                'name', OLD.service_name
            ),
            'address_group', jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name
            ),
            'migration', '036'
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RAISE NOTICE '[036] ✅ Created DELETE outbox entry for AddressGroupBinding %.%',
        OLD.namespace, OLD.name;

    -- PREVENT actual deletion
    RAISE NOTICE '[036] ⛔ Preventing DELETE from DB (resource stays until Worker processes)';

    RETURN NULL; -- Prevents DELETE
END;
$$;


ALTER FUNCTION public.trigger_address_group_binding_before_delete() OWNER TO netguard;

--
-- Name: FUNCTION trigger_address_group_binding_before_delete(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_address_group_binding_before_delete() IS 'Migration 036: Sync-First DELETE trigger for AddressGroupBinding resources.
Fires BEFORE DELETE, creates outbox entry for internal processing, prevents actual deletion.
Worker will update affected Service and then delete AddressGroupBinding from DB.';


--
-- Name: trigger_address_group_binding_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_address_group_binding_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    resource_id UUID;
    operation_type sync_operation;
BEGIN
    -- Determine operation
    IF TG_OP = 'INSERT' THEN
        operation_type := 'CREATE'::sync_operation;
        RAISE NOTICE '[036] AddressGroupBinding %.% created, creating outbox entry', NEW.namespace, NEW.name;
    ELSE
        operation_type := 'UPDATE'::sync_operation;
        RAISE NOTICE '[036] AddressGroupBinding %.% updated, creating outbox entry', NEW.namespace, NEW.name;
    END IF;

    -- Generate deterministic UUID for this AddressGroupBinding
    resource_id := uuid_generate_v5(
        uuid_ns_dns(),
        'AddressGroupBinding:' || NEW.namespace || '/' || NEW.name
    );

    -- Create outbox entry
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'AddressGroupBinding',
        resource_id,
        operation_type,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'service', jsonb_build_object(
                'namespace', NEW.service_namespace,
                'name', NEW.service_name
            ),
            'address_group', jsonb_build_object(
                'namespace', NEW.address_group_namespace,
                'name', NEW.address_group_name
            ),
            'trigger_reason', TG_OP::text,
            'migration', '036'
        ),
        NEW.namespace,
        NEW.name,
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW();

    RAISE NOTICE '[036] ✅ Outbox entry created for AddressGroupBinding %.% (resource_id: %)',
        NEW.namespace, NEW.name, resource_id;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_address_group_binding_upsert_outbox() OWNER TO netguard;

--
-- Name: FUNCTION trigger_address_group_binding_upsert_outbox(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_address_group_binding_upsert_outbox() IS 'Migration 036: Unified outbox trigger for AddressGroupBinding resources.
Fires on INSERT or UPDATE, ALWAYS creates outbox entry for internal processing.
AddressGroupBinding affects Service (triggers Service UPDATE for SGROUP sync).';


--
-- Name: trigger_address_group_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_address_group_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_resource_id UUID;  -- ✅ FIXED
    v_operation_type sync_operation;  -- ✅ FIXED
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
        RAISE NOTICE '[035] AddressGroup %.% created, creating outbox entry', NEW.namespace, NEW.name;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
        RAISE NOTICE '[035] AddressGroup %.% updated, creating outbox entry', NEW.namespace, NEW.name;
    END IF;

    v_resource_id := uuid_generate_v5(
        uuid_ns_dns(),
        'AddressGroup:' || NEW.namespace || '/' || NEW.name
    );

    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'AddressGroup',
        v_resource_id,  -- ✅ FIXED
        v_operation_type,  -- ✅ FIXED
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'trigger_reason', TG_OP::text,
            'migration', '035'
        ),
        NEW.namespace,
        NEW.name,
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW();

    RAISE NOTICE '[035] ✅ Outbox entry created for AddressGroup %.% (resource_id: %)',
        NEW.namespace, NEW.name, v_resource_id;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_address_group_upsert_outbox() OWNER TO netguard;

--
-- Name: FUNCTION trigger_address_group_upsert_outbox(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_address_group_upsert_outbox() IS 'Migration 035: Fixed variable naming conflict from Migration 034.';


--
-- Name: trigger_ag_update_on_binding_change(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_ag_update_on_binding_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    host_ready BOOLEAN;
    ag_ready BOOLEAN;
    ag_resource_version BIGINT;
    outbox_resource_id UUID;
    use_namespace TEXT;
    use_name TEXT;
BEGIN
    -- Determine which binding to process (handle DELETE vs INSERT/UPDATE)
    IF TG_OP = 'DELETE' THEN
        use_namespace := OLD.address_group_namespace;
        use_name := OLD.address_group_name;

        RAISE NOTICE '[033] Binding DELETED: % (Host: %.%, AG: %.%)',
            OLD.name, OLD.host_namespace, OLD.host_name, OLD.address_group_namespace, OLD.address_group_name;
    ELSE
        use_namespace := NEW.address_group_namespace;
        use_name := NEW.address_group_name;

        RAISE NOTICE '[033] Binding %: % (Host: %.%, AG: %.%)',
            TG_OP, NEW.name, NEW.host_namespace, NEW.host_name, NEW.address_group_namespace, NEW.address_group_name;
    END IF;

    -- ========================================================================
    -- STEP 1A: Check if Host is Ready
    -- ========================================================================

    IF TG_OP = 'DELETE' THEN
        SELECT h.ready INTO host_ready
        FROM hosts h
        WHERE h.namespace = OLD.host_namespace
          AND h.name = OLD.host_name;
    ELSE
        SELECT h.ready INTO host_ready
        FROM hosts h
        WHERE h.namespace = NEW.host_namespace
          AND h.name = NEW.host_name;
    END IF;

    IF host_ready IS NULL THEN
        RAISE WARNING '[033] Host not found in hosts table, skipping outbox creation';
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        ELSE
            RETURN NEW;
        END IF;
    END IF;

    IF NOT host_ready THEN
        RAISE NOTICE '[033] Host %.% not Ready yet (ready=FALSE), skipping outbox creation',
            COALESCE(NEW.host_namespace, OLD.host_namespace),
            COALESCE(NEW.host_name, OLD.host_name);
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        ELSE
            RETURN NEW;
        END IF;
    END IF;

    RAISE NOTICE '[033] Host %.% is Ready ✅',
        COALESCE(NEW.host_namespace, OLD.host_namespace),
        COALESCE(NEW.host_name, OLD.host_name);

    -- ========================================================================
    -- STEP 1B: Check if AddressGroup is Ready
    -- ========================================================================

    -- Get AG resource_version first
    SELECT ag.resource_version INTO ag_resource_version
    FROM address_groups ag
    WHERE ag.namespace = use_namespace
      AND ag.name = use_name;

    IF ag_resource_version IS NULL THEN
        RAISE WARNING '[033] AddressGroup %.% not found in address_groups table, skipping outbox creation',
            use_namespace, use_name;
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        ELSE
            RETURN NEW;
        END IF;
    END IF;

    -- Check AG Ready condition from k8s_metadata
    SELECT EXISTS(
        SELECT 1 FROM k8s_metadata km
        WHERE km.resource_version = ag_resource_version
          AND km.conditions @> '[{"type":"Ready","status":"True"}]'::jsonb
    ) INTO ag_ready;

    IF NOT ag_ready THEN
        RAISE NOTICE '[033] AddressGroup %.% not Ready yet, skipping outbox creation',
            use_namespace, use_name;
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        ELSE
            RETURN NEW;
        END IF;
    END IF;

    RAISE NOTICE '[033] AddressGroup %.% is Ready ✅', use_namespace, use_name;

    -- ========================================================================
    -- STEP 1C: Both Ready → Create Outbox Entry
    -- ========================================================================
    -- CRITICAL: This enforces user's business logic:
    -- "Binding is valid only when both resources have Condition.Ready = TRUE"
    -- "As soon as we created such Binding process - at THIS moment we should sync to SGROUP"

    RAISE NOTICE '[033] Both Host and AG are Ready → Creating Outbox entry for AG sync...';

    -- Generate UUID for outbox entry
    outbox_resource_id := gen_random_uuid();

    -- Build payload based on operation type
    IF TG_OP = 'DELETE' THEN
        INSERT INTO sync_outbox (
            resource_type,
            resource_id,
            resource_namespace,
            resource_name,
            operation,
            target_system,
            payload,
            status,
            attempts,
            max_retries,
            created_at,
            updated_at,
            next_retry_at
        )
        VALUES (
            'AddressGroup',
            outbox_resource_id,
            OLD.address_group_namespace,
            OLD.address_group_name,
            'UPDATE'::sync_operation,
            'SGROUP'::target_system,
            jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name,
                'trigger_reason', 'binding_deleted',
                'binding', jsonb_build_object(
                    'namespace', OLD.namespace,
                    'name', OLD.name
                ),
                'host', jsonb_build_object(
                    'namespace', OLD.host_namespace,
                    'name', OLD.host_name
                )
            ),
            'PENDING'::outbox_status,
            0,  -- attempts
            5,  -- max_retries
            NOW(),
            NOW(),
            NOW()  -- Process immediately
        )
        -- Handle conflict: If concurrent binding changes, reset to PENDING
        ON CONFLICT (resource_type, resource_id, operation, target_system)
        DO UPDATE SET
            status = 'PENDING'::outbox_status,
            attempts = 0,
            next_retry_at = NOW(),
            updated_at = NOW(),
            payload = EXCLUDED.payload;

        RAISE NOTICE '[033] ✅ Created Outbox entry for AG %.% (binding deleted, resource_id: %)',
            OLD.address_group_namespace, OLD.address_group_name, outbox_resource_id;
    ELSE
        INSERT INTO sync_outbox (
            resource_type,
            resource_id,
            resource_namespace,
            resource_name,
            operation,
            target_system,
            payload,
            status,
            attempts,
            max_retries,
            created_at,
            updated_at,
            next_retry_at
        )
        VALUES (
            'AddressGroup',
            outbox_resource_id,
            NEW.address_group_namespace,
            NEW.address_group_name,
            'UPDATE'::sync_operation,
            'SGROUP'::target_system,
            jsonb_build_object(
                'namespace', NEW.address_group_namespace,
                'name', NEW.address_group_name,
                'trigger_reason', lower(TG_OP::text) || '_binding',
                'binding', jsonb_build_object(
                    'namespace', NEW.namespace,
                    'name', NEW.name
                ),
                'host', jsonb_build_object(
                    'namespace', NEW.host_namespace,
                    'name', NEW.host_name
                )
            ),
            'PENDING'::outbox_status,
            0,  -- attempts
            5,  -- max_retries
            NOW(),
            NOW(),
            NOW()  -- Process immediately
        )
        -- Handle conflict: If concurrent binding changes, reset to PENDING
        ON CONFLICT (resource_type, resource_id, operation, target_system)
        DO UPDATE SET
            status = 'PENDING'::outbox_status,
            attempts = 0,
            next_retry_at = NOW(),
            updated_at = NOW(),
            payload = EXCLUDED.payload;

        RAISE NOTICE '[033] ✅ Created Outbox entry for AG %.% (binding %, resource_id: %)',
            NEW.address_group_namespace, NEW.address_group_name, lower(TG_OP::text), outbox_resource_id;
    END IF;

    -- ========================================================================
    -- STEP 1D: Update AG.aggregated_hosts (PRESERVE OLD BEHAVIOR)
    -- ========================================================================
    -- This ensures AG always has correct aggregated_hosts list
    -- Even if Outbox processing fails, AG state remains consistent

    RAISE NOTICE '[033] Updating aggregated_hosts for AG %.%...', use_namespace, use_name;

    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(use_namespace::TEXT, use_name::TEXT)
    WHERE namespace = use_namespace AND name = use_name;

    IF FOUND THEN
        RAISE NOTICE '[033] ✅ Updated aggregated_hosts for AG %.%', use_namespace, use_name;
    ELSE
        RAISE WARNING '[033] Failed to update aggregated_hosts for AG %.%', use_namespace, use_name;
    END IF;

    -- Return appropriate value
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$;


ALTER FUNCTION public.trigger_ag_update_on_binding_change() OWNER TO netguard;

--
-- Name: FUNCTION trigger_ag_update_on_binding_change(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_ag_update_on_binding_change() IS 'Fires when host_bindings table changes (INSERT/UPDATE/DELETE).
Creates sync_outbox entry for AddressGroup UPDATE ONLY if:
1. Host is Ready (hosts.ready = TRUE)
2. AddressGroup is Ready (k8s_metadata.conditions contains Ready=True)
This enforces the business logic: Binding is valid only between Ready resources.
Migration 033 - Restores binding triggers removed in Migration 026.';


--
-- Name: trigger_host_before_delete(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_host_before_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    host_uid UUID;
    host_resource_version BIGINT;
    already_marked_for_deletion BOOLEAN;
BEGIN
    RAISE NOTICE '[032] BEFORE DELETE trigger fired for Host %.%',
        OLD.namespace, OLD.name;

    -- Get Host metadata
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO host_uid, already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    -- If already marked for deletion, allow actual deletion
    -- This happens when Worker calls DELETE after SGROUP sync
    IF already_marked_for_deletion THEN
        RAISE NOTICE '[032] Host %.% already marked for deletion (Worker calling DELETE after sync)',
            OLD.namespace, OLD.name;
        RAISE NOTICE '[032] Allowing actual deletion from DB...';
        RETURN OLD; -- Allow DELETE to proceed
    END IF;

    -- Otherwise, this is initial DELETE request → apply Sync-First strategy
    RAISE NOTICE '[032] Initial DELETE request for Host %.% - applying Sync-First strategy',
        OLD.namespace, OLD.name;

    -- Mark as pending deletion in k8s_metadata
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    RAISE NOTICE '[032] ✅ Marked Host %.% as pending deletion',
        OLD.namespace, OLD.name;

    -- Create DELETE outbox entry
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'Host',
        host_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'uuid', OLD.uuid
        ),
        'PENDING'::outbox_status,
        0,  -- attempts
        5,  -- max_retries
        NOW(),
        NOW(),
        NOW()  -- Process immediately
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RAISE NOTICE '[032] ✅ Created DELETE outbox entry for Host %.%',
        OLD.namespace, OLD.name;

    -- PREVENT actual deletion - Worker will delete after SGROUP sync
    RAISE NOTICE '[032] ⛔ Preventing DELETE from DB (resource stays until Worker processes)';

    RETURN NULL; -- Prevents DELETE
END;
$$;


ALTER FUNCTION public.trigger_host_before_delete() OWNER TO netguard;

--
-- Name: trigger_host_binding_before_delete(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_host_binding_before_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    hb_uid UUID;
    hb_resource_version BIGINT;
    already_marked_for_deletion BOOLEAN;
BEGIN
    RAISE NOTICE '[036] BEFORE DELETE trigger fired for HostBinding %.%',
        OLD.namespace, OLD.name;

    -- Get HostBinding metadata
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO hb_uid, already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    -- If already marked for deletion, allow actual deletion
    IF already_marked_for_deletion THEN
        RAISE NOTICE '[036] HostBinding %.% already marked for deletion (Worker calling DELETE after sync)',
            OLD.namespace, OLD.name;
        RAISE NOTICE '[036] Allowing actual deletion from DB...';
        RETURN OLD; -- Allow DELETE to proceed
    END IF;

    -- Otherwise, apply Sync-First strategy
    RAISE NOTICE '[036] Initial DELETE request for HostBinding %.% - applying Sync-First strategy',
        OLD.namespace, OLD.name;

    -- Mark as pending deletion
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting internal processing before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    RAISE NOTICE '[036] ✅ Marked HostBinding %.% as pending deletion',
        OLD.namespace, OLD.name;

    -- Create DELETE outbox entry
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'HostBinding',
        hb_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'host', jsonb_build_object(
                'namespace', OLD.host_namespace,
                'name', OLD.host_name
            ),
            'address_group', jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name
            ),
            'migration', '036'
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RAISE NOTICE '[036] ✅ Created DELETE outbox entry for HostBinding %.%',
        OLD.namespace, OLD.name;

    -- PREVENT actual deletion
    RAISE NOTICE '[036] ⛔ Preventing DELETE from DB (resource stays until Worker processes)';

    RETURN NULL; -- Prevents DELETE
END;
$$;


ALTER FUNCTION public.trigger_host_binding_before_delete() OWNER TO netguard;

--
-- Name: FUNCTION trigger_host_binding_before_delete(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_host_binding_before_delete() IS 'Migration 036: Sync-First DELETE trigger for HostBinding resources.
Fires BEFORE DELETE, creates outbox entry for internal processing, prevents actual deletion.
Worker will update affected AddressGroup and then delete HostBinding from DB.';


--
-- Name: trigger_host_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_host_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_resource_id UUID;  -- ✅ FIXED: Renamed from resource_id
    v_operation_type sync_operation;  -- ✅ FIXED: Renamed from operation_type
BEGIN
    -- Determine operation: INSERT → CREATE, UPDATE → UPDATE
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
        RAISE NOTICE '[035] Host %.% created, creating outbox entry', NEW.namespace, NEW.name;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
        RAISE NOTICE '[035] Host %.% updated, creating outbox entry', NEW.namespace, NEW.name;
    END IF;

    -- Generate deterministic UUID for this Host
    -- Host table has uuid column, use it!
    v_resource_id := NEW.uuid;

    -- Create outbox entry (with ON CONFLICT for deduplication)
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'Host',
        v_resource_id,  -- ✅ FIXED: Uses v_ prefixed variable
        v_operation_type,  -- ✅ FIXED: Uses v_ prefixed variable
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'uuid', NEW.uuid::text,
            'trigger_reason', TG_OP::text,
            'migration', '035'
        ),
        NEW.namespace,
        NEW.name,
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW(),
        payload = jsonb_build_object(
            'namespace', EXCLUDED.payload->>'namespace',
            'name', EXCLUDED.payload->>'name',
            'uuid', EXCLUDED.payload->>'uuid',
            'trigger_reason', 'conflict_reset',
            'migration', '035'
        );

    RAISE NOTICE '[035] ✅ Outbox entry created for Host %.% (resource_id: %)',
        NEW.namespace, NEW.name, v_resource_id;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_host_upsert_outbox() OWNER TO netguard;

--
-- Name: FUNCTION trigger_host_upsert_outbox(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_host_upsert_outbox() IS 'Migration 035: Fixed variable naming conflict from Migration 034.
Renamed: resource_id → v_resource_id, operation_type → v_operation_type.
Prevents ambiguous column reference error in ON CONFLICT clause.';


--
-- Name: trigger_network_before_delete(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_network_before_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    network_uid UUID;
    network_resource_version BIGINT;
    already_marked_for_deletion BOOLEAN;
BEGIN
    RAISE NOTICE '[032] BEFORE DELETE trigger fired for Network %.%',
        OLD.namespace, OLD.name;

    -- Get Network metadata
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO network_uid, already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    -- If already marked for deletion, allow actual deletion
    IF already_marked_for_deletion THEN
        RAISE NOTICE '[032] Network %.% already marked for deletion (Worker calling DELETE after sync)',
            OLD.namespace, OLD.name;
        RAISE NOTICE '[032] Allowing actual deletion from DB...';
        RETURN OLD; -- Allow DELETE to proceed
    END IF;

    -- Otherwise, apply Sync-First strategy
    RAISE NOTICE '[032] Initial DELETE request for Network %.% - applying Sync-First strategy',
        OLD.namespace, OLD.name;

    -- Mark as pending deletion
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    RAISE NOTICE '[032] ✅ Marked Network %.% as pending deletion',
        OLD.namespace, OLD.name;

    -- Create DELETE outbox entry
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'Network',
        network_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'cidr', OLD.cidr::text
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RAISE NOTICE '[032] ✅ Created DELETE outbox entry for Network %.%',
        OLD.namespace, OLD.name;

    -- PREVENT actual deletion
    RAISE NOTICE '[032] ⛔ Preventing DELETE from DB (resource stays until Worker processes)';

    RETURN NULL; -- Prevents DELETE
END;
$$;


ALTER FUNCTION public.trigger_network_before_delete() OWNER TO netguard;

--
-- Name: trigger_network_binding_before_delete(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_network_binding_before_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    nb_uid UUID;
    nb_resource_version BIGINT;
    already_marked_for_deletion BOOLEAN;
BEGIN
    RAISE NOTICE '[036] BEFORE DELETE trigger fired for NetworkBinding %.%',
        OLD.namespace, OLD.name;

    -- Get NetworkBinding metadata
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO nb_uid, already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    -- If already marked for deletion, allow actual deletion
    IF already_marked_for_deletion THEN
        RAISE NOTICE '[036] NetworkBinding %.% already marked for deletion (Worker calling DELETE after sync)',
            OLD.namespace, OLD.name;
        RAISE NOTICE '[036] Allowing actual deletion from DB...';
        RETURN OLD; -- Allow DELETE to proceed
    END IF;

    -- Otherwise, apply Sync-First strategy
    RAISE NOTICE '[036] Initial DELETE request for NetworkBinding %.% - applying Sync-First strategy',
        OLD.namespace, OLD.name;

    -- Mark as pending deletion
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting internal processing before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    RAISE NOTICE '[036] ✅ Marked NetworkBinding %.% as pending deletion',
        OLD.namespace, OLD.name;

    -- Create DELETE outbox entry
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'NetworkBinding',
        nb_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'network', jsonb_build_object(
                'namespace', OLD.network_namespace,
                'name', OLD.network_name
            ),
            'address_group', jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name
            ),
            'migration', '036'
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RAISE NOTICE '[036] ✅ Created DELETE outbox entry for NetworkBinding %.%',
        OLD.namespace, OLD.name;

    -- PREVENT actual deletion
    RAISE NOTICE '[036] ⛔ Preventing DELETE from DB (resource stays until Worker processes)';

    RETURN NULL; -- Prevents DELETE
END;
$$;


ALTER FUNCTION public.trigger_network_binding_before_delete() OWNER TO netguard;

--
-- Name: FUNCTION trigger_network_binding_before_delete(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_network_binding_before_delete() IS 'Migration 036: Sync-First DELETE trigger for NetworkBinding resources.
Fires BEFORE DELETE, creates outbox entry for internal processing, prevents actual deletion.
Worker will update affected AddressGroup and then delete NetworkBinding from DB.';


--
-- Name: trigger_network_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_network_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_resource_id UUID;  -- ✅ FIXED
    v_operation_type sync_operation;  -- ✅ FIXED
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
        RAISE NOTICE '[035] Network %.% created, creating outbox entry', NEW.namespace, NEW.name;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
        RAISE NOTICE '[035] Network %.% updated, creating outbox entry', NEW.namespace, NEW.name;
    END IF;

    v_resource_id := uuid_generate_v5(
        uuid_ns_dns(),
        'Network:' || NEW.namespace || '/' || NEW.name
    );

    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'Network',
        v_resource_id,  -- ✅ FIXED
        v_operation_type,  -- ✅ FIXED
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'cidr', NEW.cidr::text,
            'trigger_reason', TG_OP::text,
            'migration', '035'
        ),
        NEW.namespace,
        NEW.name,
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW();

    RAISE NOTICE '[035] ✅ Outbox entry created for Network %.% (resource_id: %)',
        NEW.namespace, NEW.name, v_resource_id;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_network_upsert_outbox() OWNER TO netguard;

--
-- Name: FUNCTION trigger_network_upsert_outbox(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_network_upsert_outbox() IS 'Migration 035: Fixed variable naming conflict from Migration 034.';


--
-- Name: trigger_service_before_delete(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_service_before_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    service_uid UUID;
    service_resource_version BIGINT;
    already_marked_for_deletion BOOLEAN;
BEGIN
    RAISE NOTICE '[036] BEFORE DELETE trigger fired for Service %.%',
        OLD.namespace, OLD.name;

    -- Get Service metadata
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO service_uid, already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    -- If already marked for deletion, allow actual deletion
    -- This happens when Worker calls DELETE after SGROUP sync
    IF already_marked_for_deletion THEN
        RAISE NOTICE '[036] Service %.% already marked for deletion (Worker calling DELETE after sync)',
            OLD.namespace, OLD.name;
        RAISE NOTICE '[036] Allowing actual deletion from DB...';
        RETURN OLD; -- Allow DELETE to proceed
    END IF;

    -- Otherwise, this is initial DELETE request → apply Sync-First strategy
    RAISE NOTICE '[036] Initial DELETE request for Service %.% - applying Sync-First strategy',
        OLD.namespace, OLD.name;

    -- Mark as pending deletion in k8s_metadata
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    RAISE NOTICE '[036] ✅ Marked Service %.% as pending deletion',
        OLD.namespace, OLD.name;

    -- Create DELETE outbox entry
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'Service',
        service_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'migration', '036'
        ),
        'PENDING'::outbox_status,
        0,  -- attempts
        5,  -- max_retries
        NOW(),
        NOW(),
        NOW()  -- Process immediately
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RAISE NOTICE '[036] ✅ Created DELETE outbox entry for Service %.%',
        OLD.namespace, OLD.name;

    -- PREVENT actual deletion - Worker will delete after SGROUP sync
    RAISE NOTICE '[036] ⛔ Preventing DELETE from DB (resource stays until Worker processes)';

    RETURN NULL; -- Prevents DELETE
END;
$$;


ALTER FUNCTION public.trigger_service_before_delete() OWNER TO netguard;

--
-- Name: FUNCTION trigger_service_before_delete(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_service_before_delete() IS 'Migration 036: Sync-First DELETE trigger for Service resources.
Fires BEFORE DELETE, creates outbox entry for SGROUP sync, prevents actual deletion.
Worker will delete from DB after successful SGROUP sync.';


--
-- Name: trigger_service_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_service_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_resource_id UUID;  -- ✅ FIXED
    v_operation_type sync_operation;  -- ✅ FIXED
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
        RAISE NOTICE '[035] Service %.% created, creating outbox entry', NEW.namespace, NEW.name;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
        RAISE NOTICE '[035] Service %.% updated, creating outbox entry', NEW.namespace, NEW.name;
    END IF;

    v_resource_id := uuid_generate_v5(
        uuid_ns_dns(),
        'Service:' || NEW.namespace || '/' || NEW.name
    );

    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'Service',
        v_resource_id,  -- ✅ FIXED
        v_operation_type,  -- ✅ FIXED
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'trigger_reason', TG_OP::text,
            'migration', '035'
        ),
        NEW.namespace,
        NEW.name,
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW();

    RAISE NOTICE '[035] ✅ Outbox entry created for Service %.% (resource_id: %)',
        NEW.namespace, NEW.name, v_resource_id;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_service_upsert_outbox() OWNER TO netguard;

--
-- Name: FUNCTION trigger_service_upsert_outbox(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_service_upsert_outbox() IS 'Migration 035: Fixed variable naming conflict from Migration 034.';


--
-- Name: trigger_update_aggregated_ags_on_binding_change(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_update_aggregated_ags_on_binding_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    -- Update aggregated_address_groups for the affected Service(s)
    IF TG_OP = 'DELETE' THEN
        UPDATE services
        SET aggregated_address_groups = aggregate_service_address_groups(OLD.service_namespace, OLD.service_name)
        WHERE namespace = OLD.service_namespace AND name = OLD.service_name;
        RETURN OLD;
    ELSE
        UPDATE services
        SET aggregated_address_groups = aggregate_service_address_groups(NEW.service_namespace, NEW.service_name)
        WHERE namespace = NEW.service_namespace AND name = NEW.service_name;

        -- If UPDATE changed the Service reference, also update the old one
        IF TG_OP = 'UPDATE' AND (OLD.service_namespace != NEW.service_namespace OR OLD.service_name != NEW.service_name) THEN
            UPDATE services
            SET aggregated_address_groups = aggregate_service_address_groups(OLD.service_namespace, OLD.service_name)
            WHERE namespace = OLD.service_namespace AND name = OLD.service_name;
        END IF;

        RETURN NEW;
    END IF;
END;
$$;


ALTER FUNCTION public.trigger_update_aggregated_ags_on_binding_change() OWNER TO netguard;

--
-- Name: trigger_update_aggregated_ags_on_spec_change(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_update_aggregated_ags_on_spec_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    -- Update aggregated_address_groups with separate UPDATE statement after the main operation
    UPDATE services
    SET aggregated_address_groups = aggregate_service_address_groups(NEW.namespace, NEW.name)
    WHERE namespace = NEW.namespace AND name = NEW.name;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_update_aggregated_ags_on_spec_change() OWNER TO netguard;

--
-- Name: trigger_update_aggregated_hosts_on_spec_change(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_update_aggregated_hosts_on_spec_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    ag_currently_ready BOOLEAN;
    ag_resource_version BIGINT;
    outbox_resource_id UUID;
BEGIN
    RAISE NOTICE '[028] Trigger fired for AddressGroup %.% (spec.hosts changed)',
        NEW.namespace, NEW.name;

    RAISE NOTICE '[028] Updating aggregated_hosts for AG %.%...',
        NEW.namespace, NEW.name;

    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(NEW.namespace::TEXT, NEW.name::TEXT)
    WHERE namespace = NEW.namespace AND name = NEW.name
    RETURNING resource_version INTO ag_resource_version;

    IF NOT FOUND THEN
        RAISE WARNING '[028] AddressGroup %.% not found! Skipping.',
            NEW.namespace, NEW.name;
        RETURN NEW;
    END IF;

    RAISE NOTICE '[028] ✅ Updated aggregated_hosts for AG %.% (resource_version: %)',
        NEW.namespace, NEW.name, ag_resource_version;

    RAISE NOTICE '[028] Checking if AG %.% is Ready...',
        NEW.namespace, NEW.name;

    SELECT EXISTS(
        SELECT 1 FROM k8s_metadata km
        WHERE km.resource_version = ag_resource_version
          AND km.conditions @> '[{"type":"Ready","status":"True"}]'::jsonb
    ) INTO ag_currently_ready;

    RAISE NOTICE '[028] AG %.% Ready status: %',
        NEW.namespace, NEW.name, ag_currently_ready;

    IF ag_currently_ready THEN
        RAISE NOTICE '[028] Creating Outbox entry for AG %.% sync...',
            NEW.namespace, NEW.name;

        outbox_resource_id := gen_random_uuid();

        INSERT INTO sync_outbox (
            resource_type,
            resource_id,
            resource_namespace,
            resource_name,
            operation,
            target_system,
            payload,
            status,
            attempts,
            max_retries,
            created_at,
            updated_at,
            next_retry_at
        )
        VALUES (
            'AddressGroup',
            outbox_resource_id,
            NEW.namespace,
            NEW.name,
            'UPDATE'::sync_operation,
            'SGROUP'::target_system,
            jsonb_build_object(
                'namespace', NEW.namespace,
                'name', NEW.name,
                'trigger_reason', 'spec_hosts_change'
            ),
            'PENDING'::outbox_status,
            0,
            5,
            NOW(),
            NOW(),
            NOW()
        )
        ON CONFLICT (resource_type, resource_id, operation, target_system)
        DO UPDATE SET
            resource_namespace = EXCLUDED.resource_namespace,
            resource_name = EXCLUDED.resource_name,
            status = 'PENDING'::outbox_status,
            attempts = 0,
            next_retry_at = NOW(),
            updated_at = NOW(),
            payload = jsonb_build_object(
                'namespace', EXCLUDED.payload->>'namespace',
                'name', EXCLUDED.payload->>'name',
                'trigger_reason', 'spec_hosts_change'
            );

        RAISE NOTICE '[028] ✅ Created/updated Outbox entry for AG %.% (resource_id: %)',
            NEW.namespace, NEW.name, outbox_resource_id;
    ELSE
        RAISE NOTICE '[028] ⏭️  AG %.% not Ready yet, skipping Outbox creation',
            NEW.namespace, NEW.name;
        RAISE NOTICE '[028] ℹ️  AG will sync when it becomes Ready (separate trigger)';
    END IF;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_update_aggregated_hosts_on_spec_change() OWNER TO netguard;

--
-- Name: FUNCTION trigger_update_aggregated_hosts_on_spec_change(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_update_aggregated_hosts_on_spec_change() IS 'Fires when AddressGroup.Spec.Hosts changes.
Updates AG.aggregated_hosts immediately (preserves migration 014 behavior) and
creates sync_outbox entry for SGROUP sync (NEW in migration 028).
Only creates Outbox if AG is currently Ready.
Migration 028 - UPDATED from migration 014.
Migration 031 - FIXED to populate resource_namespace and resource_name.';


--
-- Name: unbind_hosts_on_address_group_deletion(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.unbind_hosts_on_address_group_deletion() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    host_ref JSONB;
    host_name TEXT;
    hosts_to_unbind JSONB := COALESCE(OLD.hosts, '[]'::jsonb);
BEGIN
    IF jsonb_typeof(hosts_to_unbind) != 'array' THEN
        hosts_to_unbind := '[]'::jsonb;
    END IF;

    IF hosts_to_unbind IS NOT NULL AND hosts_to_unbind != 'null'::jsonb
       AND jsonb_typeof(hosts_to_unbind) = 'array' AND jsonb_array_length(hosts_to_unbind) > 0 THEN
        FOR host_ref IN SELECT jsonb_array_elements(hosts_to_unbind)
        LOOP
            host_name := host_ref->>'name';

            -- Unbind the host: set is_bound = false and clear address_group_ref
            UPDATE hosts
            SET
                is_bound = false,
                address_group_ref_namespace = NULL,
                address_group_ref_name = NULL
            WHERE namespace = OLD.namespace::namespace_name
            AND name = host_name::resource_name
            AND is_bound = true
            AND address_group_ref_namespace = OLD.namespace
            AND address_group_ref_name = OLD.name;

            RAISE NOTICE 'Unbound host %.% from deleted AddressGroup %.%', OLD.namespace, host_name, OLD.namespace, OLD.name;
        END LOOP;
    END IF;

    RETURN OLD;
END;
$$;


ALTER FUNCTION public.unbind_hosts_on_address_group_deletion() OWNER TO netguard;

--
-- Name: FUNCTION unbind_hosts_on_address_group_deletion(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.unbind_hosts_on_address_group_deletion() IS 'Automatically unbinds hosts when AddressGroup is deleted';


--
-- Name: update_aggregated_ags_for_service(text, text); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.update_aggregated_ags_for_service(svc_namespace text, svc_name text) RETURNS void
    LANGUAGE plpgsql
    AS $$
BEGIN
    UPDATE services
    SET aggregated_address_groups = aggregate_service_address_groups(svc_namespace, svc_name)
    WHERE namespace = svc_namespace::namespace_name AND name = svc_name::resource_name;

    RAISE NOTICE 'Updated aggregated_address_groups for Service %.%', svc_namespace, svc_name;
END;
$$;


ALTER FUNCTION public.update_aggregated_ags_for_service(svc_namespace text, svc_name text) OWNER TO netguard;

--
-- Name: update_aggregated_hosts_for_address_group(text, text); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.update_aggregated_hosts_for_address_group(ag_namespace text, ag_name text) RETURNS void
    LANGUAGE plpgsql
    AS $$
BEGIN
    UPDATE address_groups 
    SET aggregated_hosts = aggregate_address_group_hosts(ag_namespace, ag_name)
    WHERE namespace = ag_namespace::namespace_name AND name = ag_name::resource_name;
    
    RAISE NOTICE 'Updated aggregated_hosts for AddressGroup %.%', ag_namespace, ag_name;
END;
$$;


ALTER FUNCTION public.update_aggregated_hosts_for_address_group(ag_namespace text, ag_name text) OWNER TO netguard;

--
-- Name: update_host_binding_status_on_spec_change(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.update_host_binding_status_on_spec_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    old_hosts JSONB := COALESCE(OLD.hosts, '[]'::jsonb);
    new_hosts JSONB := COALESCE(NEW.hosts, '[]'::jsonb);
    host_ref JSONB;
    host_name TEXT;
BEGIN
    IF jsonb_typeof(old_hosts) != 'array' THEN
        old_hosts := '[]'::jsonb;
    END IF;
    IF jsonb_typeof(new_hosts) != 'array' THEN
        new_hosts := '[]'::jsonb;
    END IF;

    -- Only process if hosts field actually changed
    IF old_hosts = new_hosts THEN
        RETURN NEW;
    END IF;

    IF jsonb_typeof(old_hosts) = 'array' AND jsonb_array_length(old_hosts) > 0 THEN
        FOR host_ref IN SELECT jsonb_array_elements(old_hosts)
        LOOP
            host_name := host_ref->>'name';

            IF NOT (new_hosts @> jsonb_build_array(host_ref)) THEN
                UPDATE hosts
                SET
                    is_bound = false,
                    address_group_ref_namespace = NULL,
                    address_group_ref_name = NULL
                WHERE namespace = NEW.namespace::namespace_name
                AND name = host_name::resource_name
                AND is_bound = true
                AND address_group_ref_namespace = OLD.namespace
                AND address_group_ref_name = OLD.name;

                RAISE NOTICE 'Unbound host %.% from AddressGroup %.%', NEW.namespace, host_name, OLD.namespace, OLD.name;
            END IF;
        END LOOP;
    END IF;

    IF jsonb_typeof(new_hosts) = 'array' AND jsonb_array_length(new_hosts) > 0 THEN
        FOR host_ref IN SELECT jsonb_array_elements(new_hosts)
        LOOP
            host_name := host_ref->>'name';

            IF NOT (old_hosts @> jsonb_build_array(host_ref)) THEN
                UPDATE hosts
                SET
                    is_bound = true,
                    address_group_ref_namespace = NEW.namespace,
                    address_group_ref_name = NEW.name
                WHERE namespace = NEW.namespace::namespace_name
                AND name = host_name::resource_name;

                RAISE NOTICE 'Bound host %.% to AddressGroup %.%', NEW.namespace, host_name, NEW.namespace, NEW.name;
            END IF;
        END LOOP;
    END IF;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.update_host_binding_status_on_spec_change() OWNER TO netguard;

--
-- Name: FUNCTION update_host_binding_status_on_spec_change(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.update_host_binding_status_on_spec_change() IS 'Automatically updates host binding status when AddressGroup.spec.hosts changes';


--
-- Name: validate_address_group_hosts(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.validate_address_group_hosts() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    host_ref JSONB;
    v_host_name TEXT;
    seen_names TEXT[] := ARRAY[]::TEXT[];
    conflicting_ag RECORD;
    new_hosts JSONB := COALESCE(NEW.hosts, '[]'::jsonb);
BEGIN
    IF new_hosts IS NULL OR new_hosts = 'null'::jsonb OR
       jsonb_typeof(new_hosts) != 'array' OR jsonb_array_length(new_hosts) = 0 THEN
        RETURN NEW;
    END IF;

    FOR host_ref IN SELECT jsonb_array_elements(new_hosts)
    LOOP
        v_host_name := host_ref->>'name';

        IF v_host_name IS NULL OR v_host_name = '' THEN
            CONTINUE;
        END IF;

        IF v_host_name = ANY(seen_names) THEN
            RAISE EXCEPTION 'Duplicate host "%" in AddressGroup %.%', v_host_name, NEW.namespace, NEW.name
                USING ERRCODE = '23505';
        END IF;
        seen_names := array_append(seen_names, v_host_name);

        SELECT ag.namespace, ag.name INTO conflicting_ag
        FROM address_groups ag
        WHERE ag.hosts @> jsonb_build_array(
            jsonb_build_object(
                'apiVersion', 'netguard.sgroups.io/v1beta1',
                'kind', 'Host',
                'name', v_host_name
            )
        )
        AND (ag.namespace != NEW.namespace OR ag.name != NEW.name)
        LIMIT 1;

        IF FOUND THEN
            RAISE EXCEPTION 'Host "%" already belongs to AddressGroup %.%', v_host_name, conflicting_ag.namespace, conflicting_ag.name
                USING ERRCODE = '23505';
        END IF;

        SELECT ag.namespace, ag.name INTO conflicting_ag
        FROM host_bindings hb
        JOIN address_groups ag ON ag.namespace = hb.address_group_namespace AND ag.name = hb.address_group_name
        WHERE hb.host_namespace = NEW.namespace
        AND hb.host_name = v_host_name
        AND (ag.namespace != NEW.namespace OR ag.name != NEW.name)
        LIMIT 1;

        IF FOUND THEN
            RAISE EXCEPTION 'Host "%" already belongs to AddressGroup %.%', v_host_name, conflicting_ag.namespace, conflicting_ag.name
                USING ERRCODE = '23505';
        END IF;

        IF EXISTS (
            SELECT 1 FROM host_bindings hb
            WHERE hb.host_namespace = NEW.namespace
            AND hb.host_name = v_host_name
            AND hb.address_group_namespace = NEW.namespace
            AND hb.address_group_name = NEW.name
        ) THEN
            RAISE EXCEPTION 'Host "%" is already bound to AddressGroup %.%', v_host_name, NEW.namespace, NEW.name
                USING ERRCODE = '23505';
        END IF;
    END LOOP;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.validate_address_group_hosts() OWNER TO netguard;

--
-- Name: validate_host_binding_conflicts(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.validate_host_binding_conflicts() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    conflicting_ag RECORD;
    host_in_spec BOOLEAN := false;
BEGIN
    SELECT EXISTS(
        SELECT 1 FROM address_groups ag
        WHERE ag.hosts @> jsonb_build_array(
            jsonb_build_object(
                'apiVersion', 'netguard.sgroups.io/v1beta1',
                'kind', 'Host',
                'name', NEW.host_name
            )
        )
    ) INTO host_in_spec;

    IF host_in_spec THEN
        -- Find which AddressGroup contains this host in spec.hosts
        SELECT ag.namespace, ag.name INTO conflicting_ag
        FROM address_groups ag
        WHERE ag.hosts @> jsonb_build_array(
            jsonb_build_object(
                'apiVersion', 'netguard.sgroups.io/v1beta1',
                'kind', 'Host',
                'name', NEW.host_name
            )
        )
        LIMIT 1;

        IF conflicting_ag.namespace = NEW.address_group_namespace AND
           conflicting_ag.name = NEW.address_group_name THEN
            RAISE EXCEPTION 'Host %.% already in AddressGroup %.%', NEW.host_namespace, NEW.host_name, NEW.address_group_namespace, NEW.address_group_name
                USING ERRCODE = '23505';
        ELSE
            RAISE EXCEPTION 'Host %.% already belongs to AddressGroup %.%', NEW.host_namespace, NEW.host_name, conflicting_ag.namespace, conflicting_ag.name
                USING ERRCODE = '23505';
        END IF;
    END IF;

    SELECT ag.namespace, ag.name INTO conflicting_ag
    FROM host_bindings hb
    JOIN address_groups ag ON ag.namespace = hb.address_group_namespace AND ag.name = hb.address_group_name
    WHERE hb.host_namespace = NEW.host_namespace
    AND hb.host_name = NEW.host_name
    AND (hb.address_group_namespace != NEW.address_group_namespace OR hb.address_group_name != NEW.address_group_name)
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'Host %.% already belongs to AddressGroup %.%', NEW.host_namespace, NEW.host_name, conflicting_ag.namespace, conflicting_ag.name
            USING ERRCODE = '23505';
    END IF;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.validate_host_binding_conflicts() OWNER TO netguard;

--
-- Name: validate_service_ag_binding_conflicts(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.validate_service_ag_binding_conflicts() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    conflicting_service RECORD;
    ag_in_spec BOOLEAN := false;
BEGIN
    -- Check if AddressGroup is already in spec.address_groups of ANY Service (including the target one)
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
        -- Find which Service contains this AddressGroup in spec.address_groups
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

    -- Check if AddressGroup is already bound to a different Service via AddressGroupBinding
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
$$;


ALTER FUNCTION public.validate_service_ag_binding_conflicts() OWNER TO netguard;

--
-- Name: validate_service_spec_ag_conflicts(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.validate_service_spec_ag_conflicts() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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
$$;


ALTER FUNCTION public.validate_service_spec_ag_conflicts() OWNER TO netguard;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: address_group_binding_policies; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.address_group_binding_policies (
    namespace public.namespace_name NOT NULL,
    name public.resource_name NOT NULL,
    resource_version bigint NOT NULL,
    address_group_ref jsonb DEFAULT '{}'::jsonb,
    service_ref jsonb DEFAULT '{}'::jsonb
);


ALTER TABLE public.address_group_binding_policies OWNER TO netguard;

--
-- Name: address_group_bindings; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.address_group_bindings (
    namespace public.namespace_name NOT NULL,
    name public.resource_name NOT NULL,
    service_namespace public.namespace_name NOT NULL,
    service_name public.resource_name NOT NULL,
    address_group_namespace public.namespace_name NOT NULL,
    address_group_name public.resource_name NOT NULL,
    resource_version bigint NOT NULL
);


ALTER TABLE public.address_group_bindings OWNER TO netguard;

--
-- Name: address_group_port_mappings; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.address_group_port_mappings (
    namespace public.namespace_name NOT NULL,
    name public.resource_name NOT NULL,
    access_ports jsonb DEFAULT '{}'::jsonb NOT NULL,
    resource_version bigint NOT NULL
);


ALTER TABLE public.address_group_port_mappings OWNER TO netguard;

--
-- Name: address_groups; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.address_groups (
    namespace public.namespace_name NOT NULL,
    name public.resource_name NOT NULL,
    default_action public.rule_action DEFAULT 'DROP'::text NOT NULL,
    logs boolean DEFAULT false NOT NULL,
    trace boolean DEFAULT false NOT NULL,
    description text DEFAULT ''::text,
    resource_version bigint NOT NULL,
    networks jsonb DEFAULT '[]'::jsonb NOT NULL,
    hosts jsonb DEFAULT '[]'::jsonb NOT NULL,
    aggregated_hosts jsonb DEFAULT '[]'::jsonb NOT NULL
);


ALTER TABLE public.address_groups OWNER TO netguard;

--
-- Name: COLUMN address_groups.networks; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.address_groups.networks IS 'NetworkItem[] - List of networks bound to this AddressGroup via NetworkBindings';


--
-- Name: COLUMN address_groups.hosts; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.address_groups.hosts IS 'ObjectReference[] - List of hosts that belong exclusively to this AddressGroup';


--
-- Name: COLUMN address_groups.aggregated_hosts; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.address_groups.aggregated_hosts IS 'HostReference[] - Aggregated list of all hosts from both spec.hosts and HostBindings with source tracking';


--
-- Name: host_bindings; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.host_bindings (
    namespace public.namespace_name NOT NULL,
    name public.resource_name NOT NULL,
    host_namespace public.namespace_name NOT NULL,
    host_name public.resource_name NOT NULL,
    address_group_namespace public.namespace_name NOT NULL,
    address_group_name public.resource_name NOT NULL,
    resource_version bigint NOT NULL
);


ALTER TABLE public.host_bindings OWNER TO netguard;

--
-- Name: hosts; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.hosts (
    namespace public.namespace_name NOT NULL,
    name public.resource_name NOT NULL,
    uuid text NOT NULL,
    host_name_sync text,
    address_group_name text,
    is_bound boolean DEFAULT false NOT NULL,
    binding_ref_namespace public.namespace_name,
    binding_ref_name public.resource_name,
    address_group_ref_namespace public.namespace_name,
    address_group_ref_name public.resource_name,
    resource_version bigint NOT NULL,
    ip_list jsonb,
    ready boolean DEFAULT false NOT NULL,
    CONSTRAINT check_ip_list_is_array CHECK (((ip_list IS NULL) OR (jsonb_typeof(ip_list) = 'array'::text)))
);


ALTER TABLE public.hosts OWNER TO netguard;

--
-- Name: COLUMN hosts.ready; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.hosts.ready IS 'Boolean flag indicating Host Ready status (synced from k8s_metadata.conditions).
Used by trigger trg_update_ag_on_host_ready to fire ONLY when Host becomes Ready (FALSE→TRUE transition).
Application must update this field when Host.status.conditions contains {type:Ready,status:True}.';


--
-- Name: ie_ag_ag_rules; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.ie_ag_ag_rules (
    namespace public.namespace_name NOT NULL,
    name public.resource_name NOT NULL,
    transport public.transport_protocol NOT NULL,
    traffic public.traffic_direction NOT NULL,
    action public.rule_action NOT NULL,
    address_group_local_namespace public.namespace_name NOT NULL,
    address_group_local_name public.resource_name NOT NULL,
    address_group_namespace public.namespace_name NOT NULL,
    address_group_name public.resource_name NOT NULL,
    ports jsonb DEFAULT '[]'::jsonb,
    resource_version bigint NOT NULL,
    trace boolean DEFAULT false NOT NULL
);


ALTER TABLE public.ie_ag_ag_rules OWNER TO netguard;

--
-- Name: k8s_metadata; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.k8s_metadata (
    resource_version bigint NOT NULL,
    labels jsonb DEFAULT '{}'::jsonb,
    annotations jsonb DEFAULT '{}'::jsonb,
    finalizers text[] DEFAULT '{}'::text[],
    conditions jsonb DEFAULT '[]'::jsonb,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    uid uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    generation bigint DEFAULT 1 NOT NULL,
    deletion_timestamp timestamp with time zone
);


ALTER TABLE public.k8s_metadata OWNER TO netguard;

--
-- Name: COLUMN k8s_metadata.deletion_timestamp; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.k8s_metadata.deletion_timestamp IS 'Timestamp when object deletion was requested. NULL means object is not being deleted.';


--
-- Name: k8s_metadata_resource_version_seq; Type: SEQUENCE; Schema: public; Owner: netguard
--

CREATE SEQUENCE public.k8s_metadata_resource_version_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.k8s_metadata_resource_version_seq OWNER TO netguard;

--
-- Name: k8s_metadata_resource_version_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: netguard
--

ALTER SEQUENCE public.k8s_metadata_resource_version_seq OWNED BY public.k8s_metadata.resource_version;


--
-- Name: netguard_db_ver; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.netguard_db_ver (
    id integer NOT NULL,
    version_id bigint NOT NULL,
    is_applied boolean NOT NULL,
    tstamp timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.netguard_db_ver OWNER TO netguard;

--
-- Name: netguard_db_ver_id_seq; Type: SEQUENCE; Schema: public; Owner: netguard
--

ALTER TABLE public.netguard_db_ver ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.netguard_db_ver_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: network_bindings; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.network_bindings (
    namespace public.namespace_name NOT NULL,
    name public.resource_name NOT NULL,
    network_namespace public.namespace_name NOT NULL,
    network_name public.resource_name NOT NULL,
    address_group_namespace public.namespace_name NOT NULL,
    address_group_name public.resource_name NOT NULL,
    resource_version bigint NOT NULL
);


ALTER TABLE public.network_bindings OWNER TO netguard;

--
-- Name: networks; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.networks (
    namespace public.namespace_name NOT NULL,
    name public.resource_name NOT NULL,
    network_items jsonb DEFAULT '[]'::jsonb NOT NULL,
    is_bound boolean DEFAULT false NOT NULL,
    binding_ref_namespace public.namespace_name,
    binding_ref_name public.resource_name,
    address_group_ref_namespace public.namespace_name,
    address_group_ref_name public.resource_name,
    resource_version bigint NOT NULL,
    cidr cidr NOT NULL,
    ready boolean DEFAULT false NOT NULL
);


ALTER TABLE public.networks OWNER TO netguard;

--
-- Name: COLUMN networks.cidr; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.networks.cidr IS 'Network CIDR block - must be unique across all networks';


--
-- Name: COLUMN networks.ready; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.networks.ready IS 'Boolean flag indicating Network Ready status (synced from k8s_metadata.conditions).
Used by trigger trg_update_ag_on_network_ready to fire ONLY when Network becomes Ready (FALSE→TRUE transition).
Application must update this field when Network.status.conditions contains {type:Ready,status:True}.';


--
-- Name: rule_s2s; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.rule_s2s (
    namespace public.namespace_name NOT NULL,
    name public.resource_name NOT NULL,
    traffic public.traffic_direction NOT NULL,
    resource_version bigint NOT NULL,
    service_local_ref jsonb NOT NULL,
    service_ref jsonb NOT NULL,
    ieagag_rule_refs jsonb DEFAULT '[]'::jsonb NOT NULL,
    trace boolean DEFAULT false
);


ALTER TABLE public.rule_s2s OWNER TO netguard;

--
-- Name: service_aliases; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.service_aliases (
    namespace public.namespace_name NOT NULL,
    name public.resource_name NOT NULL,
    service_namespace public.namespace_name NOT NULL,
    service_name public.resource_name NOT NULL,
    resource_version bigint NOT NULL
);


ALTER TABLE public.service_aliases OWNER TO netguard;

--
-- Name: services; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.services (
    namespace public.namespace_name NOT NULL,
    name public.resource_name NOT NULL,
    description text DEFAULT ''::text,
    ingress_ports jsonb DEFAULT '[]'::jsonb NOT NULL,
    resource_version bigint NOT NULL,
    address_groups jsonb DEFAULT '[]'::jsonb NOT NULL,
    aggregated_address_groups jsonb DEFAULT '[]'::jsonb NOT NULL
);


ALTER TABLE public.services OWNER TO netguard;

--
-- Name: COLUMN services.address_groups; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.services.address_groups IS 'NamespacedObjectReference[] - List of AddressGroups that belong exclusively to this Service (direct registration)';


--
-- Name: COLUMN services.aggregated_address_groups; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.services.aggregated_address_groups IS 'AddressGroupReference[] - Aggregated list of all AddressGroups from both spec.addressGroups and AddressGroupBindings with source tracking';


--
-- Name: sync_outbox; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.sync_outbox (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    resource_type character varying(50) NOT NULL,
    resource_id uuid NOT NULL,
    operation public.sync_operation NOT NULL,
    target_system public.target_system NOT NULL,
    payload jsonb NOT NULL,
    delta jsonb,
    affects_resources jsonb,
    status public.outbox_status DEFAULT 'PENDING'::public.outbox_status NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    max_retries integer DEFAULT 5 NOT NULL,
    next_retry_at timestamp with time zone,
    last_error text,
    error_category character varying(50),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    processed_at timestamp with time zone,
    resource_namespace character varying(253) NOT NULL,
    resource_name character varying(253) NOT NULL
);


ALTER TABLE public.sync_outbox OWNER TO netguard;

--
-- Name: TABLE sync_outbox; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TABLE public.sync_outbox IS 'Transactional outbox for resilient resource synchronization to external systems. Implements Outbox Pattern for at-least-once delivery guarantee with retry logic.';


--
-- Name: COLUMN sync_outbox.id; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.id IS 'Unique identifier for outbox entry';


--
-- Name: COLUMN sync_outbox.resource_type; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.resource_type IS 'Type of resource being synchronized (Host, AddressGroup, Network, Service, etc.)';


--
-- Name: COLUMN sync_outbox.resource_id; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.resource_id IS 'UUID of the resource being synchronized';


--
-- Name: COLUMN sync_outbox.operation; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.operation IS 'Sync operation: CREATE (new resource), UPDATE (modify existing), DELETE (remove resource)';


--
-- Name: COLUMN sync_outbox.target_system; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.target_system IS 'Target system: SGROUP (external sync) or INTERNAL (process resources)';


--
-- Name: COLUMN sync_outbox.payload; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.payload IS 'Full resource state in JSON format (for audit, idempotency checks, and rollback)';


--
-- Name: COLUMN sync_outbox.delta; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.delta IS 'Delta changes for UPDATE operations (stores only what changed to prevent lost updates)';


--
-- Name: COLUMN sync_outbox.affects_resources; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.affects_resources IS 'For process resources (HostBinding, NetworkBinding): list of affected entity resources in JSON format';


--
-- Name: COLUMN sync_outbox.status; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.status IS 'Current status: PENDING → PROCESSING → SUCCESS/FAILED_RETRYABLE → FAILED_PERMANENT';


--
-- Name: COLUMN sync_outbox.attempts; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.attempts IS 'Number of processing attempts (incremented on each retry, used for exponential backoff)';


--
-- Name: COLUMN sync_outbox.max_retries; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.max_retries IS 'Maximum number of retry attempts before marking as FAILED_PERMANENT (default: 5)';


--
-- Name: COLUMN sync_outbox.next_retry_at; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.next_retry_at IS 'Timestamp when to retry processing (NULL for immediate processing, uses exponential backoff: NOW() + 2^attempts seconds)';


--
-- Name: COLUMN sync_outbox.last_error; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.last_error IS 'Last error message if processing failed (for debugging and monitoring)';


--
-- Name: COLUMN sync_outbox.error_category; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.error_category IS 'Error category for classification: NETWORK, VALIDATION, TIMEOUT, CONFLICT, AUTH, etc.';


--
-- Name: COLUMN sync_outbox.created_at; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.created_at IS 'When the outbox entry was created (immutable)';


--
-- Name: COLUMN sync_outbox.updated_at; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.updated_at IS 'When the entry was last updated (modified on each attempt)';


--
-- Name: COLUMN sync_outbox.processed_at; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.processed_at IS 'When the entry was successfully processed (NULL if not yet completed)';


--
-- Name: COLUMN sync_outbox.resource_namespace; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.resource_namespace IS 'Kubernetes namespace of the resource. Used for querying entity tables by (namespace, name).
Fixes P0-1: Enables JOIN with entity tables instead of non-existent resource_id column.';


--
-- Name: COLUMN sync_outbox.resource_name; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.sync_outbox.resource_name IS 'Kubernetes name of the resource. Used for querying entity tables by (namespace, name).
Fixes P0-1: Enables JOIN with entity tables instead of non-existent resource_id column.';


--
-- Name: sync_status; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.sync_status (
    id integer DEFAULT 1 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sync_status_id_check CHECK ((id = 1))
);


ALTER TABLE public.sync_status OWNER TO netguard;

--
-- Name: k8s_metadata resource_version; Type: DEFAULT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.k8s_metadata ALTER COLUMN resource_version SET DEFAULT nextval('public.k8s_metadata_resource_version_seq'::regclass);


--
-- Data for Name: address_group_binding_policies; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.address_group_binding_policies (namespace, name, resource_version, address_group_ref, service_ref) FROM stdin;
\.


--
-- Data for Name: address_group_bindings; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.address_group_bindings (namespace, name, service_namespace, service_name, address_group_namespace, address_group_name, resource_version) FROM stdin;
\.


--
-- Data for Name: address_group_port_mappings; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.address_group_port_mappings (namespace, name, access_ports, resource_version) FROM stdin;
incloud-sgroups	bug009-ag-test	{}	255
incloud-sgroups	p0-fix-test-ag	{"incloud-sgroups/service-ready-fix-test": {"Ports": {"TCP": [{"End": 9999, "Start": 9999}]}}}	161
incloud-sgroups	test-ag-namespace-fix	{"incloud-sgroups/test-svc-namespace-fix": {"Ports": {"TCP": [{"End": 8080, "Start": 8080}]}}}	274
incloud-sgroups	test-ag-001	{"incloud-sgroups/test-svc-001": {"Ports": {"TCP": [{"End": 8080, "Start": 8080}]}}}	277
incloud-sgroups	test-ag-sgroup-001	{"incloud-sgroups/test-svc-sgroup-001": {"Ports": {"TCP": [{"End": 8080, "Start": 8080}, {"End": 9090, "Start": 9090}]}}}	305
\.


--
-- Data for Name: address_groups; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.address_groups (namespace, name, default_action, logs, trace, description, resource_version, networks, hosts, aggregated_hosts) FROM stdin;
incloud-sgroups	test-ag-sgroup-001	ACCEPT	f	f		303	[]	[]	[]
default	example	DROP	t	t		106	[]	[]	[]
incloud-sgroups	p0-fix-test-ag	ACCEPT	f	f		127	[]	[{"kind": "Host", "name": "dynamic-199ee517b0763868", "apiVersion": "netguard.sgroups.io/v1beta1"}]	[]
incloud-sgroups	e2e-scenario2-ag	ACCEPT	f	f		187	[]	[{"kind": "Host", "name": "e2e-scenario2-host", "apiVersion": "netguard.sgroups.io/v1beta1"}]	[]
incloud-sgroups	test-ag-namespace-fix	ACCEPT	f	f		272	[]	[]	[]
incloud-sgroups	test-ag-001	ACCEPT	f	f		275	[]	[]	[]
incloud-sgroups	bug009-final-ag	ACCEPT	f	f		284	[]	[]	[]
\.


--
-- Data for Name: host_bindings; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.host_bindings (namespace, name, host_namespace, host_name, address_group_namespace, address_group_name, resource_version) FROM stdin;
incloud-sgroups	dynamic-199f0f2a8626e714	incloud-sgroups	dynamic-199ee3b35bf19eb0	incloud-sgroups	e2e-scenario2-ag	223
\.


--
-- Data for Name: hosts; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.hosts (namespace, name, uuid, host_name_sync, address_group_name, is_bound, binding_ref_namespace, binding_ref_name, address_group_ref_namespace, address_group_ref_name, resource_version, ip_list, ready) FROM stdin;
incloud-sgroups	e2e-scenario2-host	22222222-3333-4444-5555-666666666602	\N	\N	t	\N	\N	incloud-sgroups	e2e-scenario2-ag	188	\N	t
default	dynamic-199e96ff364a3802	b0607910-0c16-4851-bba7-cd7807d1631e	\N	\N	f	\N	\N	\N	\N	105	\N	t
incloud-sgroups	to-nft-agent-jd9qk	f2aca337-fb77-4fa1-80cf-b66e2216002f	\N	\N	f	\N	\N	\N	\N	307	[{"ip": "fe80::d848:f8ff:fe7a:c2aa"}, {"ip": "127.0.0.1"}, {"ip": "::1"}, {"ip": "10.244.0.8"}]	t
incloud-sgroups	sgroup-test-available	bbbbbbbb-1111-2222-3333-444444444001	\N	\N	f	\N	\N	\N	\N	109	\N	t
incloud-sgroups	sgroup-test-unavailable	cccccccc-1111-2222-3333-444444444002	\N	\N	f	\N	\N	\N	\N	110	\N	t
incloud-sgroups	test-with-sgroup-down	550e8400-aaaa-aaaa-aaaa-446655440099	\N	\N	f	\N	\N	\N	\N	118	\N	t
incloud-sgroups	dynamic-199ee3b35bf19eb0	38fcf701-5a6d-4788-a836-587acd3d35b8	\N	incloud-sgroups/e2e-scenario2-ag	t	incloud-sgroups	dynamic-199f0f2a8626e714	incloud-sgroups	e2e-scenario2-ag	224	\N	t
incloud-sgroups	dynamic-199ee517b0763868	08039aa6-0f79-40f7-a718-99379802abdd	\N	\N	t	\N	\N	incloud-sgroups	p0-fix-test-ag	162	\N	t
\.


--
-- Data for Name: ie_ag_ag_rules; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.ie_ag_ag_rules (namespace, name, transport, traffic, action, address_group_local_namespace, address_group_local_name, address_group_namespace, address_group_name, ports, resource_version, trace) FROM stdin;
\.


--
-- Data for Name: k8s_metadata; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.k8s_metadata (resource_version, labels, annotations, finalizers, conditions, created_at, updated_at, uid, generation, deletion_timestamp) FROM stdin;
135	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-16T09:47:31Z"}]	2025-10-16 09:47:31.666179+00	2025-10-16 09:47:31.666179+00	0a9a1211-2b4a-40c5-8a43-8932bf024915	1	\N
157	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "NetworkBinding committed to backend successfully", "lastTransitionTime": "2025-10-16T18:08:44Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "NetworkBinding passed validation", "lastTransitionTime": "2025-10-16T18:08:44Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "NetworkBinding is ready for use", "lastTransitionTime": "2025-10-16T18:08:44Z"}]	2025-10-16 18:08:44.238793+00	2025-10-16 18:08:44.238793+00	7ba6aeb2-b608-402d-8c5a-5c246a7504c2	1	\N
222	{}	{}	{}	null	2025-10-17 06:54:18.723283+00	2025-10-17 06:54:18.723283+00	13a7ae4a-7c18-455f-a63d-ee2a0fb2deba	1	\N
97	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-42sgv\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"d09112e9-8031-4906-9219-31196d4c6b14\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:13:32Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:13:32Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:13:42Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 14:13:32.252817+00	2025-10-15 14:13:42.246866+00	c3afdb34-8a31-4ff2-a9e3-cb367548e43c	1	2025-10-15 14:13:42.252901+00
8	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-42sgv\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"d09112e9-8031-4906-9219-31196d4c6b14\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-11T22:58:03Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-11T22:58:03Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-11T22:58:03Z"}]	2025-10-11 22:58:03.163429+00	2025-10-11 22:58:03.163429+00	f3115e4a-2817-412f-9f6e-c978e1d6c473	1	\N
1	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Address group is ready and operational", "lastTransitionTime": "2025-10-10T14:18:15Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Address group successfully synced to backend and SGROUP", "lastTransitionTime": "2025-10-10T14:18:15Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-10T14:18:15Z"}]	2025-10-10 14:18:15.395388+00	2025-10-10 14:18:17.424884+00	6ab7b126-450c-4553-b58d-8c6ec65dc93f	1	2025-10-14 16:47:30.760144+00
30	{}	{}	{}	null	2025-10-15 07:52:22.472744+00	2025-10-15 07:52:22.472744+00	31e3d8ac-2398-4b28-a7cd-3f3728926c59	1	\N
57	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T09:11:37Z"}]	2025-10-15 09:11:37.995309+00	2025-10-15 09:11:37.995309+00	c040465b-94be-49c1-9eea-b2a759797cc3	1	\N
54	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T09:02:34Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:03:12Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 09:02:34.718696+00	2025-10-15 14:03:12.223238+00	42caf040-67a3-4e07-a768-494954946e47	1	2025-10-15 14:03:12.239254+00
2	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-2qxp7\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"22a58fef-2022-45a3-bc0d-eaa1643ab694\\"}}\\n"}	{}	[null, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-10 15:31:06.627769+00	2025-10-10 15:31:06.627769+00	677a5c3d-ddb3-47fd-9f55-7e09398e228f	1	2025-10-15 14:10:42.249059+00
18	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-14T15:20:01Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-14T15:20:01Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-14T15:20:01Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:11:52Z"}]	2025-10-14 15:20:01.19586+00	2025-10-15 14:11:52.238281+00	1a18825d-6276-4fa8-bc83-cb79139b3709	1	2025-10-14 15:22:51.409502+00
7	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-42sgv\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"d09112e9-8031-4906-9219-31196d4c6b14\\"}}\\n"}	{}	[null, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-11 22:58:03.146276+00	2025-10-11 22:58:03.146276+00	88d84d6e-9afc-448f-a5a4-7becea73aa81	1	2025-10-15 14:13:32.252817+00
6	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-kpk8w\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"49c2dce9-23de-45af-8090-55bb367765cc\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-10T23:13:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-10T23:13:06Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-10T23:13:06Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:40:02Z"}]	2025-10-10 23:13:10.014263+00	2025-10-15 14:40:02.254031+00	e84c345c-1311-458a-a507-96cbe3c0c94f	1	2025-10-11 22:57:20.1527+00
9	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-42sgv\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"d09112e9-8031-4906-9219-31196d4c6b14\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-11T22:58:03Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-11T22:58:03Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-11T22:58:03Z"}]	2025-10-11 22:58:07.679927+00	2025-10-11 22:58:07.679927+00	67c70e19-747c-42a6-8842-c5b88735eb3f	1	\N
10	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	null	2025-10-14 13:08:34.860722+00	2025-10-14 13:08:34.860722+00	2929e70e-efca-49e4-a831-6572f419620d	1	\N
11	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-14T13:08:34Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-14T13:08:34Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-14T13:08:34Z"}]	2025-10-14 13:08:34.92368+00	2025-10-14 13:08:34.92368+00	e390e116-9d32-4448-a7f4-94f7d71ee0d2	1	2025-10-14 13:12:44.081289+00
12	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-fix-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"00000000-0000-0000-0000-000000000999\\"}}\\n"}	{}	null	2025-10-14 14:19:53.557725+00	2025-10-14 14:19:53.557725+00	686cbbf2-6800-407c-8724-7c2f5b34f067	1	\N
13	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-fix-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"00000000-0000-0000-0000-000000000999\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-14T14:19:53Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-14T14:19:53Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-14T14:19:53Z"}]	2025-10-14 14:19:53.567775+00	2025-10-14 14:19:53.567775+00	3a593df7-ccbd-4d0e-b8d9-e79441102b67	1	2025-10-14 14:23:59.267443+00
56	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T09:10:38Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 09:10:38.345733+00	2025-10-15 09:14:56.206611+00	7a917fe8-8fad-439e-921f-e07b64817faa	1	2025-10-15 09:14:56.210772+00
15	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-host-031\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440031\\"}}\\n"}	{}	null	2025-10-14 15:10:56.799521+00	2025-10-14 15:10:56.799521+00	eef3030a-c52a-47ca-bbbb-976ecb8046fa	1	\N
16	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-host-031\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440031\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-14T15:10:56Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-14T15:10:56Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-14T15:10:56Z"}]	2025-10-14 15:10:56.804059+00	2025-10-14 15:10:56.804059+00	23b54b4e-3438-4b11-93eb-812ed505aa14	1	\N
71	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T12:27:41Z"}]	2025-10-15 12:27:41.014718+00	2025-10-15 12:27:41.014718+00	86574ad1-74d8-4be5-bac2-58262a6230bd	1	\N
28	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T07:38:24Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T07:38:24Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T07:38:21Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:39:04Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:39:04Z"}]	2025-10-15 07:38:21.497743+00	2025-10-15 07:39:04.486122+00	c9265ebe-5260-489c-9514-20739d21f0c7	1	2025-10-15 07:38:56.092245+00
29	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T07:41:54Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T07:41:54Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T07:40:29Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-15T07:41:54Z"}]	2025-10-15 07:40:29.668795+00	2025-10-15 07:41:54.595218+00	75535b62-2443-44e2-8a77-ec845384efd6	1	\N
49	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T08:34:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:35:16Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T08:34:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:35:16Z"}]	2025-10-15 08:34:56.525071+00	2025-10-15 08:35:16.50557+00	880d0e7b-d293-4f1e-85ba-30c7bd02d4fb	1	2025-10-15 08:35:06.517249+00
22	{"test": "infinite-loop-fix", "app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"test\\":\\"infinite-loop-fix\\"},\\"name\\":\\"test-ag-new-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-14T16:43:01Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-14T16:43:01Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-14T16:42:54Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-14T16:43:01Z"}]	2025-10-14 16:42:54.568439+00	2025-10-14 16:43:01.356603+00	0765e35a-dfc6-4684-893b-cce5bd103aaf	1	\N
31	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T07:52:22Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T07:52:22Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-15T07:52:22Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:52:24Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 07:52:22.487213+00	2025-10-15 07:52:24.495019+00	41d160b4-9e55-48f7-83cd-2cc36a17edf6	1	2025-10-15 07:52:24.502847+00
99	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-2222-3333-4444-555555555001\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T14:50:43Z"}]	2025-10-15 14:50:43.230392+00	2025-10-15 14:50:43.230392+00	eca39f38-3e98-4e65-800a-867b6bff521b	1	\N
223	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "HostBinding committed to backend successfully", "lastTransitionTime": "2025-10-17T06:54:18Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "HostBinding passed validation", "lastTransitionTime": "2025-10-17T06:54:18Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "HostBinding is ready for use", "lastTransitionTime": "2025-10-17T06:54:18Z"}]	2025-10-17 06:54:18.731909+00	2025-10-17 06:54:18.731909+00	e5eb1386-f98f-4a7f-856d-8569d63c6dcf	1	\N
55	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T09:02:34Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T09:02:34Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T09:02:34Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:14:56Z"}]	2025-10-15 09:02:34.735991+00	2025-10-15 09:14:56.213404+00	8403af42-acc8-498c-be35-dbfd1567f4fc	1	2025-10-15 09:11:11.496638+00
32	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T07:52:24Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T07:52:22Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T07:52:24Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:52:44Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:52:44Z"}]	2025-10-15 07:52:24.502847+00	2025-10-15 07:52:44.495898+00	55974f94-b853-4e4e-9f16-af83870a664c	1	2025-10-15 07:52:34.508929+00
43	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T08:07:56Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T08:07:56Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T08:07:47Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:08:16Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:08:16Z"}]	2025-10-15 08:07:56.504148+00	2025-10-15 08:08:16.498672+00	8be4d9da-b4d8-46f1-b1d4-88aa48bc3c93	1	2025-10-15 08:08:06.509957+00
45	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T08:08:56Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T08:08:56Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T08:08:51Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-15T08:08:56Z"}]	2025-10-15 08:08:51.899178+00	2025-10-15 08:08:56.512243+00	2a8bd906-ea2d-4064-8c90-5b3237f64790	1	\N
62	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T11:02:13Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:04:48Z"}]	2025-10-15 11:02:13.779102+00	2025-10-15 11:04:48.023486+00	f1a5b870-d14f-4318-bb10-39fe447aaf6e	1	\N
21	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-14T15:53:29Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-14T15:53:12Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-14T15:53:29Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-14T15:53:29Z"}]	2025-10-14 15:53:29.248788+00	2025-10-14 15:53:29.250691+00	854cf576-22b1-400d-8a1c-74272d6700e1	1	2025-10-14 15:54:27.479285+00
23	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-resilient-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-14T16:48:41Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-14T16:48:41Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-14T16:48:34Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-14T16:48:41Z"}]	2025-10-14 16:48:34.357433+00	2025-10-14 16:48:41.377661+00	8796ae50-34b1-4720-8725-8e945fad3076	1	2025-10-14 16:53:27.903181+00
24	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-resilient-002\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-14T16:52:31Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-14T16:52:31Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-14T16:49:19Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-14T16:52:31Z"}]	2025-10-14 16:49:19.248171+00	2025-10-14 16:52:31.390966+00	c71a1a34-0bc9-47db-8985-fcea1cf67673	1	2025-10-14 16:53:27.954743+00
25	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-resilient-003\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"DROP\\",\\"logs\\":true,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-14T16:52:31Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-14T16:52:31Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-14T16:49:20Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-14T16:52:31Z"}]	2025-10-14 16:49:20.059068+00	2025-10-14 16:52:31.395713+00	d4a51648-c1fe-49e7-ac24-e03b9e40868e	1	2025-10-14 16:53:27.997348+00
27	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-resilient-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440010\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-14T16:52:31Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-14T16:52:31Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-14T16:52:31Z"}]	2025-10-14 16:52:31.399404+00	2025-10-14 16:52:31.401453+00	7d5cec0b-319a-4e73-996c-de6a6df6f15a	1	2025-10-14 16:53:28.039264+00
50	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T08:46:56Z"}]	2025-10-15 08:46:56.579745+00	2025-10-15 08:46:56.579745+00	2b0cbf91-7ea1-423b-88d8-f4a56014bea3	1	\N
33	{}	{}	{}	null	2025-10-15 07:53:11.951549+00	2025-10-15 07:53:11.951549+00	6609b30d-70ca-4e57-9186-da9499bbc956	1	\N
113	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ready-bug-v2\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"22222222-3333-4444-5555-666666666666\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T22:04:35Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T22:04:35Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-15T22:04:35Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T22:07:59Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T22:07:59Z"}]	2025-10-15 22:04:35.972325+00	2025-10-15 22:07:59.506968+00	fca795fb-8e29-4b51-af70-e962279c088a	1	2025-10-15 22:07:15.043545+00
26	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-resilient-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440010\\"}}\\n"}	{}	null	2025-10-14 16:49:20.845073+00	2025-10-14 16:49:20.845073+00	a4dcec85-7fdc-49f9-9d87-35703d488f57	1	\N
44	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T08:08:46Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T08:08:46Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T08:08:36Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-15T08:08:46Z"}]	2025-10-15 08:08:36.584648+00	2025-10-15 08:08:46.514543+00	90cdcdca-9424-4f9a-95a3-83b2447da100	1	\N
20	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-14T15:53:12Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-14T15:53:12Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-14T15:53:12Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-14T15:53:29Z"}]	2025-10-14 15:53:12.213535+00	2025-10-14 15:53:29.24533+00	802bb30a-1616-4bbb-93cd-b2123ff2c840	1	\N
34	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T07:53:11Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T07:53:11Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-15T07:53:11Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:53:14Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 07:53:11.963492+00	2025-10-15 07:53:14.495505+00	6f8a2b91-0290-4db8-8002-ed5cc8e2ec3f	1	2025-10-15 07:53:14.502672+00
136	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-16T09:47:31Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T09:47:31Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:47:37Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-16 09:47:31.674489+00	2025-10-16 09:47:37.596669+00	4a18f063-6880-4229-836f-dfeca42512a3	1	2025-10-16 09:47:37.614466+00
35	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T07:53:14Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T07:53:11Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T07:53:14Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:53:34Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:53:34Z"}]	2025-10-15 07:53:14.502672+00	2025-10-15 07:53:34.495982+00	aec6bb45-2107-4a93-b664-b128ecfead5e	1	2025-10-15 07:53:24.510227+00
59	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:15:16Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:15:16Z"}]	2025-10-15 09:14:56.197641+00	2025-10-15 09:15:16.178932+00	156f73b0-8be2-4bb1-a736-597133ea843d	1	2025-10-15 09:15:06.201681+00
48	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T08:31:17Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:34:56Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 08:31:17.612431+00	2025-10-15 08:34:56.521732+00	b62aef20-bfd8-4778-866f-2a2295e535c2	1	2025-10-15 08:34:56.525071+00
60	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:15:16Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:15:16Z"}]	2025-10-15 09:14:56.210772+00	2025-10-15 09:15:16.187711+00	63cb7eff-01d7-4682-b59e-e84192221138	1	2025-10-15 09:15:06.21007+00
51	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T08:46:56Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T08:46:56Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-15T08:46:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:47:06Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 08:46:56.591938+00	2025-10-15 08:47:06.503391+00	2e678ebf-36b5-4f4a-ac32-2c85d36aac7b	1	2025-10-15 08:47:06.512734+00
19	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	null	2025-10-14 15:53:12.204579+00	2025-10-14 15:53:12.204579+00	48962faa-c371-42ca-809c-0d965c37e31a	1	\N
58	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T09:11:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T09:11:38Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T09:11:38Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 09:11:38.000771+00	2025-10-15 09:14:56.218936+00	7059d37e-ac32-43f0-ad0b-6d47c3e77994	1	2025-10-15 09:14:56.22349+00
90	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:00:42Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T14:00:38Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:00:42Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:00:42Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 14:00:42.22389+00	2025-10-15 14:00:42.2273+00	d7ccb2ae-946a-4913-9cca-37ec241e2778	1	2025-10-15 14:00:42.231561+00
52	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T08:47:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T08:46:56Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T08:47:06Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:47:26Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:47:26Z"}]	2025-10-15 08:47:06.512734+00	2025-10-15 08:47:26.50385+00	6d2178a3-c310-48e3-ad0e-1a188019f714	1	2025-10-15 08:47:16.514998+00
91	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:00:42Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T14:00:38Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:00:42Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:00:42Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 14:00:42.231561+00	2025-10-15 14:00:42.234975+00	707fa024-1825-473d-8b21-c43bd4513a1f	1	2025-10-15 14:00:42.238234+00
61	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T09:11:38Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:15:16Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:15:16Z"}]	2025-10-15 09:14:56.22349+00	2025-10-15 09:15:16.193492+00	608fea93-f246-4b1e-a9f3-956ee01fdf84	1	2025-10-15 09:15:06.215907+00
63	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T11:02:54Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T11:02:54Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:04:48Z"}]	2025-10-15 11:02:54.181449+00	2025-10-15 11:04:48.025258+00	b417f972-a1ab-42e0-b998-4f92b9a183b2	1	\N
72	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T12:27:41Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T12:27:41Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T12:27:41Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:27:50Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 12:27:41.02136+00	2025-10-15 12:27:50.086604+00	c5494ccc-77fa-4c47-b233-3114a0655fce	1	2025-10-15 12:27:50.126809+00
3	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-2qxp7\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"22a58fef-2022-45a3-bc0d-eaa1643ab694\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-10T15:31:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-10T15:31:06Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-10T15:31:06Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:11:02Z"}]	2025-10-10 15:31:06.674209+00	2025-10-15 14:11:02.237057+00	4bde61d1-0219-4f60-9a96-42b83ce63ce9	1	2025-10-10 23:12:18.98962+00
38	{}	{}	{}	null	2025-10-15 07:53:54.479079+00	2025-10-15 07:53:54.479079+00	537b8d89-eb05-4a80-8fc5-4d62bd9b4ade	1	\N
66	{"app": "condition-manager-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"condition-manager-test\\"},\\"name\\":\\"test-network-ready-false\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.200.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T11:15:28Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T11:15:28Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T11:15:28Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:16:38Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 11:15:28.219745+00	2025-10-15 11:16:38.011506+00	3ce01e5c-a162-42b4-a8ae-48d3e9e37933	1	2025-10-15 11:16:38.034133+00
39	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 07:53:54.488311+00	2025-10-15 07:53:54.494917+00	cbd2d62d-1f43-4779-a349-81964fa74fa1	1	2025-10-15 07:53:54.502851+00
125	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-fix-test-network\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.200.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-16T09:07:10Z"}]	2025-10-16 09:07:10.443742+00	2025-10-16 09:07:10.443742+00	c282936e-a8ee-44e8-9ec1-282deaf71b3a	1	\N
67	{"app": "condition-manager-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"condition-manager-test\\"},\\"name\\":\\"test-network-ready-false\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.200.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T11:16:38Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T11:16:38Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T11:15:28Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:16:58Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:16:58Z"}]	2025-10-15 11:16:38.034133+00	2025-10-15 11:16:58.015784+00	e79eab83-5a70-45b7-8d1f-8fd74831a9f6	1	2025-10-15 11:16:48.026069+00
46	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T08:34:56Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T08:34:56Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T08:09:54Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-15T08:34:56Z"}]	2025-10-15 08:09:54.226001+00	2025-10-15 08:34:56.516119+00	bf3d8bc0-92f8-415e-8e34-dc8ad0aa84be	1	\N
74	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T12:28:34Z"}]	2025-10-15 12:28:34.680018+00	2025-10-15 12:28:34.680018+00	9adede4d-7580-48db-98ee-0b161a932875	1	\N
75	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T12:28:34Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T12:28:34Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T12:28:34Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:28:40Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 12:28:34.683029+00	2025-10-15 12:28:40.090485+00	dd1c21ec-e09f-4bbc-b85d-7f8b1b9cb734	1	2025-10-15 12:28:40.112705+00
53	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T08:47:58Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 08:47:58.568646+00	2025-10-15 09:14:56.185216+00	7895a9e3-aab7-44aa-af92-004c79c87202	1	2025-10-15 09:14:56.197641+00
40	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:54:14Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:54:14Z"}]	2025-10-15 07:53:54.502851+00	2025-10-15 07:54:14.495494+00	8a9dd176-026b-4d0d-be0b-3c96e8048486	1	2025-10-15 07:54:04.504191+00
65	{"app": "condition-manager-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"condition-manager-test\\"},\\"name\\":\\"test-network-ready-false\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.200.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T11:15:28Z"}]	2025-10-15 11:15:28.195638+00	2025-10-15 11:15:28.195638+00	d54c4215-d45a-4d71-b3a0-ac05220ec80a	1	\N
17	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	[null, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-14 15:20:01.189838+00	2025-10-14 15:20:01.189838+00	5a78bfc3-4ddf-44d6-b03c-7251b2e29407	1	2025-10-15 13:59:52.239542+00
175	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"uid-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-16T22:58:23Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T22:58:23Z"}]	2025-10-16 22:58:23.664203+00	2025-10-16 22:58:23.6668+00	2e962f2f-36ce-4381-812f-e82162c3b4d0	1	\N
68	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T11:34:39Z"}]	2025-10-15 11:34:39.333981+00	2025-10-15 11:34:39.333981+00	f18cfe23-469d-4e44-a8e5-95d839769721	1	\N
14	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-031\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T08:34:56Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T08:34:56Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T08:18:46Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-15T08:34:56Z"}]	2025-10-14 15:10:56.771655+00	2025-10-15 08:34:56.521365+00	48eabafa-2839-4145-a3f6-64c84b30269e	1	\N
94	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:03:22Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:03:32Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:03:22Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:03:32Z"}]	2025-10-15 14:03:22.232717+00	2025-10-15 14:03:32.22022+00	07f51ac1-5458-4bc0-bc74-3583f718756d	1	2025-10-15 14:03:22.247143+00
69	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T11:34:39Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T11:34:39Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T11:34:39Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:34:48Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 11:34:39.344975+00	2025-10-15 11:34:48.016195+00	3e2d6695-d87c-467d-9de0-ca7d47de3b07	1	2025-10-15 11:34:48.028041+00
76	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T12:28:40Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T12:28:34Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T12:28:40Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:28:40Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 12:28:40.112705+00	2025-10-15 12:28:40.121173+00	1d4783c9-3634-40de-a4d9-cb4fe9e8a236	1	2025-10-15 12:28:40.189668+00
41	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T08:07:47Z"}]	2025-10-15 08:07:47.142281+00	2025-10-15 08:07:47.142281+00	38d5de01-8fe8-4601-96cc-80e41696fe42	1	\N
86	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T13:59:52Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T13:59:52Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:00:02Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 13:59:52.239542+00	2025-10-15 14:00:02.225254+00	22bac9f5-411a-472d-9941-e958ea552633	1	2025-10-15 14:00:02.233922+00
42	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T08:07:47Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T08:07:47Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T08:07:47Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:07:56Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 08:07:47.152708+00	2025-10-15 08:07:56.495691+00	096db4bf-6720-4f4a-bde3-ece4c7636baa	1	2025-10-15 08:07:56.504148+00
70	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T11:34:48Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T11:34:48Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T11:34:39Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:35:08Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:35:08Z"}]	2025-10-15 11:34:48.028041+00	2025-10-15 11:35:08.022545+00	1d980c13-856a-447b-b704-ec344f688ecf	1	2025-10-15 11:34:58.022789+00
73	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T12:27:50Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T12:27:41Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T12:27:50Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:28:10Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:28:10Z"}]	2025-10-15 12:27:50.126809+00	2025-10-15 12:28:10.099412+00	1535759d-4adc-4efb-89e5-172e7e6f6b50	1	2025-10-15 12:28:00.098051+00
87	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:00:02Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:00:02Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:00:12Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:00:12Z"}]	2025-10-15 14:00:02.233922+00	2025-10-15 14:00:12.227687+00	28d1cb94-620a-44fb-b0b1-fe289bb35c7b	1	2025-10-15 14:00:02.245866+00
92	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:00:42Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T14:00:38Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:00:42Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:00:52Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:00:52Z"}]	2025-10-15 14:00:42.238234+00	2025-10-15 14:00:52.240945+00	fda85b75-0558-4bd4-b4ab-01afb8e3396e	1	2025-10-15 14:00:52.237675+00
93	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:03:12Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:03:22Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:03:12Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 14:03:12.239254+00	2025-10-15 14:03:22.222135+00	bd2d08e3-480b-4056-b413-81c36540762a	1	2025-10-15 14:03:22.232717+00
77	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T12:28:40Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T12:28:34Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T12:28:40Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:28:50Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:28:50Z"}]	2025-10-15 12:28:40.189668+00	2025-10-15 12:28:50.101918+00	3ffba11b-cb77-4080-8b89-5d94a9865073	1	2025-10-15 12:28:50.096443+00
88	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T14:00:37Z"}]	2025-10-15 14:00:37.992397+00	2025-10-15 14:00:37.992397+00	917dce38-8ef4-43e9-a2d4-09b02bcfae8a	1	\N
100	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-2222-3333-4444-555555555001\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T14:50:43Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T14:50:43Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-15T14:50:43Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:50:51Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 14:50:43.235994+00	2025-10-15 14:50:51.579954+00	26e6df6b-8184-4ad2-977c-fc78e91c0c57	1	2025-10-15 14:50:51.594622+00
89	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T14:00:38Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T14:00:38Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T14:00:38Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:00:42Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 14:00:38.005118+00	2025-10-15 14:00:42.214677+00	adf15a90-9c42-4969-baa8-c01d44d64c28	1	2025-10-15 14:00:42.22389+00
95	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-2qxp7\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"22a58fef-2022-45a3-bc0d-eaa1643ab694\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:10:42Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:10:42Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:10:42Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 14:10:42.249059+00	2025-10-15 14:10:42.257498+00	2528e78c-9d3b-496c-9d5a-e2cdeab8c5ce	1	2025-10-15 14:10:42.262844+00
96	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-2qxp7\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"22a58fef-2022-45a3-bc0d-eaa1643ab694\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:10:42Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:10:42Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:10:52Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:10:52Z"}]	2025-10-15 14:10:42.262844+00	2025-10-15 14:10:52.233616+00	8689ac0f-4c3e-4ebe-88ef-aa27e55b5aca	1	2025-10-15 14:10:45.362439+00
4	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-kpk8w\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"49c2dce9-23de-45af-8090-55bb367765cc\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:11:22Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:11:22Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:11:32Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:11:32Z"}]	2025-10-10 23:13:05.973742+00	2025-10-15 14:11:32.232572+00	807d58c1-92dc-4dc1-93ee-52f65035880a	1	2025-10-15 14:11:24.790229+00
5	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-kpk8w\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"49c2dce9-23de-45af-8090-55bb367765cc\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-10T23:13:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-10T23:13:06Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-10T23:13:06Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:11:42Z"}]	2025-10-10 23:13:06.003458+00	2025-10-15 14:11:42.241526+00	c564d93e-079a-4706-a602-45d9c3d27f4c	1	2025-10-15 14:11:41.460428+00
169	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"uid-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-16T22:40:59Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T22:40:59Z"}]	2025-10-16 22:40:59.612869+00	2025-10-16 22:40:59.626253+00	9cc22601-7b34-4585-9876-c318d40a6930	1	\N
98	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-42sgv\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"d09112e9-8031-4906-9219-31196d4c6b14\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:13:42Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:13:42Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:13:52Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:13:52Z"}]	2025-10-15 14:13:42.252901+00	2025-10-15 14:13:52.252458+00	5083cb3e-7aa8-442b-88c1-d4af6fa8f0d4	1	2025-10-15 14:13:42.262579+00
104	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-validation-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T19:49:59Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T19:49:58Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T19:49:59Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T19:51:29Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T19:51:29Z"}]	2025-10-15 19:49:58.413378+00	2025-10-15 19:51:29.354897+00	3388efe8-737e-4c5f-8ae8-0d0aa78bcc82	1	2025-10-15 19:51:25.569336+00
105	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T19:54:19Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T19:54:12Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T19:54:19Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-15T19:54:19Z"}]	2025-10-15 19:54:12.196695+00	2025-10-15 19:54:19.375183+00	b0117d41-5ef2-499e-a421-755f29880a30	1	\N
101	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-2222-3333-4444-555555555001\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:50:51Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T14:50:43Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:50:51Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:50:51Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 14:50:51.594622+00	2025-10-15 14:50:51.602196+00	73ae9ebc-7dc8-473e-b6bc-e5553e983e41	1	2025-10-15 14:50:51.612083+00
137	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:47:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T09:47:31Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:47:37Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:47:37Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-16 09:47:37.614466+00	2025-10-16 09:47:37.619524+00	8014a1f7-beba-46e4-8696-8c371f9a1e65	1	2025-10-16 09:47:37.623102+00
102	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-2222-3333-4444-555555555001\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T14:50:51Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T14:50:43Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T14:50:51Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:51:11Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T14:51:11Z"}]	2025-10-15 14:50:51.612083+00	2025-10-15 14:51:11.578914+00	d0e36519-aa7c-4cdd-99c4-089f4b0f124a	1	2025-10-15 14:51:01.594053+00
103	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"99999999-9999-9999-9999-999999999999\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T19:38:36Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T19:38:27Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T19:38:36Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T19:39:46Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T19:39:46Z"}]	2025-10-15 19:38:27.793284+00	2025-10-15 19:39:46.267629+00	b77db5c2-2a20-4467-b795-f549a72fd285	1	2025-10-15 19:39:45.938249+00
121	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"debug-final-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"aaa00000-1111-2222-3333-444455556666\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T23:05:47Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T11:41:24Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T11:41:24Z"}]	2025-10-15 23:05:47.758932+00	2025-10-16 11:41:24.762907+00	f21e67ab-d7ee-4e0e-94bd-4f0fd227379f	1	2025-10-16 11:41:17.382812+00
111	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"ready-condition-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-2222-3333-4444-555555555999\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T21:44:49Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T21:44:44Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T21:44:49Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T21:46:09Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T21:46:09Z"}]	2025-10-15 21:44:44.816024+00	2025-10-15 21:46:09.462609+00	8d3c9a83-398b-468a-8b01-49402a774150	1	2025-10-15 21:46:02.484305+00
106	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T20:00:39Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T20:00:39Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T20:00:35Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-15T20:00:39Z"}]	2025-10-15 19:54:20.743669+00	2025-10-15 20:00:39.39488+00	5808a408-e3e4-4530-9bce-d61a975156f3	1	\N
109	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"sgroup-test-available\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"bbbbbbbb-1111-2222-3333-444444444001\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T20:43:19Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T20:43:13Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T20:43:19Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-15T20:43:19Z"}]	2025-10-15 20:43:13.97716+00	2025-10-15 20:43:19.426472+00	f7189977-74fe-4da2-a69a-d0e44e8acbbc	1	\N
119	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"debug-test-logging-trace\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-cccc-cccc-cccc-446655440099\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T22:54:43Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T22:54:43Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-15T22:54:43Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:42:37Z"}]	2025-10-15 22:54:43.667605+00	2025-10-16 09:42:37.639923+00	494008af-617a-4bc2-b328-630f7cfddd9e	1	2025-10-15 23:05:27.01844+00
114	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-baseline\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T22:12:24Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T22:12:24Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T22:14:19Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T22:14:19Z"}]	2025-10-15 22:12:24.367064+00	2025-10-15 22:14:19.524483+00	1eb79e59-4416-41f4-a694-a01ea303e9e3	1	2025-10-15 22:14:07.272796+00
116	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ready-bug-fix-v3\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440099\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T22:38:16Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T22:38:16Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-15T22:38:16Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T22:43:32Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T22:43:32Z"}]	2025-10-15 22:38:16.214611+00	2025-10-15 22:43:32.975442+00	fcaded22-989e-4f95-a1c8-a307b9b8afb6	1	2025-10-15 22:41:46.042368+00
138	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:47:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T09:47:31Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:47:37Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:47:37Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-16 09:47:37.623102+00	2025-10-16 09:47:37.62537+00	0f4e14ec-9b3b-42e6-8a2d-4e48cf32c750	1	2025-10-16 09:47:37.628284+00
142	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"e2e00000-0000-0000-0000-000000000001\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T10:16:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T10:13:47Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T10:16:06Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T10:16:06Z"}]	2025-10-16 10:13:47.288471+00	2025-10-16 10:16:06.527877+00	3c94328e-7753-433c-93cb-4dddc93d4533	1	\N
191	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug006-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"33333333-4444-5555-6666-777777777706\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:28:40Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:28:40Z"}]	2025-10-17 06:28:40.637289+00	2025-10-17 06:28:40.652233+00	62148b36-a5d4-48a4-865c-d3c054f23162	1	\N
139	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:47:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T09:47:31Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:47:37Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:47:37Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-16 09:47:37.628284+00	2025-10-16 09:47:37.629863+00	b5930ef6-d586-4660-aeb0-4dd9110ce606	1	2025-10-16 09:47:37.633767+00
122	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"trace-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440099\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T23:30:58Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T23:30:58Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:42:37Z"}]	2025-10-15 23:30:58.129486+00	2025-10-16 09:42:37.709024+00	1cf60fa8-9e9d-4054-8f66-97c530a14878	1	2025-10-16 09:04:02.152262+00
108	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"sgroup-test-available\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"bbbbbbbb-1111-2222-3333-444444444001\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T20:39:09Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T20:39:06Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T20:39:09Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T20:43:09Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T20:43:09Z"}]	2025-10-15 20:39:06.004555+00	2025-10-15 20:43:09.410241+00	3ccf9ce7-5ad5-4819-afa3-33173fcf12dc	1	2025-10-15 20:43:07.750288+00
112	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"ready-condition-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-2222-3333-4444-555555555999\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T21:57:54Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T21:57:54Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-15T21:57:54Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T21:59:09Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T21:59:09Z"}]	2025-10-15 21:57:54.559985+00	2025-10-15 21:59:09.487514+00	4ae0549e-cbf4-47ba-8a7c-f749d3e15630	1	2025-10-15 21:59:00.034954+00
110	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"sgroup-test-unavailable\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"cccccccc-1111-2222-3333-444444444002\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T20:45:49Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T20:44:32Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T20:45:49Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-15T20:45:49Z"}]	2025-10-15 20:44:32.627056+00	2025-10-15 20:45:49.450609+00	4e57e0b8-cf50-4b9c-b596-c3ef1036ef8a	1	\N
177	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"HostBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"uid-test-binding\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"uid-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"hostRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"uid-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"}}}\\n"}	{}	null	2025-10-16 22:58:23.716048+00	2025-10-16 22:58:23.716048+00	753ee620-1bfe-4498-bb51-27c3fb1dee95	1	\N
159	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T18:14:52Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T18:14:41Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T18:14:52Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T18:14:52Z"}]	2025-10-16 18:14:41.856581+00	2025-10-16 18:14:52.155312+00	9e5f6376-ba19-4a7f-803d-5f845a5a2420	1	\N
115	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-host-baseline\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"33333333-4444-5555-6666-777777777777\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T22:13:02Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T22:13:02Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-15T22:13:02Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T22:14:19Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T22:14:19Z"}]	2025-10-15 22:13:01.997986+00	2025-10-15 22:14:19.496945+00	fb9fd85b-1dd8-424f-897e-2bd0a62ffc04	1	2025-10-15 22:13:58.363916+00
140	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:47:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T09:47:31Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:47:47Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:47:37Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:47:47Z"}]	2025-10-16 09:47:37.633767+00	2025-10-16 09:47:47.619601+00	64881874-7a38-47ea-a30d-3a02afd919d5	1	2025-10-16 09:47:47.616433+00
148	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T11:23:24Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T11:23:11Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T11:23:44Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T11:23:24Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T11:23:44Z"}]	2025-10-16 11:23:11.683675+00	2025-10-16 11:23:44.733777+00	3e00fe01-0f92-4746-a8a7-5b1fcbf970d9	1	2025-10-16 11:23:35.51572+00
156	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"network-condition-fix-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.240.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T18:08:52Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T10:08:47Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T18:08:52Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T18:08:52Z"}]	2025-10-16 18:08:44.234001+00	2025-10-16 18:08:52.179537+00	e42c0c04-cb90-456e-81b6-43775a077c9d	1	\N
164	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-delete-test-s1\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"E2E DELETE test - Scenario 1\\",\\"ingressPorts\\":[{\\"description\\":\\"Test port\\",\\"port\\":\\"9999\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T20:44:42Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T20:44:42Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-16T20:44:34Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T21:19:12Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T21:19:12Z"}]	2025-10-16 20:44:34.629817+00	2025-10-16 21:19:12.260402+00	a11fc95e-cfbb-4a9c-84d8-8d434d315fd5	1	2025-10-16 21:19:05.220016+00
171	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"HostBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"uid-test-binding\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"uid-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"hostRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"uid-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"}}}\\n"}	{}	null	2025-10-16 22:40:59.678461+00	2025-10-16 22:40:59.678461+00	2437a64e-1774-4f3f-9a3b-cd6d7016c740	1	\N
149	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"migration-036-test-service\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test Service for Migration 036 DELETE trigger\\",\\"ingressPorts\\":[{\\"description\\":\\"Test port\\",\\"port\\":\\"8080\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-16T11:25:13Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Service committed to backend successfully", "lastTransitionTime": "2025-10-16T11:25:13Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-16T11:25:13Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T18:17:52Z"}]	2025-10-16 11:25:13.465106+00	2025-10-16 18:17:52.087032+00	b7ab8a52-a471-41f3-b9fa-0c7de31cff70	1	2025-10-16 18:17:44.057792+00
118	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-with-sgroup-down\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-aaaa-aaaa-aaaa-446655440099\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T22:47:32Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T09:42:37Z"}]	2025-10-15 22:47:32.73346+00	2025-10-16 09:42:37.63814+00	f7b3ad9e-cfd2-4e87-8940-9f18a61a0d09	1	\N
143	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-test-network\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.250.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T10:16:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T10:13:47Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T10:16:06Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T10:16:06Z"}]	2025-10-16 10:13:47.310556+00	2025-10-16 10:16:06.535974+00	40caeb43-9f66-4190-8256-a8df3b73dd52	1	\N
123	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"trace-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T23:31:14Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T23:31:14Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:42:37Z"}]	2025-10-15 23:31:14.245032+00	2025-10-16 09:42:37.715305+00	c2d4465c-2b77-4abe-ace7-411d14b32919	1	2025-10-16 09:04:02.316346+00
126	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-fix-test-network\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.200.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-16T09:07:10Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T09:07:10Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-16 09:07:10.463687+00	2025-10-16 09:42:37.721964+00	fa794c3d-eaed-4a8f-a59c-84f8c5d4e27f	1	2025-10-16 09:42:37.726691+00
131	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-fix-test-network\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.200.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T09:07:10Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-16 09:42:37.726691+00	2025-10-16 09:42:37.732651+00	c46a190d-d145-45a7-9452-28554fd4adc5	1	2025-10-16 09:42:37.735917+00
132	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-fix-test-network\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.200.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T09:07:10Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-16 09:42:37.735917+00	2025-10-16 09:42:37.738224+00	15631a7e-20be-46b7-a43b-a53fe4245f1d	1	2025-10-16 09:42:37.741651+00
145	{"updated": "true", "app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"updated\\":\\"true\\"},\\"name\\":\\"e2e-test-service\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"ingressPorts\\":[{\\"description\\":\\"Updated port\\",\\"port\\":\\"9090\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T10:29:16Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-16T10:27:46Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T10:29:16Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T10:29:16Z"}]	2025-10-16 10:13:47.428557+00	2025-10-16 10:29:16.70807+00	2fe9e30a-686a-460c-8926-1485c944b49f	0	2025-10-16 11:38:21.830078+00
133	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-fix-test-network\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.200.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T09:07:10Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-16 09:42:37.741651+00	2025-10-16 09:42:37.743874+00	2a75be83-c3c0-4c4c-9ba2-e4620eb5ce04	1	2025-10-16 09:42:37.746607+00
161	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "AddressGroupPortMapping committed to backend successfully", "lastTransitionTime": "2025-10-17T07:50:53Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "AddressGroupPortMapping passed validation", "lastTransitionTime": "2025-10-17T07:50:53Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "AddressGroupPortMapping is ready, 1 access ports configured", "lastTransitionTime": "2025-10-17T07:50:53Z"}]	2025-10-16 18:18:03.806964+00	2025-10-17 07:50:53.296897+00	e82126d5-ad2d-47f9-96df-b85ff6b68161	1	\N
155	{}	{}	{}	null	2025-10-16 18:08:44.230589+00	2025-10-16 18:08:44.230589+00	35d3dcf5-50d6-4953-950a-e8c87a378b6b	1	\N
134	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-fix-test-network\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.200.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T09:07:10Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:42:47Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T09:42:47Z"}]	2025-10-16 09:42:37.746607+00	2025-10-16 09:42:47.617576+00	ed20064d-db1f-48a1-8d1e-4afe810e1940	1	2025-10-16 09:42:47.613744+00
173	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"uid-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T22:41:15Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T22:40:59Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T22:41:15Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T22:41:15Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-16 22:40:59.685108+00	2025-10-16 22:41:15.262783+00	a135d829-aec8-4543-a9e3-f9aa8f9dc214	1	2025-10-16 22:56:14.696323+00
163	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T18:41:12Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T18:39:31Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T18:41:12Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T18:41:12Z"}]	2025-10-16 18:39:31.755878+00	2025-10-16 18:41:12.102016+00	46b708e8-749c-49fb-9fb2-b9fd426fd4ee	1	\N
129	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-fix-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"650e8400-e29b-41d4-a716-446655440199\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:42:47Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T09:07:32Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T11:41:54Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:42:47Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T11:41:54Z"}]	2025-10-16 09:07:32.381985+00	2025-10-16 11:41:54.769165+00	02a45be5-c5dc-4ad0-945e-4b0fd3cd2e6e	1	2025-10-16 11:41:51.392657+00
141	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"network-condition-fix-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.240.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T10:09:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T10:08:47Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T10:09:06Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T10:09:06Z"}]	2025-10-16 10:08:47.788134+00	2025-10-16 10:09:06.532842+00	7edd6099-2208-4659-9be8-6ac95ab14715	1	\N
170	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"uid-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T22:41:05Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-16T22:40:59Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T22:56:20Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T22:41:05Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T22:56:20Z"}]	2025-10-16 22:40:59.669032+00	2025-10-16 22:56:20.925268+00	0f8b126a-28f8-401c-a7cc-5811adadcb7e	1	2025-10-16 22:56:14.710872+00
128	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-fix-test-service\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"ingressPorts\\":[{\\"description\\":\\"Test port\\",\\"port\\":\\"8080\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-16T09:07:17Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T09:42:37Z"}]	2025-10-16 09:07:17.392936+00	2025-10-16 09:42:37.817507+00	d82cd486-aecd-4d98-8804-5b46714449ed	1	\N
146	{"updated": "true"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"updated\\":\\"true\\"},\\"name\\":\\"e2e-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"e2e00000-0000-0000-0000-000000000001\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T10:29:16Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T10:27:46Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T10:33:46Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T10:29:16Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T10:33:46Z"}]	2025-10-16 10:27:46.682837+00	2025-10-16 10:33:46.522833+00	938de07d-914b-4e4e-87c5-2f67e8bd6123	1	2025-10-16 10:30:36.671939+00
130	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-fix-test-service-v2\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"ingressPorts\\":[{\\"description\\":\\"Test port v2\\",\\"port\\":\\"9090\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T09:42:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-16T09:29:34Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T09:42:37Z"}]	2025-10-16 09:29:34.570479+00	2025-10-16 09:42:37.847407+00	7370d8b5-8d17-4df3-8752-a44a93f2d603	1	\N
147	{"updated": "true"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-test-network\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.250.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T10:29:16Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-16T10:28:04Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T10:33:46Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T10:29:16Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T10:33:46Z"}]	2025-10-16 10:28:04.566376+00	2025-10-16 10:33:46.556003+00	fb191f84-826b-486c-aa64-bccf8bf78849	1	2025-10-16 10:30:36.831848+00
160	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T18:18:52Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-16T18:18:49Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T18:18:52Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T18:18:52Z"}]	2025-10-16 18:18:03.798173+00	2025-10-16 18:18:52.098088+00	a3df3796-7739-4ee1-8a1c-da81187e1952	0	\N
174	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"uid-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-16T22:56:14Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T22:56:14Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T22:56:30Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-16T22:56:14Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T22:56:30Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T22:56:30Z"}]	2025-10-16 22:56:14.719087+00	2025-10-16 22:56:30.908403+00	6dfce98a-a5c3-45eb-851f-6645efb83bf8	1	2025-10-16 22:56:20.92279+00
144	{"updated": "true", "app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"updated\\":\\"true\\"},\\"name\\":\\"e2e-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"DROP\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T10:29:16Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T10:29:16Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-16T10:27:46Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T10:33:46Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T10:33:46Z"}]	2025-10-16 10:13:47.4073+00	2025-10-16 10:33:46.607764+00	4b2e6612-f188-431c-b90f-706f866db9b7	1	2025-10-16 10:30:36.98739+00
165	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-no-sgroup-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"E2E test - no SGROUP\\",\\"ingressPorts\\":[{\\"port\\":\\"8888\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T21:34:32Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T21:34:32Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-16T21:34:28Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T21:56:32Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T21:56:32Z"}]	2025-10-16 21:34:27.99431+00	2025-10-16 21:56:32.094193+00	a2b67745-10cd-497a-a752-5fb9b1cd5bec	1	2025-10-16 21:55:16.828406+00
179	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"uid-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T22:58:40Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T22:58:23Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T22:58:40Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T22:58:40Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-16 22:58:23.720364+00	2025-10-16 22:58:40.928011+00	3bd2dc24-834c-4a1f-b3b4-b4d0b2c2bc25	1	2025-10-16 23:02:30.77006+00
181	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-scenario1-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-2222-3333-4444-555555555501\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T23:30:00Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T23:29:50Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T23:30:00Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T23:30:00Z"}]	2025-10-16 23:29:50.449332+00	2025-10-16 23:30:00.931748+00	c4999943-91e3-4723-b85f-7c6655672c8b	1	\N
197	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug007-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T06:33:31Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T06:32:34Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:34:51Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T06:33:31Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:34:51Z"}]	2025-10-17 06:32:34.62792+00	2025-10-17 06:34:51.992929+00	5f2d824c-2499-4138-876c-eca61ecd7479	1	2025-10-17 06:34:43.651665+00
152	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"delete-fix-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test service for DELETE fix verification\\",\\"ingressPorts\\":[{\\"description\\":\\"HTTP port\\",\\"port\\":\\"80\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T18:02:52Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T18:02:52Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-16T18:02:47Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T18:05:02Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T18:05:02Z"}]	2025-10-16 18:02:47.371913+00	2025-10-16 18:05:02.136179+00	16b42344-2fc1-49ca-9405-0125523eabb8	1	2025-10-16 18:05:01.252857+00
154	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"delete-test-v2\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test DELETE fix v2\\",\\"ingressPorts\\":[{\\"description\\":\\"Test port\\",\\"port\\":\\"8888\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T18:08:42Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T18:08:42Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-16T18:08:38Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T18:09:22Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T18:09:22Z"}]	2025-10-16 18:08:38.985524+00	2025-10-16 18:09:22.15507+00	3f476988-71b6-4966-9e6f-35f10d40f614	1	2025-10-16 18:09:21.434398+00
176	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"uid-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T22:58:30Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-16T22:58:23Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T23:02:30Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T22:58:30Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T23:02:30Z"}]	2025-10-16 22:58:23.706041+00	2025-10-16 23:02:30.909954+00	1b59ae91-a09c-4930-86c9-83a0a230dbd3	1	2025-10-16 23:02:30.790463+00
127	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"p0-fix-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T18:42:12Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T18:42:12Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-16T18:42:07Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T18:42:12Z"}]	2025-10-16 09:07:12.940672+00	2025-10-16 18:42:12.106683+00	1ac2c135-95b2-44e0-99f2-8f183214314a	1	\N
166	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"integration-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-2222-3333-4444-555555555555\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T22:18:32Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T22:18:15Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T22:24:32Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T22:18:32Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T22:24:32Z"}]	2025-10-16 22:18:15.790437+00	2025-10-16 22:24:32.173796+00	a6c8871e-82b4-4c5b-b4c4-56a90fd2f7be	1	2025-10-16 22:24:29.668161+00
117	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"debug-ready-condition-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-ffff-ffff-ffff-446655440099\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T18:09:52Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T22:46:53Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T18:09:52Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T18:20:12Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T18:20:12Z"}]	2025-10-15 22:46:53.265964+00	2025-10-16 18:20:12.083922+00	49c5a67a-8b9f-4537-809c-f307fb97022f	1	2025-10-16 18:20:10.263166+00
162	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T18:42:12Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T18:39:01Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T18:42:12Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T18:42:12Z"}]	2025-10-16 18:39:01.38518+00	2025-10-16 18:42:12.089889+00	3e2a09ca-608a-4330-9175-06c75333194d	1	\N
150	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"m036-delete-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"DELETE trigger test\\",\\"ingressPorts\\":[{\\"port\\":\\"9999\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-16T11:28:28Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Service committed to backend successfully", "lastTransitionTime": "2025-10-16T11:28:28Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-16T11:28:28Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T11:30:34Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T11:30:34Z"}]	2025-10-16 11:28:28.664337+00	2025-10-16 11:30:34.766863+00	0e33b969-9a6d-4b67-92c1-d991984b5777	1	2025-10-16 11:29:32.261673+00
178	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"HostBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"uid-test-binding\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"uid-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"hostRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"uid-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"}}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "HostBinding committed to backend successfully", "lastTransitionTime": "2025-10-16T22:58:23Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "HostBinding passed validation", "lastTransitionTime": "2025-10-16T22:58:23Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "HostBinding is ready for use", "lastTransitionTime": "2025-10-16T22:58:23Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting internal processing before deletion"}]	2025-10-16 22:58:23.719002+00	2025-10-16 22:58:23.719002+00	c0213766-50a5-4383-b020-5d8def3677c5	1	2025-10-16 23:02:30.798734+00
183	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"HostBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-scenario1-binding\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"e2e-scenario1-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"hostRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"e2e-scenario1-host\\",\\"namespace\\":\\"incloud-sgroups\\"}}}\\n"}	{}	null	2025-10-16 23:31:20.636003+00	2025-10-16 23:31:20.636003+00	d88d70cb-3251-4482-9e09-59eddb10909e	1	\N
167	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"integration-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T22:18:42Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-16T22:18:33Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T22:24:32Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T22:18:42Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T22:24:32Z"}]	2025-10-16 22:18:33.327748+00	2025-10-16 22:24:32.204798+00	691524ed-7df2-4239-84e5-633f6e3a1b3a	1	2025-10-16 22:24:29.862909+00
172	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"HostBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"uid-test-binding\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"uid-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"hostRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"uid-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"}}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "HostBinding committed to backend successfully", "lastTransitionTime": "2025-10-16T22:40:59Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "HostBinding passed validation", "lastTransitionTime": "2025-10-16T22:40:59Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "HostBinding is ready for use", "lastTransitionTime": "2025-10-16T22:40:59Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting internal processing before deletion"}]	2025-10-16 22:40:59.681368+00	2025-10-16 22:40:59.681368+00	ab91c3a9-867d-4707-930d-05bf2f33b913	1	2025-10-16 22:56:14.717037+00
202	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test1-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-1111-1111-1111-111111111111\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:42:45Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:42:45Z"}]	2025-10-17 06:42:45.782332+00	2025-10-17 06:42:45.787245+00	1861d69c-331c-4499-9328-1971ba7d8fd4	1	\N
204	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"HostBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test1-binding\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"test1-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"hostRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"test1-host\\",\\"namespace\\":\\"incloud-sgroups\\"}}}\\n"}	{}	null	2025-10-17 06:42:45.83103+00	2025-10-17 06:42:45.83103+00	946e6f50-5673-47f0-9873-1fb539818fa4	1	\N
180	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"uid-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-16T23:02:30Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T23:02:30Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T23:02:40Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-16T23:02:30Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T23:02:40Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T23:02:40Z"}]	2025-10-16 23:02:30.801117+00	2025-10-16 23:02:40.904563+00	1c9980c7-5b0c-48d0-9d4a-9ee5d6669d36	1	2025-10-16 23:02:30.907493+00
205	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"HostBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test1-binding\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"test1-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"hostRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"test1-host\\",\\"namespace\\":\\"incloud-sgroups\\"}}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "HostBinding committed to backend successfully", "lastTransitionTime": "2025-10-17T06:42:45Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "HostBinding passed validation", "lastTransitionTime": "2025-10-17T06:42:45Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "HostBinding is ready for use", "lastTransitionTime": "2025-10-17T06:42:45Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting internal processing before deletion"}]	2025-10-17 06:42:45.834554+00	2025-10-17 06:42:45.834554+00	e45769a2-98a1-4e7f-b827-cceb5baef726	1	2025-10-17 06:43:46.971882+00
224	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T06:54:22Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:54:18Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T06:54:22Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T06:54:22Z"}]	2025-10-17 06:54:18.735038+00	2025-10-17 06:54:22.025341+00	01105392-ecce-44c4-866f-dd90d16e8869	1	\N
186	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-scenario2-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"22222222-3333-4444-5555-666666666602\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T23:35:00Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T23:34:49Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T23:35:00Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T23:35:00Z"}]	2025-10-16 23:34:49.117756+00	2025-10-16 23:35:00.935769+00	eb0598c6-2eeb-4678-925c-eaf68346890f	1	\N
192	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug006-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"hosts\\":[{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"bug006-test-host\\"}],\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:28:40Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T06:28:40Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:30:11Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:30:11Z"}]	2025-10-17 06:28:40.706149+00	2025-10-17 06:30:11.997779+00	59990b67-4b38-4498-8947-3d88bccb1c3a	1	2025-10-17 06:30:05.491927+00
182	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-scenario1-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T23:31:20Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-16T23:29:52Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T23:38:30Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T23:31:20Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T23:38:30Z"}]	2025-10-16 23:29:52.704064+00	2025-10-16 23:38:30.926585+00	c6f14e07-7c6e-436e-81e2-f3012a2a5c8a	1	2025-10-16 23:38:27.302715+00
196	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug007-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"44444444-5555-6666-7777-888888888807\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:32:34Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:32:34Z"}]	2025-10-17 06:32:34.592499+00	2025-10-17 06:32:34.595393+00	028270c6-96b3-4b96-a99f-24c726a77a70	1	\N
198	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"HostBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug007-test-binding\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"bug007-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"hostRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"bug007-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"}}}\\n"}	{}	null	2025-10-17 06:32:34.641826+00	2025-10-17 06:32:34.641826+00	cffc84d2-368f-4d63-baf8-8104755d4a14	1	\N
201	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug007-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"44444444-5555-6666-7777-888888888807\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T06:33:41Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:33:32Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:34:51Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T06:33:41Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:34:51Z"}]	2025-10-17 06:33:32.168125+00	2025-10-17 06:34:51.980367+00	cd2af157-ba0e-4793-8099-db838d4852b1	1	2025-10-17 06:34:43.497991+00
206	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test1-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-1111-1111-1111-111111111111\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T06:43:01Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:42:45Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T06:43:01Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T06:43:01Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-17 06:42:45.83607+00	2025-10-17 06:43:01.988291+00	b142a678-af9e-4712-88d1-b2a86b84bead	1	2025-10-17 06:43:46.652024+00
208	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"eeeeeeee-2222-3333-4444-555555555555\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:44:29Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:44:29Z"}]	2025-10-17 06:44:29.37868+00	2025-10-17 06:44:29.384008+00	b4554b24-6404-4b80-b1da-b5b02822a6f1	1	\N
199	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"HostBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug007-test-binding\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"bug007-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"hostRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"bug007-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"}}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "HostBinding committed to backend successfully", "lastTransitionTime": "2025-10-17T06:32:34Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "HostBinding passed validation", "lastTransitionTime": "2025-10-17T06:32:34Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "HostBinding is ready for use", "lastTransitionTime": "2025-10-17T06:32:34Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting internal processing before deletion"}]	2025-10-17 06:32:34.645519+00	2025-10-17 06:32:34.645519+00	e002469c-0d4f-4daf-8429-346ad7945ea9	1	2025-10-17 06:33:32.163319+00
230	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-ag3\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T07:19:46Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:19:46Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:19:58Z"}]	2025-10-17 07:19:46.52479+00	2025-10-17 07:19:58.405301+00	b74be400-3b61-49c7-aeab-ba610d9feab5	1	2025-10-17 07:19:54.930697+00
233	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"NetworkBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-nb1\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"bug008-ag4\\"},\\"networkRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"name\\":\\"bug008-net1\\"}}}\\n"}	{}	null	2025-10-17 07:20:06.56833+00	2025-10-17 07:20:06.56833+00	f744e893-6f77-41c3-bf15-f3058fdbda53	1	\N
193	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug006-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"33333333-4444-5555-6666-777777777706\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:28:40Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:28:40Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:30:02Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-17 06:28:40.793751+00	2025-10-17 06:30:02.011575+00	11fde84b-e8b3-4962-bd8d-22f5621b7968	1	2025-10-17 06:30:05.320906+00
185	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-scenario1-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-2222-3333-4444-555555555501\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T23:31:20Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T23:31:20Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T23:31:20Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T23:31:20Z"}]	2025-10-16 23:31:20.649364+00	2025-10-16 23:31:20.950247+00	a054d95c-aa93-44ce-9f4c-b8470aa7f9f7	1	\N
187	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-scenario2-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"hosts\\":[{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"e2e-scenario2-host\\"}],\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-16T23:35:20Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-16T23:35:20Z"}]	2025-10-16 23:35:20.546852+00	2025-10-16 23:35:22.552351+00	f074ad93-f850-4456-9bef-ce0e5f18b063	1	\N
184	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"HostBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-scenario1-binding\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"e2e-scenario1-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"hostRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"e2e-scenario1-host\\",\\"namespace\\":\\"incloud-sgroups\\"}}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "HostBinding committed to backend successfully", "lastTransitionTime": "2025-10-16T23:31:20Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "HostBinding passed validation", "lastTransitionTime": "2025-10-16T23:31:20Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "HostBinding is ready for use", "lastTransitionTime": "2025-10-16T23:31:20Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting internal processing before deletion"}]	2025-10-16 23:31:20.646574+00	2025-10-16 23:31:20.646574+00	710040af-9a3c-466d-bf7c-d4ebd7f17c64	1	2025-10-16 23:37:24.44107+00
195	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug006-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"33333333-4444-5555-6666-777777777706\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T06:30:11Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:30:05Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:30:21Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:30:21Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T06:30:11Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:30:21Z"}]	2025-10-17 06:30:05.510452+00	2025-10-17 06:30:21.985392+00	4c0ac143-daa5-42aa-9690-2ea1d5521ec8	1	2025-10-17 06:30:11.99624+00
200	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug007-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"44444444-5555-6666-7777-888888888807\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T06:32:52Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:32:34Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T06:32:52Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T06:32:52Z"}]	2025-10-17 06:32:34.647603+00	2025-10-17 06:32:52.014584+00	1abe8fe6-0857-49db-bbaa-d3dfc984c398	1	\N
194	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug006-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"33333333-4444-5555-6666-777777777706\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:28:40Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:30:05Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:30:02Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion", "lastTransitionTime": null}]	2025-10-17 06:30:05.505517+00	2025-10-17 06:30:05.509518+00	22be0f7b-1f9e-4693-989a-856971bc0141	1	\N
188	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-scenario2-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"22222222-3333-4444-5555-666666666602\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T23:35:20Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T23:34:49Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-16T23:35:20Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T23:35:20Z"}]	2025-10-16 23:35:20.563877+00	2025-10-16 23:35:20.946733+00	90e0d043-c6f8-4ef0-94e5-bcd4f76a2398	1	\N
211	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"eeeeeeee-2222-3333-4444-555555555555\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:44:29Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:44:32Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:44:31Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion", "lastTransitionTime": null}]	2025-10-17 06:44:32.975625+00	2025-10-17 06:44:32.977003+00	6595814e-d190-4c6f-8e8b-05b38955dd74	1	\N
189	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-scenario1-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-2222-3333-4444-555555555501\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-16T23:37:30Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-16T23:37:24Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T23:38:00Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-16T23:37:30Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-16T23:38:00Z"}]	2025-10-16 23:37:24.444799+00	2025-10-16 23:38:00.925031+00	a477ebf2-beba-42a8-be3b-ceafc6abf497	1	2025-10-16 23:37:53.135705+00
231	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-ag4\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T07:19:59Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:19:59Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:20:08Z"}]	2025-10-17 07:19:59.30514+00	2025-10-17 07:20:08.435328+00	6e6d2784-d0ff-46eb-b2e6-ab97d7718641	1	2025-10-17 07:20:06.773175+00
209	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"hosts\\":[{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"e2e-test-host\\"}],\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:44:29Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T06:44:29Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:44:41Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:44:41Z"}]	2025-10-17 06:44:29.411594+00	2025-10-17 06:44:41.988986+00	c5eff244-4798-4daa-a350-1659bc137aeb	1	2025-10-17 06:44:32.969817+00
203	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test1-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T06:42:51Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T06:42:45Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:43:51Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T06:42:51Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:43:51Z"}]	2025-10-17 06:42:45.821313+00	2025-10-17 06:43:51.979841+00	38856938-d601-4ed3-8f4c-a6747543c866	1	2025-10-17 06:43:46.806605+00
207	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test1-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"11111111-1111-1111-1111-111111111111\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-17T06:43:46Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:43:46Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:44:01Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-17T06:43:46Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:44:01Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:44:01Z"}]	2025-10-17 06:43:46.976844+00	2025-10-17 06:44:01.968172+00	b3f4dc4f-b043-4171-a4a1-8419b94e6fe9	1	2025-10-17 06:43:51.977845+00
210	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"eeeeeeee-2222-3333-4444-555555555555\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:44:29Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:44:29Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:44:31Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-17 06:44:29.429892+00	2025-10-17 06:44:31.985918+00	652de460-c132-4e0b-8d48-c4686aff4fde	1	2025-10-17 06:44:32.8233+00
213	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"final-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"ffffffff-ffff-ffff-ffff-ffffffffffff\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:45:35Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:45:36Z"}]	2025-10-17 06:45:35.996785+00	2025-10-17 06:45:36.002343+00	38c07d77-78a4-4bb0-8d9d-e44382999b6c	1	\N
215	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"final-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"ffffffff-ffff-ffff-ffff-ffffffffffff\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:45:35Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:45:36Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-17 06:45:36.045105+00	2025-10-17 06:45:36.045105+00	53a43805-466d-4197-a025-964e65e218c0	1	2025-10-17 06:45:39.599213+00
214	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"final-test-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"hosts\\":[{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"final-test-host\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:45:36Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T06:45:36Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:45:42Z"}]	2025-10-17 06:45:36.022128+00	2025-10-17 06:45:42.078925+00	41b5fb00-5d28-4eaf-b515-300b61bd87d0	1	2025-10-17 06:45:39.742104+00
218	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"final-test-host1\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"f1111111-1111-1111-1111-111111111111\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:45:44Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:45:44Z"}]	2025-10-17 06:45:44.199275+00	2025-10-17 06:45:44.205464+00	4aeee4bf-5571-4b09-889c-64911dd134e3	1	\N
225	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-host1\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"00000008-0001-0001-0001-000000000001\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:19:28Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T07:19:24Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:19:38Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:19:28Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:19:38Z"}]	2025-10-17 07:19:24.548622+00	2025-10-17 07:19:38.398349+00	9512ae55-e9fd-4347-a03e-7111ba696bb1	1	2025-10-17 07:19:29.916225+00
227	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-ag2\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:19:38Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:19:34Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:19:48Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:19:38Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:19:48Z"}]	2025-10-17 07:19:34.521758+00	2025-10-17 07:19:48.409328+00	e21e4a28-cdf3-40b6-b380-591413eac5a9	1	2025-10-17 07:19:42.016264+00
212	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"e2e-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"eeeeeeee-2222-3333-4444-555555555555\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T06:44:41Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:44:32Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:44:51Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:44:51Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T06:44:41Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:44:51Z"}]	2025-10-17 06:44:32.978305+00	2025-10-17 06:44:51.967742+00	5bf492ab-f446-4da6-9189-1b87cb9d5306	1	2025-10-17 06:44:41.988049+00
216	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"final-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"ffffffff-ffff-ffff-ffff-ffffffffffff\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T06:45:35Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:45:39Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion", "lastTransitionTime": null}]	2025-10-17 06:45:39.748302+00	2025-10-17 06:45:39.749456+00	13545e67-a8d8-4be9-a55e-fdaf7ce8dd8d	1	\N
234	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-net1\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.8.1.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "SyncSucceeded", "status": "True", "message": "Successfully synced with external source", "lastTransitionTime": "2025-10-17T07:20:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-17T07:20:04Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-17 07:20:06.571407+00	2025-10-17 07:20:06.571407+00	1bac6ab0-0d34-4e8e-a03e-73ef41fd2c81	1	2025-10-17 07:20:06.622124+00
241	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"NetworkBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-nb2\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"bug008-ag5\\"},\\"networkRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"name\\":\\"bug008-net2\\"}}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "NetworkBinding committed to backend successfully", "lastTransitionTime": "2025-10-17T07:20:18Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "NetworkBinding passed validation", "lastTransitionTime": "2025-10-17T07:20:18Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "NetworkBinding is ready for use", "lastTransitionTime": "2025-10-17T07:20:18Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting internal processing before deletion"}]	2025-10-17 07:20:18.457399+00	2025-10-17 07:20:18.457399+00	8137e1f6-a6d1-45e8-922c-4ac48a018394	1	2025-10-17 07:20:18.680705+00
238	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-ag5\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:20:18Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:20:16Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:20:28Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:20:18Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:20:28Z"}]	2025-10-17 07:20:16.302797+00	2025-10-17 07:20:28.420978+00	64ff2ae4-3e8d-4978-9c90-a656dc0b200e	1	2025-10-17 07:20:18.683694+00
226	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-ag1\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T07:19:27Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:19:27Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:19:38Z"}]	2025-10-17 07:19:27.690222+00	2025-10-17 07:19:38.413038+00	741b3c4d-f024-415e-84cb-254b6f9c54ff	1	2025-10-17 07:19:30.167914+00
217	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"final-test-host\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"ffffffff-ffff-ffff-ffff-ffffffffffff\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T06:45:42Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:45:39Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:45:51Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T06:45:42Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:45:51Z"}]	2025-10-17 06:45:39.750156+00	2025-10-17 06:45:51.966929+00	81d7df2f-975d-4475-bfb5-b95049516054	1	2025-10-17 06:45:42.076746+00
221	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"final-test-host1\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"f1111111-1111-1111-1111-111111111111\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T06:45:52Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:45:44Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:46:01Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T06:45:52Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:46:01Z"}]	2025-10-17 06:45:44.234295+00	2025-10-17 06:46:01.987325+00	2a559436-405c-48e6-b812-68711813b274	1	2025-10-17 06:45:53.994372+00
219	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"final-test-host2\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"f2222222-2222-2222-2222-222222222222\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T06:45:52Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T06:45:44Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:46:02Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T06:45:52Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:46:02Z"}]	2025-10-17 06:45:44.214611+00	2025-10-17 06:46:02.005277+00	3107a227-e88c-4734-acca-4c6f9e1d6edb	1	2025-10-17 06:45:54.142482+00
229	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-host3\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"00000008-0003-0003-0003-000000000003\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:19:48Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T07:19:46Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:19:58Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:19:48Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:19:58Z"}]	2025-10-17 07:19:46.443947+00	2025-10-17 07:19:58.394996+00	bf8c7ebd-b804-4b70-83a0-45f62c0f82c3	1	2025-10-17 07:19:54.777342+00
236	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-net1\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.8.1.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:20:08Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-17T07:20:04Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:20:18Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:20:08Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:20:18Z"}]	2025-10-17 07:20:06.771841+00	2025-10-17 07:20:18.387177+00	fa091d2c-dc51-428e-80de-0b4124bb1e55	1	2025-10-17 07:20:08.432981+00
239	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"NetworkBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-nb2\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"bug008-ag5\\"},\\"networkRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"name\\":\\"bug008-net2\\"}}}\\n"}	{}	null	2025-10-17 07:20:18.454073+00	2025-10-17 07:20:18.454073+00	7422a69e-1d9a-43c0-be9b-620b69b68afb	1	\N
245	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"NetworkBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-nb3\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"bug008-ag6\\"},\\"networkRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"name\\":\\"bug008-net3\\"}}}\\n"}	{}	null	2025-10-17 07:20:31.335376+00	2025-10-17 07:20:31.335376+00	5b84fad7-87fc-419a-b09c-555455d86213	1	\N
228	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-host2\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"00000008-0002-0002-0002-000000000002\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T07:19:39Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T07:19:39Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:19:48Z"}]	2025-10-17 07:19:39.648967+00	2025-10-17 07:19:48.398169+00	b51c1419-3d28-4a66-91cf-42b26a857f9a	1	2025-10-17 07:19:41.866154+00
232	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-net1\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.8.1.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T07:20:04Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-17T07:20:04Z"}]	2025-10-17 07:20:04.427313+00	2025-10-17 07:20:04.430837+00	0ca7368e-29ee-49b0-8aaf-4816f8b7d5aa	1	\N
235	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"NetworkBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-nb1\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"bug008-ag4\\"},\\"networkRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"name\\":\\"bug008-net1\\"}}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "NetworkBinding committed to backend successfully", "lastTransitionTime": "2025-10-17T07:20:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "NetworkBinding passed validation", "lastTransitionTime": "2025-10-17T07:20:06Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "NetworkBinding is ready for use", "lastTransitionTime": "2025-10-17T07:20:06Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting internal processing before deletion"}]	2025-10-17 07:20:06.574154+00	2025-10-17 07:20:06.574154+00	811a93e2-de22-479b-82f3-b9cac83bb3c4	1	2025-10-17 07:20:06.770641+00
220	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"final-test-ag2\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"hosts\\":[]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Address group is ready and operational", "lastTransitionTime": "2025-10-17T06:45:50Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Address group successfully synced to backend and SGROUP", "lastTransitionTime": "2025-10-17T06:45:50Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T06:45:50Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T06:46:02Z"}]	2025-10-17 06:45:44.226966+00	2025-10-17 06:46:02.011685+00	cbafef9f-30f3-4170-b0f7-aaccd8081616	1	2025-10-17 06:45:54.292247+00
237	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-net2\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.8.2.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:20:18Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-17T07:20:11Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T07:20:18Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:20:18Z"}]	2025-10-17 07:20:11.164588+00	2025-10-17 07:20:18.407564+00	d5186ab4-f61c-4198-ae39-43deb7d20612	1	\N
240	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-net2\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.8.2.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "SyncSucceeded", "status": "True", "message": "Successfully synced with external source", "lastTransitionTime": "2025-10-17T07:20:18Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-17T07:20:11Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T07:20:18Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:20:18Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-17 07:20:18.455725+00	2025-10-17 07:20:18.455725+00	98da2426-7388-4720-bc2b-92e8b3c6cdff	1	2025-10-17 07:20:18.525113+00
243	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-net3\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.8.3.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:20:28Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-17T07:20:23Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T07:20:28Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:20:28Z"}]	2025-10-17 07:20:23.017728+00	2025-10-17 07:20:28.464537+00	3aa45990-a25c-4701-bfcd-5b03c99b104b	1	\N
242	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-net2\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.8.2.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:20:28Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-17T07:20:11Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:20:38Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:20:28Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:20:38Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:20:38Z"}]	2025-10-17 07:20:18.681982+00	2025-10-17 07:20:38.401044+00	b3a7fa11-7482-4406-b9a3-adfe0a049f39	1	2025-10-17 07:20:28.418394+00
284	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug009-final-ag\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:15:29Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:15:21Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:15:29Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:15:29Z"}]	2025-10-17 08:15:21.651276+00	2025-10-17 08:15:29.924968+00	93c18d3c-cc5b-49e8-a8e5-a25f67bf0b34	1	\N
295	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-105\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T08:24:22Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:24:22Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:24:29Z"}]	2025-10-17 08:24:22.431449+00	2025-10-17 08:24:29.874798+00	3baf9ed7-d17f-4762-9802-8bac84e2a80a	1	2025-10-17 08:24:23.177817+00
250	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-net-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.8.99.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:28:07Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-17T07:28:05Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:28:17Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:28:07Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:28:17Z"}]	2025-10-17 07:28:05.934212+00	2025-10-17 07:28:17.556772+00	889e21dc-5e90-4dba-9570-c4dd90c791c1	1	2025-10-17 07:28:08.057057+00
249	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-ag-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:28:07Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:28:00Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:28:17Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:28:07Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:28:17Z"}]	2025-10-17 07:28:00.755298+00	2025-10-17 07:28:17.563337+00	bcb009b4-f897-461b-9c71-69c7fb65611f	1	2025-10-17 07:28:10.237177+00
251	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-ag-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T07:28:17Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:28:17Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:28:27Z"}]	2025-10-17 07:28:17.67101+00	2025-10-17 07:28:27.574783+00	de5bba12-4018-40c2-ac2d-fa1e43b59675	1	2025-10-17 07:28:19.969044+00
244	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-ag6\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:20:28Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:20:23Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:20:38Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:20:28Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:20:38Z"}]	2025-10-17 07:20:23.102928+00	2025-10-17 07:20:38.465859+00	d4f484de-2b5a-48e9-abc8-9f3ddeaf655c	1	2025-10-17 07:20:33.788821+00
303	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-sgroup-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:55:39Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:55:29Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:55:39Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:55:39Z"}]	2025-10-17 08:55:29.952937+00	2025-10-17 08:55:39.913445+00	46da6df3-1316-4796-b43f-c2f920fba79e	1	\N
267	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-svc-006\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test Service 006\\",\\"ingressPorts\\":[{\\"port\\":\\"9090\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:57:49Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:57:49Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T07:57:46Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T07:57:49Z"}]	2025-10-17 07:57:46.126275+00	2025-10-17 07:57:49.871918+00	644a893b-3835-42b6-b4e8-d6372a0476c6	1	\N
255	{}	{}	{}	null	2025-10-17 07:44:23.239576+00	2025-10-17 07:44:31.713803+00	46e92248-221b-4e7b-9a0d-4d665402c98a	1	\N
256	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug009-ag-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T07:44:42Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:44:42Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:44:47Z"}]	2025-10-17 07:44:42.290276+00	2025-10-17 07:44:47.579418+00	f7ac43aa-1791-45f3-a93e-630f94fded2e	1	2025-10-17 07:44:44.563836+00
269	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-svc-007\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test Service 007\\",\\"ingressPorts\\":[{\\"port\\":\\"9090\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:57:59Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:57:59Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T07:57:53Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T07:57:59Z"}]	2025-10-17 07:57:53.397527+00	2025-10-17 07:57:59.795777+00	3b4f0441-3dc9-40f1-9fb0-ccff1948c3fa	1	\N
262	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-002\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:57:39Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:57:32Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:58:09Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:57:39Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:58:09Z"}]	2025-10-17 07:57:32.512063+00	2025-10-17 07:58:09.804858+00	2a75e030-f058-4714-81dc-286f0c7a90fe	1	2025-10-17 07:58:00.920267+00
274	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "AddressGroupPortMapping committed to backend successfully", "lastTransitionTime": "2025-10-17T08:12:10Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "AddressGroupPortMapping passed validation", "lastTransitionTime": "2025-10-17T08:12:10Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "AddressGroupPortMapping is ready, 1 access ports configured", "lastTransitionTime": "2025-10-17T08:12:10Z"}]	2025-10-17 08:12:10.13632+00	2025-10-17 08:12:10.137557+00	a14d1777-a329-4620-be31-4d05a1b32f2c	1	\N
246	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-net3\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.8.3.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "SyncSucceeded", "status": "True", "message": "Successfully synced with external source", "lastTransitionTime": "2025-10-17T07:20:31Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-17T07:20:23Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T07:20:28Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:20:28Z"}]	2025-10-17 07:20:31.337371+00	2025-10-17 07:20:31.337371+00	1a1dea66-bf99-41c8-943a-bec6b3d922f0	1	\N
247	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"NetworkBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-nb3\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"bug008-ag6\\"},\\"networkRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"name\\":\\"bug008-net3\\"}}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "NetworkBinding committed to backend successfully", "lastTransitionTime": "2025-10-17T07:20:31Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "NetworkBinding passed validation", "lastTransitionTime": "2025-10-17T07:20:31Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "NetworkBinding is ready for use", "lastTransitionTime": "2025-10-17T07:20:31Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting internal processing before deletion"}]	2025-10-17 07:20:31.339127+00	2025-10-17 07:20:31.339127+00	4f69701e-e8d8-4764-afb7-1174c402e7c5	1	2025-10-17 07:20:33.452844+00
253	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug009-ag-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:44:27Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:44:21Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:44:37Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:44:27Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:44:37Z"}]	2025-10-17 07:44:21.05792+00	2025-10-17 07:44:37.630233+00	b158b02f-5975-4c21-93d8-50136abce776	1	2025-10-17 07:44:33.965095+00
265	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-svc-005\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test Service 005\\",\\"ingressPorts\\":[{\\"port\\":\\"9090\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:57:39Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:57:39Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T07:57:33Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T07:57:39Z"}]	2025-10-17 07:57:33.781015+00	2025-10-17 07:57:39.808964+00	69a23d5a-805f-40dd-975f-6fdc576273a5	1	\N
260	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:57:29Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:57:26Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:58:09Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:57:29Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:58:09Z"}]	2025-10-17 07:57:26.253139+00	2025-10-17 07:58:09.793982+00	7757d8b1-959b-48aa-94df-f0d6aa65786b	1	2025-10-17 07:58:00.915746+00
257	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug009-ag-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T07:46:29Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:46:29Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:46:39Z"}]	2025-10-17 07:46:29.261547+00	2025-10-17 07:46:39.811718+00	98a2b86d-fda6-4488-8771-8ea5f08b2081	1	2025-10-17 07:46:31.504583+00
305	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "AddressGroupPortMapping committed to backend successfully", "lastTransitionTime": "2025-10-17T08:56:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "AddressGroupPortMapping passed validation", "lastTransitionTime": "2025-10-17T08:56:06Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "AddressGroupPortMapping is ready, 1 access ports configured", "lastTransitionTime": "2025-10-17T08:56:06Z"}]	2025-10-17 08:56:06.739161+00	2025-10-17 08:56:06.740335+00	1e4c565f-5baa-4287-a616-edb2da1711ef	1	\N
268	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-006\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:57:49Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:57:47Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:58:09Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:57:49Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:58:09Z"}]	2025-10-17 07:57:47.202163+00	2025-10-17 07:58:09.854775+00	7b3af3ed-7a7f-4138-8be0-716d0d606751	1	2025-10-17 07:58:00.930951+00
248	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug008-net3\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.8.3.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "SyncSucceeded", "status": "True", "message": "Successfully synced with external source", "lastTransitionTime": "2025-10-17T07:20:33Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-17T07:20:23Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:20:38Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:20:28Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:20:38Z"}]	2025-10-17 07:20:33.454607+00	2025-10-17 07:20:38.458864+00	6b741243-19c9-47ef-841d-e3087c428dfc	1	2025-10-17 07:20:33.617869+00
254	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug009-svc-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test Service for BUG-009\\",\\"ingressPorts\\":[{\\"port\\":\\"8080\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:44:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T07:44:31Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:44:37Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T07:44:37Z"}]	2025-10-17 07:44:23.229689+00	2025-10-17 07:44:37.629773+00	755d449a-d2c6-440a-ad04-df870eab192a	0	\N
285	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug009-final-svc\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Final test Service\\",\\"ingressPorts\\":[{\\"port\\":\\"8080\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:15:29Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:15:29Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T08:15:26Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:15:29Z"}]	2025-10-17 08:15:26.832828+00	2025-10-17 08:15:29.989957+00	7bfadd18-1e34-47b5-b864-86688174f4b6	1	\N
258	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"bug009-ag-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T07:46:50Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:46:50Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:46:59Z"}]	2025-10-17 07:46:50.425991+00	2025-10-17 07:46:59.777896+00	70a8b0e8-090a-4a09-b0b2-a30dcf0e00d3	1	2025-10-17 07:46:52.72867+00
151	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"service-ready-fix-test\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"ingressPorts\\":[{\\"description\\":\\"E2E test for Service Ready fix\\",\\"port\\":\\"9999\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:50:59Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T07:50:53Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:50:59Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T07:50:59Z"}]	2025-10-16 12:09:07.634285+00	2025-10-17 07:50:59.92976+00	194fc396-532d-4cc8-b06c-355b19ca2e34	0	\N
271	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-svc-011\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test Service 011\\",\\"ingressPorts\\":[{\\"port\\":\\"8080\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:58:09Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:58:09Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T07:58:00Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T07:58:09Z"}]	2025-10-17 07:58:00.734891+00	2025-10-17 07:58:09.793678+00	c46592b2-1c53-4239-a5df-67e8ca4fd7d1	1	\N
266	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-005\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T07:57:49Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:57:39Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:58:09Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T07:57:49Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:58:09Z"}]	2025-10-17 07:57:39.86608+00	2025-10-17 07:58:09.809694+00	9015c4d3-4176-4d9b-8e8f-ea5ac92945da	1	2025-10-17 07:58:00.927726+00
270	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-007\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T07:57:59Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T07:57:59Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T07:58:09Z"}]	2025-10-17 07:57:59.524717+00	2025-10-17 07:58:09.860448+00	eb70d741-ae86-46fb-a37e-b803df1795ae	1	2025-10-17 07:58:00.933848+00
286	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-101\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T08:24:10Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:24:10Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:24:19Z"}]	2025-10-17 08:24:10.876346+00	2025-10-17 08:24:19.88347+00	ca908f6c-ced0-47d2-8b4a-f2466276b439	1	2025-10-17 08:24:11.316044+00
288	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-102\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T08:24:13Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:24:13Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:24:19Z"}]	2025-10-17 08:24:13.527616+00	2025-10-17 08:24:19.933207+00	36da483d-f4d7-48f9-b794-ee27d1ea6bcc	1	2025-10-17 08:24:14.219715+00
297	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-201\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T08:26:55Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:26:55Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:26:59Z"}]	2025-10-17 08:26:55.229988+00	2025-10-17 08:26:59.835516+00	4adf0a97-0a28-4af2-9f63-e637fee80fb5	1	2025-10-17 08:26:55.385272+00
302	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-203\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T08:27:10Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:27:10Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:27:19Z"}]	2025-10-17 08:27:10.313873+00	2025-10-17 08:27:19.834508+00	87dc465f-5c8f-44c6-b0f3-1c5151e916a1	1	2025-10-17 08:27:10.491893+00
272	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-namespace-fix\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:12:09Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:12:04Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:12:09Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:12:09Z"}]	2025-10-17 08:12:04.93711+00	2025-10-17 08:12:09.846418+00	93cfd2ae-d197-4a17-b7bf-85ce2d230bd2	1	\N
304	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-svc-sgroup-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroups\\":[{\\"name\\":\\"test-ag-sgroup-001\\"}],\\"description\\":\\"Test Service for SGROUP E2E verification\\",\\"ingressPorts\\":[{\\"port\\":\\"8080\\",\\"protocol\\":\\"TCP\\"},{\\"port\\":\\"9090\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:56:09Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:56:09Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T08:56:06Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:56:09Z"}]	2025-10-17 08:56:06.725739+00	2025-10-17 08:56:09.887611+00	dce333fc-d122-4a0a-8629-b29e33ef61ee	1	\N
291	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-103\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T08:24:16Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:24:16Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:24:20Z"}]	2025-10-17 08:24:16.517222+00	2025-10-17 08:24:20.01397+00	08d1cf03-eaf4-4f3f-bb14-fbf1e0a4e6e4	1	2025-10-17 08:24:16.835311+00
293	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-svc-104\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test Service 104\\",\\"ingressPorts\\":[{\\"port\\":\\"9090\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T08:24:19Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Service committed to backend successfully", "lastTransitionTime": "2025-10-17T08:24:19Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T08:24:19Z"}]	2025-10-17 08:24:19.260007+00	2025-10-17 08:24:21.044689+00	1306c5d3-bc65-413b-ad50-c4a0e7fd310f	1	\N
300	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-svc-202\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test Service 202\\",\\"ingressPorts\\":[{\\"port\\":\\"9090\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:27:09Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:27:09Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T08:27:02Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:27:09Z"}]	2025-10-17 08:27:02.766415+00	2025-10-17 08:27:09.855557+00	43b3cf80-285f-4cb8-8c26-e31b4ceeeb38	1	\N
301	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-svc-203\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test Service 203\\",\\"ingressPorts\\":[{\\"port\\":\\"9090\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:27:09Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:27:09Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T08:27:05Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:27:09Z"}]	2025-10-17 08:27:05.183739+00	2025-10-17 08:27:09.874753+00	976c2c2b-192d-4db3-9db6-8f0a639acaff	1	\N
306	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-jd9qk\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"f2aca337-fb77-4fa1-80cf-b66e2216002f\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:56:19Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T08:56:19Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:56:19Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:56:19Z"}]	2025-10-17 08:56:19.011551+00	2025-10-17 08:56:19.872063+00	b558b006-0526-4820-95ea-a2c455896915	1	\N
273	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-svc-namespace-fix\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroups\\":[{\\"name\\":\\"test-ag-namespace-fix\\"}],\\"description\\":\\"Test Service for namespace normalization\\",\\"ingressPorts\\":[{\\"port\\":\\"8080\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:12:19Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:12:19Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T08:12:10Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:12:19Z"}]	2025-10-17 08:12:10.1312+00	2025-10-17 08:12:19.805604+00	97e83bba-9cf9-4c80-8c48-24dd8f64b707	1	\N
290	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-svc-103\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test Service 103\\",\\"ingressPorts\\":[{\\"port\\":\\"9090\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:24:20Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:24:20Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T08:24:16Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:24:20Z"}]	2025-10-17 08:24:16.41942+00	2025-10-17 08:24:20.0123+00	99e23711-6bb3-45a4-af6d-b3395793bb36	1	\N
292	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-104\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T08:24:19Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:24:19Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:24:29Z"}]	2025-10-17 08:24:19.041287+00	2025-10-17 08:24:29.837988+00	29d1eca7-d8eb-40f6-9e2f-92af173e53de	1	2025-10-17 08:24:20.084172+00
299	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-202\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:26:59Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:26:57Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:27:09Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:26:59Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:27:09Z"}]	2025-10-17 08:26:57.618087+00	2025-10-17 08:27:09.855839+00	3c57ab0e-9093-4026-9805-58fbae61bb86	1	2025-10-17 08:27:02.977664+00
281	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-005\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:13:09Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:13:01Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:13:29Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:13:09Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:13:29Z"}]	2025-10-17 08:13:01.227357+00	2025-10-17 08:13:29.842679+00	5597d4e9-3a8e-4b48-8229-8bac1c8f62b8	1	2025-10-17 08:13:22.369784+00
283	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-007\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T08:13:21Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:13:21Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:13:29Z"}]	2025-10-17 08:13:21.007609+00	2025-10-17 08:13:29.854436+00	8472dac9-0c41-41af-a2a8-c441b6bdde4c	1	2025-10-17 08:13:22.378721+00
275	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:12:49Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:12:47Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:12:49Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:12:49Z"}]	2025-10-17 08:12:47.321615+00	2025-10-17 08:12:49.849777+00	333ca751-ba6e-40c7-95c9-bf4361a35a40	1	\N
277	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "AddressGroupPortMapping committed to backend successfully", "lastTransitionTime": "2025-10-17T08:12:53Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "AddressGroupPortMapping passed validation", "lastTransitionTime": "2025-10-17T08:12:53Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "AddressGroupPortMapping is ready, 1 access ports configured", "lastTransitionTime": "2025-10-17T08:12:53Z"}]	2025-10-17 08:12:53.49719+00	2025-10-17 08:12:53.498579+00	36b3fe1f-f954-4548-9d5c-67694be586d7	1	\N
294	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-svc-105\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"description\\":\\"Test Service 105\\",\\"ingressPorts\\":[{\\"port\\":\\"9090\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:24:29Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:24:29Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T08:24:22Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:24:29Z"}]	2025-10-17 08:24:22.314839+00	2025-10-17 08:24:29.870926+00	1d6404dc-f95e-4847-937d-ff06b548b539	1	\N
276	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-svc-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroups\\":[{\\"name\\":\\"test-ag-001\\"}],\\"description\\":\\"Test Service 001\\",\\"ingressPorts\\":[{\\"port\\":\\"8080\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:13:01Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:13:01Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-17T08:12:53Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:13:01Z"}]	2025-10-17 08:12:53.489654+00	2025-10-17 08:13:01.410311+00	f643bcd4-0296-4b5d-9c6c-3b6505362738	1	\N
278	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-002\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:13:01Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:12:53Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:13:29Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:13:01Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:13:29Z"}]	2025-10-17 08:12:53.561794+00	2025-10-17 08:13:29.824179+00	ca9ecdc5-324b-46a3-871e-a008f916f1f7	1	2025-10-17 08:13:22.357669+00
282	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-006\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-17T08:13:08Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-17T08:13:08Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-17T08:13:29Z"}]	2025-10-17 08:13:08.667151+00	2025-10-17 08:13:29.848463+00	dc88e0f0-051b-43c0-8cb1-3515c09ad1a5	1	2025-10-17 08:13:22.374161+00
307	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-jd9qk\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"f2aca337-fb77-4fa1-80cf-b66e2216002f\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-17T08:56:29Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-17T08:56:19Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-17T08:56:29Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-17T08:56:29Z"}]	2025-10-17 08:56:21.840892+00	2025-10-17 08:56:29.854278+00	da7ab69b-f740-495d-a74b-292f970f8ea6	1	\N
308	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-18T12:06:53Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-18T12:06:53Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-18T12:06:57Z"}]	2025-10-18 12:06:53.972121+00	2025-10-18 12:06:57.462586+00	7855648f-aacd-4285-b050-9f7be9c61863	1	2025-10-18 12:06:56.060707+00
309	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-18T12:08:18Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-18T12:08:18Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-18T12:08:20Z"}]	2025-10-18 12:08:18.078822+00	2025-10-18 12:08:20.137166+00	77a45bfb-832e-4b3c-a56d-370a572c8176	1	2025-10-18 12:08:19.155223+00
\.


--
-- Data for Name: netguard_db_ver; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.netguard_db_ver (id, version_id, is_applied, tstamp) FROM stdin;
1	0	t	2025-10-10 13:18:54.944535
2	1	t	2025-10-10 13:18:54.991713
3	2	t	2025-10-10 13:18:55.370116
4	3	t	2025-10-10 13:18:55.382402
5	4	t	2025-10-10 13:18:55.429825
6	5	t	2025-10-10 13:18:55.464172
7	6	t	2025-10-10 13:18:55.482668
8	7	t	2025-10-10 13:18:55.641595
9	8	t	2025-10-10 13:18:55.652868
10	9	t	2025-10-10 13:18:55.656983
11	10	t	2025-10-10 13:18:55.660344
12	11	t	2025-10-10 13:18:55.671176
13	12	t	2025-10-10 13:18:55.675813
14	13	t	2025-10-10 13:18:55.694815
15	14	t	2025-10-10 13:18:55.751301
16	15	t	2025-10-10 13:18:55.754797
17	16	t	2025-10-10 13:18:55.75596
18	17	t	2025-10-10 13:18:55.757164
19	18	t	2025-10-10 13:18:55.758388
20	19	t	2025-10-10 13:18:55.760785
21	20	t	2025-10-10 13:18:55.762185
22	21	t	2025-10-10 13:18:55.764123
23	22	t	2025-10-10 13:18:55.764783
24	23	t	2025-10-10 13:18:55.765453
25	24	t	2025-10-10 13:18:55.76791
26	25	t	2025-10-14 08:56:29.855726
27	25	t	2025-10-14 08:58:57.971634
28	26	t	2025-10-14 08:58:57.971634
29	27	t	2025-10-14 08:58:57.971634
30	28	t	2025-10-14 08:58:57.971634
31	29	t	2025-10-14 08:58:57.971634
32	30	t	2025-10-14 14:13:04.591765
33	31	t	2025-10-14 15:09:03.237728
34	32	t	2025-10-15 07:34:33.47903
35	33	t	2025-10-15 07:34:33.486253
36	34	t	2025-10-15 13:30:38.999564
37	35	t	2025-10-15 13:58:51.763184
38	36	t	2025-10-16 11:21:13.62508
39	37	t	2025-10-16 22:55:30.714415
\.


--
-- Data for Name: network_bindings; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.network_bindings (namespace, name, network_namespace, network_name, address_group_namespace, address_group_name, resource_version) FROM stdin;
incloud-sgroups	dynamic-199ee35c0c4cb5f3	incloud-sgroups	network-condition-fix-test	incloud-sgroups	p0-fix-test-ag	157
\.


--
-- Data for Name: networks; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.networks (namespace, name, network_items, is_bound, binding_ref_namespace, binding_ref_name, address_group_ref_namespace, address_group_ref_name, resource_version, cidr, ready) FROM stdin;
incloud-sgroups	network-condition-fix-test	[{"cidr": "10.240.0.0/16", "name": ""}]	t	incloud-sgroups	dynamic-199ee35c0c4cb5f3	incloud-sgroups	p0-fix-test-ag	156	10.240.0.0/16	t
incloud-sgroups	dynamic-199ee51f1aacdcfe	[{"cidr": "1.1.1.1/32", "name": ""}]	f	\N	\N	\N	\N	163	1.1.1.1/32	t
\.


--
-- Data for Name: rule_s2s; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.rule_s2s (namespace, name, traffic, resource_version, service_local_ref, service_ref, ieagag_rule_refs, trace) FROM stdin;
\.


--
-- Data for Name: service_aliases; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.service_aliases (namespace, name, service_namespace, service_name, resource_version) FROM stdin;
\.


--
-- Data for Name: services; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.services (namespace, name, description, ingress_ports, resource_version, address_groups, aggregated_address_groups) FROM stdin;
incloud-sgroups	p0-fix-test-service	Service p0-fix-test-service managed by netguard-apiserver	[{"port": "8080", "protocol": "TCP", "description": "Test port"}]	128	[]	[]
incloud-sgroups	p0-fix-test-service-v2	Service p0-fix-test-service-v2 managed by netguard-apiserver	[{"port": "9090", "protocol": "TCP", "description": "Test port v2"}]	130	[]	[]
incloud-sgroups	service-ready-fix-test	Service service-ready-fix-test managed by netguard-apiserver	[{"port": "9999", "protocol": "TCP", "description": "E2E test for Service Ready fix"}]	151	[{"kind": "AddressGroup", "name": "p0-fix-test-ag", "namespace": "incloud-sgroups", "apiVersion": "netguard.sgroups.io/v1beta1"}]	[{"ref": {"kind": "AddressGroup", "name": "p0-fix-test-ag", "namespace": "incloud-sgroups", "apiVersion": "netguard.sgroups.io/v1beta1"}, "source": "spec"}]
incloud-sgroups	dynamic-199ee3e4a95c034b	опишите сервис	[{"port": "80", "protocol": "TCP", "description": ""}]	160	[]	[]
incloud-sgroups	bug009-svc-test	Test Service for BUG-009	[{"port": "8080", "protocol": "TCP", "description": ""}]	254	[]	[]
incloud-sgroups	test-svc-005	Test Service 005	[{"port": "9090", "protocol": "TCP", "description": ""}]	265	[]	[]
incloud-sgroups	test-svc-006	Test Service 006	[{"port": "9090", "protocol": "TCP", "description": ""}]	267	[]	[]
incloud-sgroups	test-svc-007	Test Service 007	[{"port": "9090", "protocol": "TCP", "description": ""}]	269	[]	[]
incloud-sgroups	test-svc-011	Test Service 011	[{"port": "8080", "protocol": "TCP", "description": ""}]	271	[]	[]
incloud-sgroups	test-svc-namespace-fix	Test Service for namespace normalization	[{"port": "8080", "protocol": "TCP", "description": ""}]	273	[{"kind": "AddressGroup", "name": "test-ag-namespace-fix", "namespace": "incloud-sgroups", "apiVersion": "netguard.sgroups.io/v1beta1"}]	[{"ref": {"kind": "AddressGroup", "name": "test-ag-namespace-fix", "namespace": "incloud-sgroups", "apiVersion": "netguard.sgroups.io/v1beta1"}, "source": "spec"}]
incloud-sgroups	test-svc-001	Test Service 001	[{"port": "8080", "protocol": "TCP", "description": ""}]	276	[{"kind": "AddressGroup", "name": "test-ag-001", "namespace": "incloud-sgroups", "apiVersion": "netguard.sgroups.io/v1beta1"}]	[{"ref": {"kind": "AddressGroup", "name": "test-ag-001", "namespace": "incloud-sgroups", "apiVersion": "netguard.sgroups.io/v1beta1"}, "source": "spec"}]
incloud-sgroups	bug009-final-svc	Final test Service	[{"port": "8080", "protocol": "TCP", "description": ""}]	285	[]	[]
incloud-sgroups	test-svc-103	Test Service 103	[{"port": "9090", "protocol": "TCP", "description": ""}]	290	[]	[]
incloud-sgroups	test-svc-104	Test Service 104	[{"port": "9090", "protocol": "TCP", "description": ""}]	293	[]	[]
incloud-sgroups	test-svc-105	Test Service 105	[{"port": "9090", "protocol": "TCP", "description": ""}]	294	[]	[]
incloud-sgroups	test-svc-202	Test Service 202	[{"port": "9090", "protocol": "TCP", "description": ""}]	300	[]	[]
incloud-sgroups	test-svc-203	Test Service 203	[{"port": "9090", "protocol": "TCP", "description": ""}]	301	[]	[]
incloud-sgroups	test-svc-sgroup-001	Test Service for SGROUP E2E verification	[{"port": "8080", "protocol": "TCP", "description": ""}, {"port": "9090", "protocol": "TCP", "description": ""}]	304	[{"kind": "AddressGroup", "name": "test-ag-sgroup-001", "namespace": "incloud-sgroups", "apiVersion": "netguard.sgroups.io/v1beta1"}]	[{"ref": {"kind": "AddressGroup", "name": "test-ag-sgroup-001", "namespace": "incloud-sgroups", "apiVersion": "netguard.sgroups.io/v1beta1"}, "source": "spec"}]
\.


--
-- Data for Name: sync_outbox; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.sync_outbox (id, resource_type, resource_id, operation, target_system, payload, delta, affects_resources, status, attempts, max_retries, next_retry_at, last_error, error_category, created_at, updated_at, processed_at, resource_namespace, resource_name) FROM stdin;
037065ce-ad90-48d7-a313-92d8130bd8aa	Host	550e8400-cccc-cccc-cccc-446655440099	CREATE	SGROUP	{"name": "debug-test-logging-trace", "uuid": "550e8400-cccc-cccc-cccc-446655440099", "ip_list": null, "hostname": "", "is_bound": false, "namespace": "incloud-sgroups", "address_group_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-15 22:54:43.667605+00	2025-10-15 22:54:43.67008+00	\N	incloud-sgroups	debug-test-logging-trace
6a0a8c54-5192-487d-8505-717b7ff8fe1b	AddressGroup	f3420bc8-27f6-5380-aee9-f5b5afcdb6a5	CREATE	SGROUP	{"name": "bug006-test-ag", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 06:28:40.706149+00	\N	\N	2025-10-17 06:28:40.706149+00	2025-10-17 06:28:40.706149+00	\N	incloud-sgroups	bug006-test-ag
a7e956c4-5128-4778-9e88-f0081ad048a3	AddressGroup	231476e0-9c4d-41c6-97d7-8dc36fb2d99b	CREATE	SGROUP	{"logs": false, "name": "bug006-test-ag", "hosts": [{"kind": "Host", "name": "bug006-test-host", "apiVersion": "netguard.sgroups.io/v1beta1"}], "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 06:28:40.771454+00	2025-10-17 06:28:40.771454+00	\N	incloud-sgroups	bug006-test-ag
332173f7-d6aa-4dba-ae49-2435a47b9cae	Host	550e8400-e29b-41d4-a716-446655440099	UPDATE	SGROUP	{"name": "test-ready-bug-fix-v3", "uuid": "550e8400-e29b-41d4-a716-446655440099", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "UPDATE"}	\N	\N	PENDING	0	5	2025-10-15 22:38:16.226255+00	\N	\N	2025-10-15 22:38:16.226255+00	2025-10-15 22:38:16.226255+00	\N	incloud-sgroups	test-ready-bug-fix-v3
7c427fc9-57fa-419f-a6ba-dcd73829ddf6	HostBinding	0b969f26-98f0-4f9b-9b35-96268bbd5469	CREATE	INTERNAL	{"name": "uid-test-binding", "ag_ref": "uid-test-ag", "host_ref": "uid-test-host", "namespace": "incloud-sgroups"}	\N	[{"name": "uid-test-host", "type": "Host", "namespace": "incloud-sgroups"}, {"name": "uid-test-ag", "type": "AddressGroup", "namespace": "incloud-sgroups"}]	FAILED_PERMANENT	20	5	2025-10-16 23:06:10.928746+00	waiting for affected resources: Waiting for: Host/uid-test-host, AddressGroup/uid-test-ag	temporary	2025-10-16 22:40:59.68042+00	2025-10-16 23:06:10.946864+00	2025-10-16 23:06:10.946864+00	incloud-sgroups	uid-test-binding
24e33e21-e138-41c3-ab69-a5fa89b169c0	Host	550e8400-cccc-cccc-cccc-446655440099	UPDATE	SGROUP	{"name": "debug-test-logging-trace", "uuid": "550e8400-cccc-cccc-cccc-446655440099", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "UPDATE"}	\N	\N	PENDING	0	5	2025-10-15 22:54:43.670682+00	\N	\N	2025-10-15 22:54:43.670682+00	2025-10-15 22:54:43.670682+00	\N	incloud-sgroups	debug-test-logging-trace
bbc54504-6c4b-4bce-860e-4698855671e4	AddressGroup	242fd1c4-23ad-5d4e-9bed-350ef3cd574c	CREATE	SGROUP	{"name": "bug008-ag4", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 07:19:59.30514+00	\N	\N	2025-10-17 07:19:59.30514+00	2025-10-17 07:19:59.30514+00	\N	incloud-sgroups	bug008-ag4
76dd0253-c3e3-4d7e-bd57-26cfe91bb18c	AddressGroup	6488aab0-f63e-435c-9ffa-0c8bcc2fdb09	CREATE	SGROUP	{"logs": false, "name": "bug008-ag4", "hosts": null, "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 07:19:59.308759+00	2025-10-17 07:19:59.308759+00	\N	incloud-sgroups	bug008-ag4
3ee14f16-e18a-4c7b-9e2b-b8fd4f11d456	HostBinding	d397b47d-3787-4dc2-a7d7-ac6273d673fa	CREATE	INTERNAL	{"name": "uid-test-binding", "ag_ref": "uid-test-ag", "host_ref": "uid-test-host", "namespace": "incloud-sgroups"}	\N	[{"name": "uid-test-host", "type": "Host", "namespace": "incloud-sgroups"}, {"name": "uid-test-ag", "type": "AddressGroup", "namespace": "incloud-sgroups"}]	FAILED_PERMANENT	20	5	2025-10-16 23:08:50.923685+00	waiting for affected resources: Waiting for: Host/uid-test-host, AddressGroup/uid-test-ag	temporary	2025-10-16 22:58:23.717995+00	2025-10-16 23:09:00.926287+00	2025-10-16 23:09:00.926287+00	incloud-sgroups	uid-test-binding
9e3b51fe-12aa-4871-ac08-0f7417ac2654	HostBinding	e45769a2-98a1-4e7f-b827-cceb5baef726	DELETE	INTERNAL	{"host": {"name": "test1-host", "namespace": "incloud-sgroups"}, "name": "test1-binding", "migration": "036", "namespace": "incloud-sgroups", "address_group": {"name": "test1-ag", "namespace": "incloud-sgroups"}}	\N	\N	PENDING	0	5	2025-10-17 06:43:46.971882+00	\N	\N	2025-10-17 06:43:46.971882+00	2025-10-17 06:43:46.971882+00	\N	incloud-sgroups	test1-binding
070ce374-277e-4755-b4f6-fa883c214d1b	Host	550e8400-e29b-41d4-a716-446655440099	CREATE	SGROUP	{"name": "trace-test-host", "uuid": "550e8400-e29b-41d4-a716-446655440099", "ip_list": null, "hostname": "", "is_bound": false, "namespace": "incloud-sgroups", "address_group_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-15 22:38:16.214611+00	2025-10-15 23:30:58.140158+00	\N	incloud-sgroups	trace-test-host
f57ebadb-382a-4642-9315-747a7d6625ab	AddressGroup	f39263d3-1338-53ac-9f50-8c543d755028	CREATE	SGROUP	{"name": "trace-test-ag", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-15 23:31:14.245032+00	\N	\N	2025-10-15 23:31:14.245032+00	2025-10-15 23:31:14.245032+00	\N	incloud-sgroups	trace-test-ag
4a1556c4-918a-4690-b15a-2d98720c868a	AddressGroup	0003e3f9-bf24-46be-9355-7c5ef9b89efd	CREATE	SGROUP	{"logs": false, "name": "trace-test-ag", "hosts": null, "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-15 23:31:14.250052+00	2025-10-15 23:31:14.250052+00	\N	incloud-sgroups	trace-test-ag
53b6a167-368b-4269-bb75-f15220615ef2	HostBinding	710040af-9a3c-466d-bf7c-d4ebd7f17c64	DELETE	INTERNAL	{"host": {"name": "e2e-scenario1-host", "namespace": "incloud-sgroups"}, "name": "e2e-scenario1-binding", "migration": "036", "namespace": "incloud-sgroups", "address_group": {"name": "e2e-scenario1-ag", "namespace": "incloud-sgroups"}}	\N	\N	PENDING	0	5	2025-10-16 23:37:24.44107+00	\N	\N	2025-10-16 23:37:24.44107+00	2025-10-16 23:37:24.44107+00	\N	incloud-sgroups	e2e-scenario1-binding
2e4562be-fd2d-494e-9650-7d5dfd72d4c4	Host	11111111-2222-3333-4444-555555555999	CREATE	SGROUP	{"name": "ready-condition-test-host", "uuid": "11111111-2222-3333-4444-555555555999", "ip_list": null, "hostname": "", "is_bound": false, "namespace": "incloud-sgroups", "address_group_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-15 21:57:54.559985+00	2025-10-15 21:57:54.575462+00	\N	incloud-sgroups	ready-condition-test-host
23c22b69-6d0b-4ae6-af82-8ed25a48541d	Host	11111111-2222-3333-4444-555555555999	UPDATE	SGROUP	{"name": "ready-condition-test-host", "uuid": "11111111-2222-3333-4444-555555555999", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "UPDATE"}	\N	\N	PENDING	0	5	2025-10-15 21:57:54.578296+00	\N	\N	2025-10-15 21:57:54.578296+00	2025-10-15 21:57:54.578296+00	\N	incloud-sgroups	ready-condition-test-host
69ca2663-d97f-4846-bef9-903a3dc52907	Host	22222222-3333-4444-5555-666666666666	CREATE	SGROUP	{"name": "test-ready-bug-v2", "uuid": "22222222-3333-4444-5555-666666666666", "ip_list": null, "hostname": "", "is_bound": false, "namespace": "incloud-sgroups", "address_group_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-15 22:04:35.972325+00	2025-10-15 22:04:35.977526+00	\N	incloud-sgroups	test-ready-bug-v2
d5279819-fc77-45a2-b493-739c7c478b1c	Host	22222222-3333-4444-5555-666666666666	UPDATE	SGROUP	{"name": "test-ready-bug-v2", "uuid": "22222222-3333-4444-5555-666666666666", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "UPDATE"}	\N	\N	PENDING	0	5	2025-10-15 22:04:35.978748+00	\N	\N	2025-10-15 22:04:35.978748+00	2025-10-15 22:04:35.978748+00	\N	incloud-sgroups	test-ready-bug-v2
ec91fd9e-bf43-4483-aba2-a7032b028906	AddressGroup	a64e4cca-560a-5564-ab10-a742f2c70f2b	CREATE	SGROUP	{"name": "test-ag-baseline", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-15 22:12:24.367064+00	\N	\N	2025-10-15 22:12:24.367064+00	2025-10-15 22:12:24.367064+00	\N	incloud-sgroups	test-ag-baseline
56c8fde8-8de0-4908-b839-caba126ca9a7	AddressGroup	346f3034-eb3e-43fa-8ffc-496b2b67a798	CREATE	SGROUP	{"logs": false, "name": "test-ag-baseline", "hosts": null, "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-15 22:12:24.370985+00	2025-10-15 22:12:24.370985+00	\N	incloud-sgroups	test-ag-baseline
619d5029-88fb-428b-a932-d3e731178a2f	Host	33333333-4444-5555-6666-777777777777	CREATE	SGROUP	{"name": "test-host-baseline", "uuid": "33333333-4444-5555-6666-777777777777", "ip_list": null, "hostname": "", "is_bound": false, "namespace": "incloud-sgroups", "address_group_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-15 22:13:01.997986+00	2025-10-15 22:13:01.999876+00	\N	incloud-sgroups	test-host-baseline
a4304220-23bf-43f2-b156-847b42d3b97d	Host	33333333-4444-5555-6666-777777777777	UPDATE	SGROUP	{"name": "test-host-baseline", "uuid": "33333333-4444-5555-6666-777777777777", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "UPDATE"}	\N	\N	PENDING	0	5	2025-10-15 22:13:02.00107+00	\N	\N	2025-10-15 22:13:02.00107+00	2025-10-15 22:13:02.00107+00	\N	incloud-sgroups	test-host-baseline
cbd96d3c-df1e-4948-b697-393999080b30	HostBinding	ab91c3a9-867d-4707-930d-05bf2f33b913	DELETE	INTERNAL	{"host": {"name": "uid-test-host", "namespace": "incloud-sgroups"}, "name": "uid-test-binding", "migration": "036", "namespace": "incloud-sgroups", "address_group": {"name": "uid-test-ag", "namespace": "incloud-sgroups"}}	\N	\N	PENDING	0	5	2025-10-16 22:56:14.717037+00	\N	\N	2025-10-16 22:56:14.717037+00	2025-10-16 22:56:14.717037+00	\N	incloud-sgroups	uid-test-binding
343549a0-bf2d-4ebb-a632-383589c82b0f	Network	37e443f9-fd86-5cc5-bc3f-047ed266d79a	UPDATE	SGROUP	{"cidr": "10.8.99.0/24", "name": "bug008-net-test", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "UPDATE"}	\N	\N	PENDING	0	5	2025-10-17 07:28:07.72133+00	\N	\N	2025-10-17 07:28:07.72133+00	2025-10-17 07:28:07.72133+00	\N	incloud-sgroups	bug008-net-test
f2400aac-819b-46d1-95d8-472fb3e8c220	AddressGroup	1ea31a43-5ca2-5626-ac79-9ff1d5b7dfd2	CREATE	SGROUP	{"name": "bug008-ag-test", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 07:28:17.67101+00	\N	\N	2025-10-17 07:28:17.67101+00	2025-10-17 07:28:17.67101+00	\N	incloud-sgroups	bug008-ag-test
0d03c844-1c0f-47a3-be95-da31bda9094a	AddressGroup	3c71f471-0b3a-4903-afd8-9a916258db29	CREATE	SGROUP	{"logs": false, "name": "bug008-ag-test", "hosts": null, "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 07:28:17.673487+00	2025-10-17 07:28:17.673487+00	\N	incloud-sgroups	bug008-ag-test
b7356bc5-d660-4ace-92c0-5d4075e5301f	NetworkBinding	ff21a6ad-d738-4d4d-aa67-7a430218eeda	CREATE	INTERNAL	{"name": "dynamic-199ee35c0c4cb5f3", "ag_ref": "p0-fix-test-ag", "namespace": "incloud-sgroups", "network_ref": "network-condition-fix-test"}	\N	[{"name": "network-condition-fix-test", "type": "Network", "namespace": "incloud-sgroups"}, {"name": "p0-fix-test-ag", "type": "AddressGroup", "namespace": "incloud-sgroups"}]	PENDING	0	5	\N	\N	\N	2025-10-16 18:08:44.232869+00	2025-10-16 18:08:44.239643+00	\N	incloud-sgroups	dynamic-199ee35c0c4cb5f3
7d9f6ba5-adaf-4f9b-9d57-2b4d95da0d6e	AddressGroup	5698b303-7c8c-528c-aa6a-e748402a0b8c	CREATE	SGROUP	{"name": "final-test-ag", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 06:45:36.022128+00	\N	\N	2025-10-17 06:45:36.022128+00	2025-10-17 06:45:36.022128+00	\N	incloud-sgroups	final-test-ag
85dfb2ca-a3dd-439a-b046-2f5f4a084580	AddressGroup	dfb8a89e-df70-4ac3-bf60-4f65de6193f2	CREATE	SGROUP	{"logs": false, "name": "final-test-ag", "hosts": [{"kind": "Host", "name": "final-test-host", "apiVersion": "netguard.sgroups.io/v1beta1"}], "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 06:45:36.026983+00	2025-10-17 06:45:36.026983+00	\N	incloud-sgroups	final-test-ag
c5ff1378-9ff6-4bee-bbf6-fe470dc023e9	Host	11111111-1111-1111-1111-111111111111	UPDATE	SGROUP	{"name": "test1-host", "uuid": "11111111-1111-1111-1111-111111111111", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "UPDATE"}	\N	\N	PENDING	0	5	2025-10-17 06:43:46.976844+00	\N	\N	2025-10-17 06:43:46.976844+00	2025-10-17 06:43:46.976844+00	\N	incloud-sgroups	test1-host
82ceaef5-c424-4dd1-94a5-de8d7162b1ce	Host	11111111-1111-1111-1111-111111111111	CREATE	SGROUP	{"name": "test1-host", "uuid": "11111111-1111-1111-1111-111111111111", "ip_list": null, "hostname": "", "is_bound": false, "namespace": "incloud-sgroups", "address_group_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 06:43:46.980329+00	2025-10-17 06:43:46.980329+00	\N	incloud-sgroups	test1-host
571c0041-8cf8-41ea-9c1d-aa2a6f587af8	Host	00000008-0002-0002-0002-000000000002	CREATE	SGROUP	{"name": "bug008-host2", "uuid": "00000008-0002-0002-0002-000000000002", "ip_list": null, "hostname": "", "is_bound": false, "namespace": "incloud-sgroups", "address_group_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 07:19:39.648967+00	2025-10-17 07:19:39.653328+00	\N	incloud-sgroups	bug008-host2
9dd762b2-99c8-4eae-86c2-11c85a1b1251	HostBinding	c0213766-50a5-4383-b020-5d8def3677c5	DELETE	INTERNAL	{"host": {"name": "uid-test-host", "namespace": "incloud-sgroups"}, "name": "uid-test-binding", "migration": "036", "namespace": "incloud-sgroups", "address_group": {"name": "uid-test-ag", "namespace": "incloud-sgroups"}}	\N	\N	PENDING	0	5	2025-10-16 23:02:30.798734+00	\N	\N	2025-10-16 23:02:30.798734+00	2025-10-16 23:02:30.798734+00	\N	incloud-sgroups	uid-test-binding
dc11e120-d339-4eaf-8972-b7ad07a3f894	Host	aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee	UPDATE	SGROUP	{"name": "uid-test-host", "uuid": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "UPDATE"}	\N	\N	PENDING	0	5	2025-10-16 23:02:30.801117+00	\N	\N	2025-10-16 23:02:30.801117+00	2025-10-16 23:02:30.801117+00	\N	incloud-sgroups	uid-test-host
cd398c54-ed92-4c6b-b511-6908a515884a	Host	aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee	CREATE	SGROUP	{"name": "uid-test-host", "uuid": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "ip_list": null, "hostname": "", "is_bound": false, "namespace": "incloud-sgroups", "address_group_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-16 23:02:30.803145+00	2025-10-16 23:02:30.803145+00	\N	incloud-sgroups	uid-test-host
b39a2895-fd42-44c3-aa34-079b1fd7dd37	AddressGroup	9b8c55f5-7af2-5656-99d3-e0722ea2b08d	CREATE	SGROUP	{"name": "test-ag-203", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 08:27:10.313873+00	\N	\N	2025-10-17 08:27:10.313873+00	2025-10-17 08:27:10.313873+00	\N	incloud-sgroups	test-ag-203
da5715e6-e5dc-445b-9787-f8bbbe39fd63	AddressGroup	98c09fe0-f653-4dc4-b0d4-5fb57b5d80cc	CREATE	SGROUP	{"logs": false, "name": "test-ag-203", "hosts": null, "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 08:27:10.31541+00	2025-10-17 08:27:10.31541+00	\N	incloud-sgroups	test-ag-203
1c6f7197-8932-48fe-b0ca-f4d88b591bd7	AddressGroup	15b8e6d0-aeef-5890-a8ee-54b00143211f	CREATE	SGROUP	{"name": "e2e-test-ag", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 06:44:29.411594+00	\N	\N	2025-10-17 06:44:29.411594+00	2025-10-17 06:44:29.411594+00	\N	incloud-sgroups	e2e-test-ag
37114a54-b1fd-495c-b083-e0a226c52eba	AddressGroup	367b8e70-c4b6-4fd1-804a-d32325efce52	CREATE	SGROUP	{"logs": false, "name": "e2e-test-ag", "hosts": [{"kind": "Host", "name": "e2e-test-host", "apiVersion": "netguard.sgroups.io/v1beta1"}], "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 06:44:29.417512+00	2025-10-17 06:44:29.417512+00	\N	incloud-sgroups	e2e-test-ag
15c0f7a0-5676-4ad8-8b97-33b2fded9bed	NetworkBinding	811a93e2-de22-479b-82f3-b9cac83bb3c4	DELETE	INTERNAL	{"name": "bug008-nb1", "network": {"name": "bug008-net1", "namespace": "incloud-sgroups"}, "migration": "036", "namespace": "incloud-sgroups", "address_group": {"name": "bug008-ag4", "namespace": "incloud-sgroups"}}	\N	\N	PENDING	0	5	2025-10-17 07:20:06.770641+00	\N	\N	2025-10-17 07:20:06.770641+00	2025-10-17 07:20:06.770641+00	\N	incloud-sgroups	bug008-nb1
668d35c7-2d33-432f-99ac-147acd17e9df	AddressGroup	1c1360e5-6e41-5186-8778-fccb5025fa2a	CREATE	SGROUP	{"name": "bug009-ag-test", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 07:46:50.425991+00	\N	\N	2025-10-17 07:46:50.425991+00	2025-10-17 07:46:50.425991+00	\N	incloud-sgroups	bug009-ag-test
97e37f93-a656-4421-9ed5-0e1a00b9dfca	AddressGroup	73eb2d00-9362-4f22-921d-7fbe89b9cbcf	CREATE	SGROUP	{"logs": false, "name": "bug009-ag-test", "hosts": null, "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 07:46:50.429992+00	2025-10-17 07:46:50.429992+00	\N	incloud-sgroups	bug009-ag-test
b87ad159-9e22-476d-814e-e0c1381ee981	NetworkBinding	3a1a7ca6-f184-413b-8549-407b041ea0ea	CREATE	INTERNAL	{"name": "bug008-nb2", "ag_ref": "bug008-ag5", "namespace": "incloud-sgroups", "network_ref": "bug008-net2"}	\N	[{"name": "bug008-net2", "type": "Network", "namespace": "incloud-sgroups"}, {"name": "bug008-ag5", "type": "AddressGroup", "namespace": "incloud-sgroups"}]	FAILED_PERMANENT	20	5	2025-10-17 07:27:07.581768+00	waiting for affected resources: Waiting for: Network/bug008-net2, AddressGroup/bug008-ag5	temporary	2025-10-17 07:20:18.454962+00	2025-10-17 07:27:17.58902+00	2025-10-17 07:27:17.58902+00	incloud-sgroups	bug008-nb2
b15e43b1-ff19-468e-8a43-e0d884f96d5c	HostBinding	d99d3f94-9670-4943-8d4a-8fa313508b5a	CREATE	INTERNAL	{"name": "test1-binding", "ag_ref": "test1-ag", "host_ref": "test1-host", "namespace": "incloud-sgroups"}	\N	[{"name": "test1-host", "type": "Host", "namespace": "incloud-sgroups"}, {"name": "test1-ag", "type": "AddressGroup", "namespace": "incloud-sgroups"}]	FAILED_PERMANENT	20	5	2025-10-17 06:50:11.996555+00	waiting for affected resources: Waiting for: Host/test1-host, AddressGroup/test1-ag	temporary	2025-10-17 06:42:45.832672+00	2025-10-17 06:50:22.199879+00	2025-10-17 06:50:22.199879+00	incloud-sgroups	test1-binding
a8120eab-6972-4201-88a8-1ae5f322f7ff	HostBinding	f920e93e-4481-4cad-83a9-025f8f4292f4	CREATE	INTERNAL	{"name": "e2e-scenario1-binding", "ag_ref": "e2e-scenario1-ag", "host_ref": "e2e-scenario1-host", "namespace": "incloud-sgroups"}	\N	[{"name": "e2e-scenario1-host", "type": "Host", "namespace": "incloud-sgroups"}, {"name": "e2e-scenario1-ag", "type": "AddressGroup", "namespace": "incloud-sgroups"}]	FAILED_PERMANENT	20	5	2025-10-16 23:44:40.96545+00	waiting for affected resources: Waiting for: Host/e2e-scenario1-host, AddressGroup/e2e-scenario1-ag	temporary	2025-10-16 23:31:20.645157+00	2025-10-16 23:44:50.962219+00	2025-10-16 23:44:50.962219+00	incloud-sgroups	e2e-scenario1-binding
805f336a-637e-4e72-bf34-d094b5f70b23	HostBinding	e002469c-0d4f-4daf-8429-346ad7945ea9	DELETE	INTERNAL	{"host": {"name": "bug007-test-host", "namespace": "incloud-sgroups"}, "name": "bug007-test-binding", "migration": "036", "namespace": "incloud-sgroups", "address_group": {"name": "bug007-test-ag", "namespace": "incloud-sgroups"}}	\N	\N	PENDING	0	5	2025-10-17 06:33:32.163319+00	\N	\N	2025-10-17 06:33:32.163319+00	2025-10-17 06:33:32.163319+00	\N	incloud-sgroups	bug007-test-binding
ba4e812b-3ea1-4959-be14-24e8435818e3	Service	68d87dea-5848-5b03-be42-bd5083f2ee75	CREATE	SGROUP	{"name": "m036-delete-test", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-16 11:28:28.664337+00	\N	\N	2025-10-16 11:28:28.664337+00	2025-10-16 11:28:28.664337+00	\N	incloud-sgroups	m036-delete-test
2a536fcd-1ea9-47b4-8924-aab0380b7032	Service	0e33b969-9a6d-4b67-92c1-d991984b5777	CREATE	SGROUP	{"name": "m036-delete-test", "namespace": "incloud-sgroups", "description": "DELETE trigger test", "ingress_ports": [{"Port": "9999", "Protocol": "TCP", "Description": ""}], "address_groups": null}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-16 11:28:28.668998+00	2025-10-16 11:28:28.668998+00	\N	incloud-sgroups	m036-delete-test
85ad4c62-bb23-42d3-a06f-4494c4153bde	Service	68d87dea-5848-5b03-be42-bd5083f2ee75	UPDATE	SGROUP	{"name": "m036-delete-test", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "UPDATE"}	\N	\N	PENDING	0	5	2025-10-16 11:28:28.664337+00	\N	\N	2025-10-16 11:28:28.664337+00	2025-10-16 11:28:28.664337+00	\N	incloud-sgroups	m036-delete-test
2cb5e632-d978-4947-90da-322cdad4998d	HostBinding	754ee893-f849-4e5a-bb3e-5441b7eaab7e	CREATE	INTERNAL	{"name": "dynamic-199f0f2a8626e714", "ag_ref": "e2e-scenario2-ag", "host_ref": "dynamic-199ee3b35bf19eb0", "namespace": "incloud-sgroups"}	\N	[{"name": "dynamic-199ee3b35bf19eb0", "type": "Host", "namespace": "incloud-sgroups"}, {"name": "e2e-scenario2-ag", "type": "AddressGroup", "namespace": "incloud-sgroups"}]	FAILED_PERMANENT	20	5	2025-10-17 07:00:51.991341+00	waiting for affected resources: Waiting for: AddressGroup/e2e-scenario2-ag	temporary	2025-10-17 06:54:18.729929+00	2025-10-17 07:01:02.006863+00	2025-10-17 07:01:02.006863+00	incloud-sgroups	dynamic-199f0f2a8626e714
1609abb9-5b2a-4a0e-9c32-6b446f697696	Host	00000008-0001-0001-0001-000000000001	UPDATE	SGROUP	{"name": "bug008-host1", "uuid": "00000008-0001-0001-0001-000000000001", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "UPDATE"}	\N	\N	PENDING	0	5	2025-10-17 07:19:28.442254+00	\N	\N	2025-10-17 07:19:28.442254+00	2025-10-17 07:19:28.442254+00	\N	incloud-sgroups	bug008-host1
39b55295-256b-4ae6-822c-8938c809e625	Host	00000008-0003-0003-0003-000000000003	UPDATE	SGROUP	{"name": "bug008-host3", "uuid": "00000008-0003-0003-0003-000000000003", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "UPDATE"}	\N	\N	PENDING	0	5	2025-10-17 07:19:48.422569+00	\N	\N	2025-10-17 07:19:48.422569+00	2025-10-17 07:19:48.422569+00	\N	incloud-sgroups	bug008-host3
165aca53-efe7-47ac-b164-d0908c20da8c	Network	3957f381-955b-4ab8-9a62-26d32e5af43e	CREATE	SGROUP	{"cidr": "10.8.1.0/24", "name": "bug008-net1", "is_bound": false, "namespace": "incloud-sgroups", "network_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 07:20:06.772891+00	2025-10-17 07:20:06.772891+00	\N	incloud-sgroups	bug008-net1
144b6c97-c963-4b29-89dd-8d04216a1e43	HostBinding	2436345c-226b-4c63-bf6c-9ac66bb8d6ec	CREATE	INTERNAL	{"name": "bug007-test-binding", "ag_ref": "bug007-test-ag", "host_ref": "bug007-test-host", "namespace": "incloud-sgroups"}	\N	[{"name": "bug007-test-host", "type": "Host", "namespace": "incloud-sgroups"}, {"name": "bug007-test-ag", "type": "AddressGroup", "namespace": "incloud-sgroups"}]	FAILED_PERMANENT	20	5	2025-10-17 06:41:11.966687+00	waiting for affected resources: Waiting for: Host/bug007-test-host, AddressGroup/bug007-test-ag	temporary	2025-10-17 06:32:34.644009+00	2025-10-17 06:41:21.9743+00	2025-10-17 06:41:21.9743+00	incloud-sgroups	bug007-test-binding
8209aab5-2439-4eb3-82a6-f4764e70d73a	NetworkBinding	794916bb-00d9-43eb-b572-0682cca88b00	CREATE	INTERNAL	{"name": "bug008-nb1", "ag_ref": "bug008-ag4", "namespace": "incloud-sgroups", "network_ref": "bug008-net1"}	\N	[{"name": "bug008-net1", "type": "Network", "namespace": "incloud-sgroups"}, {"name": "bug008-ag4", "type": "AddressGroup", "namespace": "incloud-sgroups"}]	FAILED_PERMANENT	20	5	2025-10-17 07:26:37.612321+00	waiting for affected resources: Waiting for: Network/bug008-net1, AddressGroup/bug008-ag4	temporary	2025-10-17 07:20:06.570484+00	2025-10-17 07:26:47.609758+00	2025-10-17 07:26:47.609758+00	incloud-sgroups	bug008-nb1
6b3e0367-8ea9-4ebb-bcd0-a83e502c709a	NetworkBinding	8137e1f6-a6d1-45e8-922c-4ac48a018394	DELETE	INTERNAL	{"name": "bug008-nb2", "network": {"name": "bug008-net2", "namespace": "incloud-sgroups"}, "migration": "036", "namespace": "incloud-sgroups", "address_group": {"name": "bug008-ag5", "namespace": "incloud-sgroups"}}	\N	\N	PENDING	0	5	2025-10-17 07:20:18.680705+00	\N	\N	2025-10-17 07:20:18.680705+00	2025-10-17 07:20:18.680705+00	\N	incloud-sgroups	bug008-nb2
180bc610-630b-4538-9e7d-6d6637a0c954	Network	6a0ec3c6-6a42-4b2b-b33c-13aee7d93532	CREATE	SGROUP	{"cidr": "10.8.2.0/24", "name": "bug008-net2", "is_bound": false, "namespace": "incloud-sgroups", "network_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 07:20:18.683273+00	2025-10-17 07:20:18.683273+00	\N	incloud-sgroups	bug008-net2
09500cdf-4316-417d-a8f4-d2b73baf2a82	NetworkBinding	19e1f4fc-d407-4302-8a54-c37934e4a8b2	CREATE	INTERNAL	{"name": "bug008-nb3", "ag_ref": "bug008-ag6", "namespace": "incloud-sgroups", "network_ref": "bug008-net3"}	\N	[{"name": "bug008-net3", "type": "Network", "namespace": "incloud-sgroups"}, {"name": "bug008-ag6", "type": "AddressGroup", "namespace": "incloud-sgroups"}]	FAILED_PERMANENT	20	5	2025-10-17 07:27:17.586395+00	waiting for affected resources: Waiting for: Network/bug008-net3, AddressGroup/bug008-ag6	temporary	2025-10-17 07:20:31.33641+00	2025-10-17 07:27:27.57856+00	2025-10-17 07:27:27.57856+00	incloud-sgroups	bug008-nb3
6af7ab1e-5a67-4f0a-bbdb-9a67551d1527	Network	11f96b2b-bd0a-4824-ae79-6325e6f48647	CREATE	SGROUP	{"cidr": "10.8.3.0/24", "name": "bug008-net3", "is_bound": true, "namespace": "incloud-sgroups", "network_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 07:20:31.338384+00	2025-10-17 07:20:31.338384+00	\N	incloud-sgroups	bug008-net3
cc3f47f4-871f-47b3-9329-e52ee4c95446	NetworkBinding	4f69701e-e8d8-4764-afb7-1174c402e7c5	DELETE	INTERNAL	{"name": "bug008-nb3", "network": {"name": "bug008-net3", "namespace": "incloud-sgroups"}, "migration": "036", "namespace": "incloud-sgroups", "address_group": {"name": "bug008-ag6", "namespace": "incloud-sgroups"}}	\N	\N	PENDING	0	5	2025-10-17 07:20:33.452844+00	\N	\N	2025-10-17 07:20:33.452844+00	2025-10-17 07:20:33.452844+00	\N	incloud-sgroups	bug008-nb3
848828da-f40c-4a9d-8b5e-371c10598643	Network	dcd24c0d-8194-536e-8c1a-8019d98649a5	UPDATE	SGROUP	{"cidr": "10.8.3.0/24", "name": "bug008-net3", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "UPDATE"}	\N	\N	PENDING	0	5	2025-10-17 07:20:33.454607+00	\N	\N	2025-10-17 07:20:28.458314+00	2025-10-17 07:20:33.454607+00	\N	incloud-sgroups	bug008-net3
768ff006-58c4-4e4d-abed-9ddddc34c180	Network	11d152df-663f-416b-a241-bf5d6b3d73f6	CREATE	SGROUP	{"cidr": "10.8.3.0/24", "name": "bug008-net3", "is_bound": false, "namespace": "incloud-sgroups", "network_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 07:20:33.455511+00	2025-10-17 07:20:33.455511+00	\N	incloud-sgroups	bug008-net3
ab5ee6fc-3c12-4edc-a898-9a6de1b947fd	AddressGroup	1166c717-4642-5187-8c81-1dacb054938e	CREATE	SGROUP	{"name": "test-ag-007", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 08:13:21.007609+00	\N	\N	2025-10-17 08:13:21.007609+00	2025-10-17 08:13:21.007609+00	\N	incloud-sgroups	test-ag-007
a1bec658-31bc-4085-b6d7-f5cc58ee4023	AddressGroup	50d6a511-7083-415c-84e6-da4d0b3b3672	CREATE	SGROUP	{"logs": false, "name": "test-ag-007", "hosts": null, "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 08:13:21.039567+00	2025-10-17 08:13:21.039567+00	\N	incloud-sgroups	test-ag-007
48392f0c-7f1d-4ae6-b16a-1cb46784c581	AddressGroup	b9f12a54-b7ac-5416-bf5b-866bd1dd4015	CREATE	SGROUP	{"name": "test-ag-101", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 08:24:10.876346+00	\N	\N	2025-10-17 08:24:10.876346+00	2025-10-17 08:24:10.876346+00	\N	incloud-sgroups	test-ag-101
9b0a75fb-8e15-4d23-8eed-60df03c85db7	AddressGroup	abcbf068-4fad-43c9-9ba3-fcd315b4a4fe	CREATE	SGROUP	{"logs": false, "name": "test-ag-101", "hosts": null, "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 08:24:10.952173+00	2025-10-17 08:24:10.952173+00	\N	incloud-sgroups	test-ag-101
238ab09c-9f7e-45ef-a802-9e796477ecc7	AddressGroup	8fa419ee-4fe0-5350-b891-ecda96880d3c	CREATE	SGROUP	{"name": "test-ag-102", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 08:24:13.527616+00	\N	\N	2025-10-17 08:24:13.527616+00	2025-10-17 08:24:13.527616+00	\N	incloud-sgroups	test-ag-102
1d0a36b8-9fbd-483a-b822-1c531bfe04f7	AddressGroup	1a3baddd-8b59-41e3-82e5-36d3b8942864	CREATE	SGROUP	{"logs": false, "name": "test-ag-102", "hosts": null, "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 08:24:13.529514+00	2025-10-17 08:24:13.529514+00	\N	incloud-sgroups	test-ag-102
014896e4-6b62-4496-8230-70a920caffde	AddressGroup	cabc3843-a8cd-525f-b659-a6eba3445389	CREATE	SGROUP	{"name": "test-ag-103", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 08:24:16.517222+00	\N	\N	2025-10-17 08:24:16.517222+00	2025-10-17 08:24:16.517222+00	\N	incloud-sgroups	test-ag-103
26b5ba0b-5031-44ed-a6ec-5dfd1d3ecf90	AddressGroup	1aefa61b-0e23-443b-9353-79ee68cee71a	CREATE	SGROUP	{"logs": false, "name": "test-ag-103", "hosts": null, "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 08:24:16.519561+00	2025-10-17 08:24:16.519561+00	\N	incloud-sgroups	test-ag-103
bd11c790-ca84-4483-884d-b01105d10c63	Host	550e8400-e29b-41d4-a716-446655440001	CREATE	SGROUP	{"name": "test-host-001", "uuid": "550e8400-e29b-41d4-a716-446655440001", "ip_list": null, "hostname": "", "is_bound": false, "namespace": "incloud-sgroups", "address_group_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-18 12:06:53.972121+00	2025-10-18 12:08:18.084469+00	\N	incloud-sgroups	test-host-001
c3c4835b-d833-4075-a859-a3ce25482cf3	AddressGroup	b4cf0563-1b42-5c97-86ec-f490b023295f	CREATE	SGROUP	{"name": "test-ag-105", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 08:24:22.431449+00	\N	\N	2025-10-17 08:24:22.431449+00	2025-10-17 08:24:22.431449+00	\N	incloud-sgroups	test-ag-105
773323d7-c684-488d-acb8-f965ac44aba0	AddressGroup	233986c8-5739-4e54-9d50-fb28c47e29f2	CREATE	SGROUP	{"logs": false, "name": "test-ag-105", "hosts": null, "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 08:24:22.432939+00	2025-10-17 08:24:22.432939+00	\N	incloud-sgroups	test-ag-105
c1c09940-0480-4114-a2f8-8f008885fc20	AddressGroup	98851602-90fb-53ef-828c-dd69b6d0206b	CREATE	SGROUP	{"name": "test-ag-201", "migration": "035", "namespace": "incloud-sgroups", "trigger_reason": "INSERT"}	\N	\N	PENDING	0	5	2025-10-17 08:26:55.229988+00	\N	\N	2025-10-17 08:26:55.229988+00	2025-10-17 08:26:55.229988+00	\N	incloud-sgroups	test-ag-201
91a7f97e-d2e0-4793-8961-f367390ba33c	AddressGroup	aa9cb831-d147-4652-a5a6-db3df86f519e	CREATE	SGROUP	{"logs": false, "name": "test-ag-201", "hosts": null, "trace": false, "networks": null, "namespace": "incloud-sgroups", "default_action": "ACCEPT"}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-17 08:26:55.237546+00	2025-10-17 08:26:55.237546+00	\N	incloud-sgroups	test-ag-201
\.


--
-- Data for Name: sync_status; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.sync_status (id, updated_at) FROM stdin;
1	2025-10-10 13:18:54.991713+00
\.


--
-- Name: k8s_metadata_resource_version_seq; Type: SEQUENCE SET; Schema: public; Owner: netguard
--

SELECT pg_catalog.setval('public.k8s_metadata_resource_version_seq', 309, true);


--
-- Name: netguard_db_ver_id_seq; Type: SEQUENCE SET; Schema: public; Owner: netguard
--

SELECT pg_catalog.setval('public.netguard_db_ver_id_seq', 39, true);


--
-- Name: address_group_binding_policies address_group_binding_policies_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.address_group_binding_policies
    ADD CONSTRAINT address_group_binding_policies_pkey PRIMARY KEY (namespace, name);


--
-- Name: address_group_bindings address_group_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.address_group_bindings
    ADD CONSTRAINT address_group_bindings_pkey PRIMARY KEY (namespace, name);


--
-- Name: address_group_port_mappings address_group_port_mappings_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.address_group_port_mappings
    ADD CONSTRAINT address_group_port_mappings_pkey PRIMARY KEY (namespace, name);


--
-- Name: address_groups address_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.address_groups
    ADD CONSTRAINT address_groups_pkey PRIMARY KEY (namespace, name);


--
-- Name: host_bindings host_bindings_host_namespace_host_name_key; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.host_bindings
    ADD CONSTRAINT host_bindings_host_namespace_host_name_key UNIQUE (host_namespace, host_name);


--
-- Name: host_bindings host_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.host_bindings
    ADD CONSTRAINT host_bindings_pkey PRIMARY KEY (namespace, name);


--
-- Name: hosts hosts_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT hosts_pkey PRIMARY KEY (namespace, name);


--
-- Name: hosts hosts_uuid_key; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT hosts_uuid_key UNIQUE (uuid);


--
-- Name: ie_ag_ag_rules ie_ag_ag_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.ie_ag_ag_rules
    ADD CONSTRAINT ie_ag_ag_rules_pkey PRIMARY KEY (namespace, name);


--
-- Name: k8s_metadata k8s_metadata_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.k8s_metadata
    ADD CONSTRAINT k8s_metadata_pkey PRIMARY KEY (resource_version);


--
-- Name: k8s_metadata k8s_metadata_uid_key; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.k8s_metadata
    ADD CONSTRAINT k8s_metadata_uid_key UNIQUE (uid);


--
-- Name: netguard_db_ver netguard_db_ver_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.netguard_db_ver
    ADD CONSTRAINT netguard_db_ver_pkey PRIMARY KEY (id);


--
-- Name: network_bindings network_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.network_bindings
    ADD CONSTRAINT network_bindings_pkey PRIMARY KEY (namespace, name);


--
-- Name: networks networks_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.networks
    ADD CONSTRAINT networks_pkey PRIMARY KEY (namespace, name);


--
-- Name: networks prevent_networks_cidr_overlap; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.networks
    ADD CONSTRAINT prevent_networks_cidr_overlap EXCLUDE USING gist (cidr inet_ops WITH &&) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: CONSTRAINT prevent_networks_cidr_overlap ON networks; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON CONSTRAINT prevent_networks_cidr_overlap ON public.networks IS 'Prevents overlapping CIDR ranges using PostgreSQL GIST index. Error code: 23P01 (exclusion_violation)';


--
-- Name: rule_s2s rule_s2s_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.rule_s2s
    ADD CONSTRAINT rule_s2s_pkey PRIMARY KEY (namespace, name);


--
-- Name: service_aliases service_aliases_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.service_aliases
    ADD CONSTRAINT service_aliases_pkey PRIMARY KEY (namespace, name);


--
-- Name: services services_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.services
    ADD CONSTRAINT services_pkey PRIMARY KEY (namespace, name);


--
-- Name: sync_outbox sync_outbox_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.sync_outbox
    ADD CONSTRAINT sync_outbox_pkey PRIMARY KEY (id);


--
-- Name: sync_status sync_status_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.sync_status
    ADD CONSTRAINT sync_status_pkey PRIMARY KEY (id);


--
-- Name: sync_outbox uq_sync_outbox_resource_op; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.sync_outbox
    ADD CONSTRAINT uq_sync_outbox_resource_op UNIQUE (resource_type, resource_id, operation, target_system);


--
-- Name: idx_address_group_binding_policies_ag_ref; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_address_group_binding_policies_ag_ref ON public.address_group_binding_policies USING gin (address_group_ref);


--
-- Name: idx_address_group_binding_policies_service_ref; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_address_group_binding_policies_service_ref ON public.address_group_binding_policies USING gin (service_ref);


--
-- Name: idx_address_group_bindings_ag; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_address_group_bindings_ag ON public.address_group_bindings USING btree (address_group_namespace, address_group_name);


--
-- Name: idx_address_group_bindings_service; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_address_group_bindings_service ON public.address_group_bindings USING btree (service_namespace, service_name);


--
-- Name: idx_address_group_port_mappings_access_ports; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_address_group_port_mappings_access_ports ON public.address_group_port_mappings USING gin (access_ports);


--
-- Name: idx_address_groups_aggregated_hosts; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_address_groups_aggregated_hosts ON public.address_groups USING gin (aggregated_hosts);


--
-- Name: idx_address_groups_hosts; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_address_groups_hosts ON public.address_groups USING gin (hosts);


--
-- Name: idx_address_groups_namespace; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_address_groups_namespace ON public.address_groups USING btree (namespace);


--
-- Name: idx_address_groups_networks; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_address_groups_networks ON public.address_groups USING gin (networks);


--
-- Name: idx_host_bindings_address_group; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_host_bindings_address_group ON public.host_bindings USING btree (address_group_namespace, address_group_name);


--
-- Name: idx_host_bindings_host; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_host_bindings_host ON public.host_bindings USING btree (host_namespace, host_name);


--
-- Name: idx_hosts_address_group_name; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_hosts_address_group_name ON public.hosts USING btree (address_group_name) WHERE (address_group_name IS NOT NULL);


--
-- Name: idx_hosts_ip_list; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_hosts_ip_list ON public.hosts USING gin (ip_list);


--
-- Name: idx_hosts_is_bound; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_hosts_is_bound ON public.hosts USING btree (is_bound);


--
-- Name: idx_hosts_ready; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_hosts_ready ON public.hosts USING btree (ready) WHERE (ready = false);


--
-- Name: idx_hosts_uuid; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_hosts_uuid ON public.hosts USING btree (uuid);


--
-- Name: idx_ie_ag_ag_rules_local; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_ie_ag_ag_rules_local ON public.ie_ag_ag_rules USING btree (address_group_local_namespace, address_group_local_name);


--
-- Name: idx_ie_ag_ag_rules_ports; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_ie_ag_ag_rules_ports ON public.ie_ag_ag_rules USING gin (ports);


--
-- Name: idx_ie_ag_ag_rules_target; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_ie_ag_ag_rules_target ON public.ie_ag_ag_rules USING btree (address_group_namespace, address_group_name);


--
-- Name: idx_ie_ag_ag_rules_trace; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_ie_ag_ag_rules_trace ON public.ie_ag_ag_rules USING btree (trace);


--
-- Name: idx_k8s_metadata_active_conditions; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_k8s_metadata_active_conditions ON public.k8s_metadata USING btree (resource_version) WHERE ((conditions <> '[]'::jsonb) AND (conditions IS NOT NULL));


--
-- Name: idx_k8s_metadata_annotations; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_k8s_metadata_annotations ON public.k8s_metadata USING gin (annotations);


--
-- Name: idx_k8s_metadata_conditions; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_k8s_metadata_conditions ON public.k8s_metadata USING gin (conditions);


--
-- Name: idx_k8s_metadata_conditions_size; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_k8s_metadata_conditions_size ON public.k8s_metadata USING btree (jsonb_array_length(conditions), resource_version) WHERE ((conditions IS NOT NULL) AND (jsonb_typeof(conditions) = 'array'::text));


--
-- Name: idx_k8s_metadata_conditions_updated_at; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_k8s_metadata_conditions_updated_at ON public.k8s_metadata USING btree (updated_at DESC, resource_version) WHERE (conditions <> '[]'::jsonb);


--
-- Name: idx_k8s_metadata_deletion_timestamp; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_k8s_metadata_deletion_timestamp ON public.k8s_metadata USING btree (deletion_timestamp) WHERE (deletion_timestamp IS NOT NULL);


--
-- Name: idx_k8s_metadata_labels; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_k8s_metadata_labels ON public.k8s_metadata USING gin (labels);


--
-- Name: idx_k8s_metadata_resource_version_conditions; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_k8s_metadata_resource_version_conditions ON public.k8s_metadata USING btree (resource_version, updated_at DESC);


--
-- Name: idx_k8s_metadata_uid; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_k8s_metadata_uid ON public.k8s_metadata USING btree (uid);


--
-- Name: idx_network_bindings_ag; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_network_bindings_ag ON public.network_bindings USING btree (address_group_namespace, address_group_name);


--
-- Name: idx_network_bindings_network; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_network_bindings_network ON public.network_bindings USING btree (network_namespace, network_name);


--
-- Name: idx_networks_namespace; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_networks_namespace ON public.networks USING btree (namespace);


--
-- Name: idx_networks_network_items; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_networks_network_items ON public.networks USING gin (network_items);


--
-- Name: idx_networks_ready_false; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_networks_ready_false ON public.networks USING btree (namespace, name) WHERE (ready = false);


--
-- Name: idx_rule_s2s_service_local_ref_name; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_rule_s2s_service_local_ref_name ON public.rule_s2s USING btree (((service_local_ref ->> 'name'::text)));


--
-- Name: idx_rule_s2s_service_local_ref_namespace; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_rule_s2s_service_local_ref_namespace ON public.rule_s2s USING btree (((service_local_ref ->> 'namespace'::text)));


--
-- Name: idx_rule_s2s_service_ref_name; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_rule_s2s_service_ref_name ON public.rule_s2s USING btree (((service_ref ->> 'name'::text)));


--
-- Name: idx_rule_s2s_service_ref_namespace; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_rule_s2s_service_ref_namespace ON public.rule_s2s USING btree (((service_ref ->> 'namespace'::text)));


--
-- Name: idx_services_address_groups; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_services_address_groups ON public.services USING gin (address_groups);


--
-- Name: idx_services_aggregated_address_groups; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_services_aggregated_address_groups ON public.services USING gin (aggregated_address_groups);


--
-- Name: idx_services_ingress_ports; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_services_ingress_ports ON public.services USING gin (ingress_ports);


--
-- Name: idx_services_namespace; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_services_namespace ON public.services USING btree (namespace);


--
-- Name: idx_sync_outbox_affects; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_sync_outbox_affects ON public.sync_outbox USING gin (affects_resources) WHERE (affects_resources IS NOT NULL);


--
-- Name: INDEX idx_sync_outbox_affects; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON INDEX public.idx_sync_outbox_affects IS 'GIN index for affected resources lookup. Used to find process resources that affect specific entities (e.g., find all HostBindings affecting a specific Host). Partial index (only non-null affects_resources).';


--
-- Name: idx_sync_outbox_pending; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_sync_outbox_pending ON public.sync_outbox USING btree (status, next_retry_at, created_at) WHERE (status = ANY (ARRAY['PENDING'::public.outbox_status, 'FAILED_RETRYABLE'::public.outbox_status]));


--
-- Name: INDEX idx_sync_outbox_pending; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON INDEX public.idx_sync_outbox_pending IS 'Partial index for worker polling: finds pending/retryable entries ready for processing. Used by OutboxWorker every 5-10 seconds. Covers status, next_retry_at, created_at for efficient ORDER BY.';


--
-- Name: idx_sync_outbox_resource; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_sync_outbox_resource ON public.sync_outbox USING btree (resource_type, resource_id, status);


--
-- Name: INDEX idx_sync_outbox_resource; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON INDEX public.idx_sync_outbox_resource IS 'Index for resource lookup: quickly find outbox entries for a specific resource. Used for ON CONFLICT resolution, resource deletion cleanup, and status checks.';


--
-- Name: idx_sync_outbox_resource_ns_name; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_sync_outbox_resource_ns_name ON public.sync_outbox USING btree (resource_type, resource_namespace, resource_name) WHERE (resource_namespace IS NOT NULL);


--
-- Name: INDEX idx_sync_outbox_resource_ns_name; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON INDEX public.idx_sync_outbox_resource_ns_name IS 'Composite index for efficient resource lookup by (type, namespace, name).
Used by OutboxWorker condition_updater to find entries without full table scan.
Fixes P0-1: Enables fast JOIN with entity tables.';


--
-- Name: address_groups address_group_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER address_group_before_delete BEFORE DELETE ON public.address_groups FOR EACH ROW EXECUTE FUNCTION public.trigger_address_group_before_delete();


--
-- Name: address_group_bindings address_group_binding_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER address_group_binding_before_delete BEFORE DELETE ON public.address_group_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_address_group_binding_before_delete();


--
-- Name: TRIGGER address_group_binding_before_delete ON address_group_bindings; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER address_group_binding_before_delete ON public.address_group_bindings IS 'Migration 036: Sync-First DELETE trigger for AddressGroupBinding.
Ensures affected Service is updated before AddressGroupBinding is removed from database.';


--
-- Name: hosts cascade_host_from_address_groups; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER cascade_host_from_address_groups AFTER DELETE ON public.hosts FOR EACH ROW EXECUTE FUNCTION public.cascade_host_deletion();


--
-- Name: address_groups enforce_host_exclusivity; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER enforce_host_exclusivity BEFORE INSERT OR UPDATE ON public.address_groups FOR EACH ROW WHEN ((new.hosts <> '[]'::jsonb)) EXECUTE FUNCTION public.check_host_exclusivity();


--
-- Name: hosts host_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER host_before_delete BEFORE DELETE ON public.hosts FOR EACH ROW EXECUTE FUNCTION public.trigger_host_before_delete();


--
-- Name: host_bindings host_binding_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER host_binding_before_delete BEFORE DELETE ON public.host_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_host_binding_before_delete();


--
-- Name: TRIGGER host_binding_before_delete ON host_bindings; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER host_binding_before_delete ON public.host_bindings IS 'Migration 036: Sync-First DELETE trigger for HostBinding.
Ensures affected AddressGroup is updated before HostBinding is removed from database.';


--
-- Name: networks network_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER network_before_delete BEFORE DELETE ON public.networks FOR EACH ROW EXECUTE FUNCTION public.trigger_network_before_delete();


--
-- Name: network_bindings network_binding_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER network_binding_before_delete BEFORE DELETE ON public.network_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_network_binding_before_delete();


--
-- Name: TRIGGER network_binding_before_delete ON network_bindings; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER network_binding_before_delete ON public.network_bindings IS 'Migration 036: Sync-First DELETE trigger for NetworkBinding.
Ensures affected AddressGroup is updated before NetworkBinding is removed from database.';


--
-- Name: services service_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER service_before_delete BEFORE DELETE ON public.services FOR EACH ROW EXECUTE FUNCTION public.trigger_service_before_delete();


--
-- Name: TRIGGER service_before_delete ON services; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER service_before_delete ON public.services IS 'Migration 036: Sync-First DELETE trigger for Service.
Ensures Service is deleted from SGROUP before being removed from database.';


--
-- Name: address_group_bindings trg_address_group_binding_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_address_group_binding_upsert_outbox AFTER INSERT OR UPDATE ON public.address_group_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_address_group_binding_upsert_outbox();


--
-- Name: TRIGGER trg_address_group_binding_upsert_outbox ON address_group_bindings; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER trg_address_group_binding_upsert_outbox ON public.address_group_bindings IS 'Migration 036: Unified outbox trigger for AddressGroupBinding.
Fires on every INSERT/UPDATE, creates outbox entry for internal processing.';


--
-- Name: address_groups trg_address_group_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_address_group_upsert_outbox AFTER INSERT OR UPDATE ON public.address_groups FOR EACH ROW EXECUTE FUNCTION public.trigger_address_group_upsert_outbox();


--
-- Name: TRIGGER trg_address_group_upsert_outbox ON address_groups; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER trg_address_group_upsert_outbox ON public.address_groups IS 'Migration 034: Unified outbox trigger for AddressGroup.
Fires on every INSERT/UPDATE, creates outbox entry for async SGROUP sync.';


--
-- Name: host_bindings trg_ag_update_on_binding_change; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_ag_update_on_binding_change AFTER INSERT OR DELETE OR UPDATE ON public.host_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_ag_update_on_binding_change();


--
-- Name: TRIGGER trg_ag_update_on_binding_change ON host_bindings; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER trg_ag_update_on_binding_change ON public.host_bindings IS 'Fires on binding lifecycle events (create/update/delete).
Creates Outbox entry for AG sync when both Host and AG are Ready.
Migration 033 - Restores functionality removed in Migration 026.';


--
-- Name: hosts trg_host_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_host_upsert_outbox AFTER INSERT OR UPDATE ON public.hosts FOR EACH ROW EXECUTE FUNCTION public.trigger_host_upsert_outbox();


--
-- Name: TRIGGER trg_host_upsert_outbox ON hosts; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER trg_host_upsert_outbox ON public.hosts IS 'Migration 034: Unified outbox trigger for Host.
Fires on every INSERT/UPDATE, creates outbox entry for async SGROUP sync.';


--
-- Name: networks trg_network_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_network_upsert_outbox AFTER INSERT OR UPDATE ON public.networks FOR EACH ROW EXECUTE FUNCTION public.trigger_network_upsert_outbox();


--
-- Name: TRIGGER trg_network_upsert_outbox ON networks; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER trg_network_upsert_outbox ON public.networks IS 'Migration 034: Unified outbox trigger for Network.
Fires on every INSERT/UPDATE, creates outbox entry for async SGROUP sync.';


--
-- Name: services trg_service_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_service_upsert_outbox AFTER INSERT OR UPDATE ON public.services FOR EACH ROW EXECUTE FUNCTION public.trigger_service_upsert_outbox();


--
-- Name: TRIGGER trg_service_upsert_outbox ON services; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER trg_service_upsert_outbox ON public.services IS 'Migration 034: Unified outbox trigger for Service.
Fires on every INSERT/UPDATE, creates outbox entry for async SGROUP sync.';


--
-- Name: k8s_metadata trg_sync_ready_from_conditions; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_sync_ready_from_conditions AFTER UPDATE OF conditions ON public.k8s_metadata FOR EACH ROW WHEN ((new.conditions IS DISTINCT FROM old.conditions)) EXECUTE FUNCTION public.sync_ready_from_conditions();


--
-- Name: TRIGGER trg_sync_ready_from_conditions ON k8s_metadata; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER trg_sync_ready_from_conditions ON public.k8s_metadata IS 'P0 BLOCKER WORKAROUND: Fires when k8s_metadata.conditions changes.
Auto-syncs hosts.ready/networks.ready from conditions (Ready=True check).
Enables migrations 026/027 triggers without backend code changes.
OPTIMIZATION: WHEN clause prevents trigger fire if conditions unchanged.
Migration 029.';


--
-- Name: address_groups unbind_hosts_on_address_group_deletion_trigger; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER unbind_hosts_on_address_group_deletion_trigger BEFORE DELETE ON public.address_groups FOR EACH ROW EXECUTE FUNCTION public.unbind_hosts_on_address_group_deletion();


--
-- Name: address_group_bindings update_aggregated_ags_on_binding_change_trigger; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER update_aggregated_ags_on_binding_change_trigger AFTER INSERT OR DELETE OR UPDATE ON public.address_group_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_update_aggregated_ags_on_binding_change();


--
-- Name: address_groups update_host_binding_status_on_spec_change_trigger; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER update_host_binding_status_on_spec_change_trigger AFTER UPDATE OF hosts ON public.address_groups FOR EACH ROW EXECUTE FUNCTION public.update_host_binding_status_on_spec_change();


--
-- Name: address_groups validate_address_group_hosts_trigger; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER validate_address_group_hosts_trigger BEFORE INSERT OR UPDATE OF hosts ON public.address_groups FOR EACH ROW EXECUTE FUNCTION public.validate_address_group_hosts();


--
-- Name: host_bindings validate_host_binding_conflicts_trigger; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER validate_host_binding_conflicts_trigger BEFORE INSERT OR UPDATE ON public.host_bindings FOR EACH ROW EXECUTE FUNCTION public.validate_host_binding_conflicts();


--
-- Name: address_group_bindings validate_service_ag_binding_conflicts_trigger; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER validate_service_ag_binding_conflicts_trigger BEFORE INSERT OR UPDATE ON public.address_group_bindings FOR EACH ROW EXECUTE FUNCTION public.validate_service_ag_binding_conflicts();


--
-- Name: services validate_service_spec_ag_conflicts_trigger; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER validate_service_spec_ag_conflicts_trigger BEFORE INSERT OR UPDATE OF address_groups ON public.services FOR EACH ROW EXECUTE FUNCTION public.validate_service_spec_ag_conflicts();


--
-- Name: address_group_binding_policies address_group_binding_policies_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.address_group_binding_policies
    ADD CONSTRAINT address_group_binding_policies_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- Name: address_group_bindings address_group_bindings_address_group_namespace_address_gro_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.address_group_bindings
    ADD CONSTRAINT address_group_bindings_address_group_namespace_address_gro_fkey FOREIGN KEY (address_group_namespace, address_group_name) REFERENCES public.address_groups(namespace, name) ON DELETE CASCADE;


--
-- Name: address_group_bindings address_group_bindings_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.address_group_bindings
    ADD CONSTRAINT address_group_bindings_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- Name: address_group_bindings address_group_bindings_service_namespace_service_name_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.address_group_bindings
    ADD CONSTRAINT address_group_bindings_service_namespace_service_name_fkey FOREIGN KEY (service_namespace, service_name) REFERENCES public.services(namespace, name) ON DELETE RESTRICT;


--
-- Name: address_group_port_mappings address_group_port_mappings_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.address_group_port_mappings
    ADD CONSTRAINT address_group_port_mappings_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- Name: address_groups address_groups_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.address_groups
    ADD CONSTRAINT address_groups_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- Name: hosts fk_hosts_binding_ref; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT fk_hosts_binding_ref FOREIGN KEY (binding_ref_namespace, binding_ref_name) REFERENCES public.host_bindings(namespace, name) ON DELETE SET NULL;


--
-- Name: host_bindings host_bindings_address_group_namespace_address_group_name_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.host_bindings
    ADD CONSTRAINT host_bindings_address_group_namespace_address_group_name_fkey FOREIGN KEY (address_group_namespace, address_group_name) REFERENCES public.address_groups(namespace, name) ON DELETE CASCADE;


--
-- Name: host_bindings host_bindings_host_namespace_host_name_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.host_bindings
    ADD CONSTRAINT host_bindings_host_namespace_host_name_fkey FOREIGN KEY (host_namespace, host_name) REFERENCES public.hosts(namespace, name) ON DELETE CASCADE;


--
-- Name: host_bindings host_bindings_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.host_bindings
    ADD CONSTRAINT host_bindings_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- Name: hosts hosts_address_group_ref_namespace_address_group_ref_name_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT hosts_address_group_ref_namespace_address_group_ref_name_fkey FOREIGN KEY (address_group_ref_namespace, address_group_ref_name) REFERENCES public.address_groups(namespace, name) ON DELETE SET NULL;


--
-- Name: hosts hosts_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT hosts_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- Name: ie_ag_ag_rules ie_ag_ag_rules_address_group_local_namespace_address_group_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.ie_ag_ag_rules
    ADD CONSTRAINT ie_ag_ag_rules_address_group_local_namespace_address_group_fkey FOREIGN KEY (address_group_local_namespace, address_group_local_name) REFERENCES public.address_groups(namespace, name) ON DELETE CASCADE;


--
-- Name: ie_ag_ag_rules ie_ag_ag_rules_address_group_namespace_address_group_name_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.ie_ag_ag_rules
    ADD CONSTRAINT ie_ag_ag_rules_address_group_namespace_address_group_name_fkey FOREIGN KEY (address_group_namespace, address_group_name) REFERENCES public.address_groups(namespace, name) ON DELETE CASCADE;


--
-- Name: ie_ag_ag_rules ie_ag_ag_rules_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.ie_ag_ag_rules
    ADD CONSTRAINT ie_ag_ag_rules_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- Name: network_bindings network_bindings_address_group_namespace_address_group_nam_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.network_bindings
    ADD CONSTRAINT network_bindings_address_group_namespace_address_group_nam_fkey FOREIGN KEY (address_group_namespace, address_group_name) REFERENCES public.address_groups(namespace, name) ON DELETE CASCADE;


--
-- Name: network_bindings network_bindings_network_namespace_network_name_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.network_bindings
    ADD CONSTRAINT network_bindings_network_namespace_network_name_fkey FOREIGN KEY (network_namespace, network_name) REFERENCES public.networks(namespace, name) ON DELETE CASCADE;


--
-- Name: network_bindings network_bindings_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.network_bindings
    ADD CONSTRAINT network_bindings_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- Name: networks networks_address_group_ref_namespace_address_group_ref_nam_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.networks
    ADD CONSTRAINT networks_address_group_ref_namespace_address_group_ref_nam_fkey FOREIGN KEY (address_group_ref_namespace, address_group_ref_name) REFERENCES public.address_groups(namespace, name) ON DELETE SET NULL;


--
-- Name: networks networks_binding_ref_namespace_binding_ref_name_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.networks
    ADD CONSTRAINT networks_binding_ref_namespace_binding_ref_name_fkey FOREIGN KEY (binding_ref_namespace, binding_ref_name) REFERENCES public.network_bindings(namespace, name) ON DELETE SET NULL;


--
-- Name: networks networks_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.networks
    ADD CONSTRAINT networks_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- Name: rule_s2s rule_s2s_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.rule_s2s
    ADD CONSTRAINT rule_s2s_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- Name: service_aliases service_aliases_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.service_aliases
    ADD CONSTRAINT service_aliases_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- Name: service_aliases service_aliases_service_namespace_service_name_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.service_aliases
    ADD CONSTRAINT service_aliases_service_namespace_service_name_fkey FOREIGN KEY (service_namespace, service_name) REFERENCES public.services(namespace, name) ON DELETE RESTRICT;


--
-- Name: services services_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.services
    ADD CONSTRAINT services_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

