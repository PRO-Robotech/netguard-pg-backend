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
-- Name: trigger_update_ag_on_host_ready(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_update_ag_on_host_ready() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    ag_currently_ready BOOLEAN;
    ag_namespace namespace_name;
    ag_name resource_name;
    ag_resource_version BIGINT;
    outbox_resource_id UUID;
BEGIN
    RAISE NOTICE '[026] Trigger fired for Host %.% (ready: % → %)',
        NEW.namespace, NEW.name, OLD.ready, NEW.ready;

    IF NOT (NEW.ready = TRUE AND OLD.ready = FALSE) THEN
        RAISE NOTICE '[026] WHEN clause not met (ready: % → %), skipping',
            OLD.ready, NEW.ready;
        RETURN NEW;
    END IF;

    ag_namespace := NEW.namespace;
    ag_name := NEW.address_group_ref_name;

    IF ag_name IS NULL OR ag_name = '' THEN
        RAISE NOTICE '[026] Host %.% not bound to any AG (address_group_ref_name IS NULL), skipping',
            NEW.namespace, NEW.name;
        RETURN NEW;
    END IF;

    RAISE NOTICE '[026] Host %.% is bound to AddressGroup %.%',
        NEW.namespace, NEW.name, ag_namespace, ag_name;

    RAISE NOTICE '[026] Updating aggregated_hosts for AG %.%...',
        ag_namespace, ag_name;

    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(ag_namespace::TEXT, ag_name::TEXT),
        updated_at = NOW()
    WHERE namespace = ag_namespace AND name = ag_name
    RETURNING resource_version INTO ag_resource_version;

    IF NOT FOUND THEN
        RAISE WARNING '[026] AddressGroup %.% not found in address_groups table! Host %.% references non-existent AG.',
            ag_namespace, ag_name, NEW.namespace, NEW.name;
        RETURN NEW;
    END IF;

    RAISE NOTICE '[026] ✅ Updated aggregated_hosts for AG %.% (resource_version: %)',
        ag_namespace, ag_name, ag_resource_version;

    RAISE NOTICE '[026] Checking if AG %.% is Ready...',
        ag_namespace, ag_name;

    SELECT EXISTS(
        SELECT 1 FROM k8s_metadata km
        WHERE km.resource_version = ag_resource_version
          AND km.conditions @> '[{"type":"Ready","status":"True"}]'::jsonb
    ) INTO ag_currently_ready;

    RAISE NOTICE '[026] AG %.% Ready status: %',
        ag_namespace, ag_name, ag_currently_ready;

    IF ag_currently_ready THEN
        RAISE NOTICE '[026] Creating Outbox entry for AG %.% sync...',
            ag_namespace, ag_name;

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
            ag_namespace,
            ag_name,
            'UPDATE'::sync_operation,
            'SGROUP'::target_system,
            jsonb_build_object(
                'namespace', ag_namespace,
                'name', ag_name,
                'trigger_reason', 'host_ready',
                'host', jsonb_build_object(
                    'namespace', NEW.namespace,
                    'name', NEW.name
                )
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
                'trigger_reason', 'host_ready',
                'host', EXCLUDED.payload->'host'
            );

        RAISE NOTICE '[026] ✅ Created/updated Outbox entry for AG %.% (resource_id: %)',
            ag_namespace, ag_name, outbox_resource_id;
    ELSE
        RAISE NOTICE '[026] ⏭️  AG %.% not Ready yet, skipping Outbox creation',
            ag_namespace, ag_name;
        RAISE NOTICE '[026] ℹ️  AG will sync when it becomes Ready (separate trigger)';
    END IF;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_update_ag_on_host_ready() OWNER TO netguard;

--
-- Name: FUNCTION trigger_update_ag_on_host_ready(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_update_ag_on_host_ready() IS 'Fires when Host.ready transitions FALSE→TRUE (Host becomes Ready).
Updates AG.aggregated_hosts immediately (preserves migration 014 behavior) and
creates sync_outbox entry for SGROUP sync (new Outbox Pattern).
Only creates Outbox if AG is currently Ready.
Migration 026 - BREAKING CHANGE from migration 014.
Migration 031 - FIXED to populate resource_namespace and resource_name.';


--
-- Name: trigger_update_ag_on_network_ready(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_update_ag_on_network_ready() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    ag_currently_ready BOOLEAN;
    ag_namespace namespace_name;
    ag_name resource_name;
    ag_resource_version BIGINT;
    ag_id UUID;
BEGIN
    RAISE NOTICE '[027] Trigger fired for Network %.% (ready: % → %)',
        NEW.namespace, NEW.name, OLD.ready, NEW.ready;

    IF NOT (NEW.ready = TRUE AND OLD.ready = FALSE) THEN
        RAISE NOTICE '[027] WHEN clause not met (ready: % → %), skipping',
            OLD.ready, NEW.ready;
        RETURN NEW;
    END IF;

    ag_namespace := NEW.address_group_ref_namespace;
    ag_name := NEW.address_group_ref_name;

    IF ag_namespace IS NULL OR ag_name IS NULL THEN
        RAISE NOTICE '[027] Network %.% not bound to any AG (address_group_ref_* IS NULL), skipping',
            NEW.namespace, NEW.name;
        RETURN NEW;
    END IF;

    RAISE NOTICE '[027] Network %.% is bound to AddressGroup %.% (1:1 link)',
        NEW.namespace, NEW.name, ag_namespace, ag_name;

    RAISE NOTICE '[027] Updating networks for AG %.%...',
        ag_namespace, ag_name;

    UPDATE address_groups
    SET networks = rebuild_address_group_networks(ag_namespace::TEXT, ag_name::TEXT),
        updated_at = NOW()
    WHERE namespace = ag_namespace AND name = ag_name
    RETURNING resource_version INTO ag_resource_version;

    IF NOT FOUND THEN
        RAISE WARNING '[027] AddressGroup %.% not found in address_groups table! Network %.% references non-existent AG.',
            ag_namespace, ag_name, NEW.namespace, NEW.name;
        RETURN NEW;
    END IF;

    RAISE NOTICE '[027] ✅ Updated networks for AG %.% (resource_version: %)',
        ag_namespace, ag_name, ag_resource_version;

    RAISE NOTICE '[027] Checking if AG %.% is Ready...',
        ag_namespace, ag_name;

    SELECT EXISTS(
        SELECT 1 FROM k8s_metadata km
        WHERE km.resource_version = ag_resource_version
          AND km.conditions @> '[{"type":"Ready","status":"True"}]'::jsonb
    ) INTO ag_currently_ready;

    RAISE NOTICE '[027] AG %.% Ready status: %',
        ag_namespace, ag_name, ag_currently_ready;

    IF ag_currently_ready THEN
        RAISE NOTICE '[027] Creating Outbox entry for AG %.% sync...',
            ag_namespace, ag_name;

        ag_id := uuid_generate_v5(
            uuid_ns_dns(),
            'AddressGroup:' || ag_namespace || '/' || ag_name
        );

        RAISE NOTICE '[027] Generated deterministic UUID for AG: %', ag_id;

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
            ag_id,
            ag_namespace,
            ag_name,
            'UPDATE'::sync_operation,
            'SGROUP'::target_system,
            jsonb_build_object(
                'namespace', ag_namespace,
                'name', ag_name,
                'trigger_reason', 'network_ready',
                'network', jsonb_build_object(
                    'namespace', NEW.namespace,
                    'name', NEW.name
                )
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
                'trigger_reason', 'network_ready',
                'network', EXCLUDED.payload->'network'
            );

        RAISE NOTICE '[027] ✅ Created/updated Outbox entry for AG %.% (resource_id: %)',
            ag_namespace, ag_name, ag_id;
    ELSE
        RAISE NOTICE '[027] ⏭️  AG %.% not Ready yet, skipping Outbox creation',
            ag_namespace, ag_name;
        RAISE NOTICE '[027] ℹ️  AG will sync when it becomes Ready (separate trigger)';
    END IF;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_update_ag_on_network_ready() OWNER TO netguard;

--
-- Name: FUNCTION trigger_update_ag_on_network_ready(); Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON FUNCTION public.trigger_update_ag_on_network_ready() IS 'Fires when Network.ready transitions FALSE→TRUE (Network becomes Ready).
Updates ONE AddressGroup (1:1 via networks.address_group_ref_*) and creates sync_outbox entry.
NO FOR loop needed - simple 1:1 relationship.
Migration 027 - BREAKING CHANGE from migration 020 (replaces TWO triggers with one).
Migration 031 - FIXED to populate resource_namespace and resource_name.';


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
\.


--
-- Data for Name: address_groups; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.address_groups (namespace, name, default_action, logs, trace, description, resource_version, networks, hosts, aggregated_hosts) FROM stdin;
incloud-sgroups	test-ag-new-001	ACCEPT	f	f		22	[]	[]	[]
default	example	DROP	t	t		29	[]	[]	[]
default	example-2	DROP	t	t		44	[]	[]	[]
default	example-4	DROP	t	t		45	[]	[]	[]
default	example-5	DROP	t	t		46	[]	[]	[]
incloud-sgroups	test-ag-031	ACCEPT	t	f		14	[]	[]	[]
default	example-10	DROP	t	t		63	[]	[]	[]
\.


--
-- Data for Name: host_bindings; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.host_bindings (namespace, name, host_namespace, host_name, address_group_namespace, address_group_name, resource_version) FROM stdin;
\.


--
-- Data for Name: hosts; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.hosts (namespace, name, uuid, host_name_sync, address_group_name, is_bound, binding_ref_namespace, binding_ref_name, address_group_ref_namespace, address_group_ref_name, resource_version, ip_list, ready) FROM stdin;
incloud-sgroups	to-nft-agent-42sgv	d09112e9-8031-4906-9219-31196d4c6b14	\N	\N	f	\N	\N	\N	\N	9	[{"ip": "fe80::fc43:42ff:fe1f:a4ea"}, {"ip": "127.0.0.1"}, {"ip": "10.244.0.49"}, {"ip": "::1"}]	t
incloud-sgroups	test-host-031	550e8400-e29b-41d4-a716-446655440031	\N	\N	f	\N	\N	\N	\N	16	\N	f
default	dynamic-199e788ea0e61d90	b0607910-0c16-4851-bba7-cd7807d1631e	\N	\N	f	\N	\N	\N	\N	62	\N	f
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
17	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	null	2025-10-14 15:20:01.189838+00	2025-10-14 15:20:01.189838+00	5a78bfc3-4ddf-44d6-b03c-7251b2e29407	1	\N
54	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T09:02:34Z"}]	2025-10-15 09:02:34.718696+00	2025-10-15 09:02:34.718696+00	42caf040-67a3-4e07-a768-494954946e47	1	\N
2	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-2qxp7\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"22a58fef-2022-45a3-bc0d-eaa1643ab694\\"}}\\n"}	{}	null	2025-10-10 15:31:06.627769+00	2025-10-10 15:31:06.627769+00	677a5c3d-ddb3-47fd-9f55-7e09398e228f	1	\N
3	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-2qxp7\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"22a58fef-2022-45a3-bc0d-eaa1643ab694\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-10T15:31:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-10T15:31:06Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-10T15:31:06Z"}]	2025-10-10 15:31:06.674209+00	2025-10-10 15:31:06.674209+00	4bde61d1-0219-4f60-9a96-42b83ce63ce9	1	2025-10-10 23:12:18.98962+00
4	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-kpk8w\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"49c2dce9-23de-45af-8090-55bb367765cc\\"}}\\n"}	{}	null	2025-10-10 23:13:05.973742+00	2025-10-10 23:13:05.973742+00	807d58c1-92dc-4dc1-93ee-52f65035880a	1	\N
5	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-kpk8w\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"49c2dce9-23de-45af-8090-55bb367765cc\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-10T23:13:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-10T23:13:06Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-10T23:13:06Z"}]	2025-10-10 23:13:06.003458+00	2025-10-10 23:13:06.003458+00	c564d93e-079a-4706-a602-45d9c3d27f4c	1	\N
18	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-14T15:20:01Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-14T15:20:01Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-14T15:20:01Z"}]	2025-10-14 15:20:01.19586+00	2025-10-14 15:20:01.19586+00	1a18825d-6276-4fa8-bc83-cb79139b3709	1	2025-10-14 15:22:51.409502+00
6	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-kpk8w\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"49c2dce9-23de-45af-8090-55bb367765cc\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-10T23:13:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-10T23:13:06Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-10T23:13:06Z"}]	2025-10-10 23:13:10.014263+00	2025-10-10 23:13:10.014263+00	e84c345c-1311-458a-a507-96cbe3c0c94f	1	2025-10-11 22:57:20.1527+00
7	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-42sgv\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"d09112e9-8031-4906-9219-31196d4c6b14\\"}}\\n"}	{}	null	2025-10-11 22:58:03.146276+00	2025-10-11 22:58:03.146276+00	88d84d6e-9afc-448f-a5a4-7becea73aa81	1	\N
8	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"to-nft-agent-42sgv\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"d09112e9-8031-4906-9219-31196d4c6b14\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-11T22:58:03Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-11T22:58:03Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-11T22:58:03Z"}]	2025-10-11 22:58:03.163429+00	2025-10-11 22:58:03.163429+00	f3115e4a-2817-412f-9f6e-c978e1d6c473	1	\N
1	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Address group is ready and operational", "lastTransitionTime": "2025-10-10T14:18:15Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Address group successfully synced to backend and SGROUP", "lastTransitionTime": "2025-10-10T14:18:15Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-10T14:18:15Z"}]	2025-10-10 14:18:15.395388+00	2025-10-10 14:18:17.424884+00	6ab7b126-450c-4553-b58d-8c6ec65dc93f	1	2025-10-14 16:47:30.760144+00
30	{}	{}	{}	null	2025-10-15 07:52:22.472744+00	2025-10-15 07:52:22.472744+00	31e3d8ac-2398-4b28-a7cd-3f3728926c59	1	\N
57	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T09:11:37Z"}]	2025-10-15 09:11:37.995309+00	2025-10-15 09:11:37.995309+00	c040465b-94be-49c1-9eea-b2a759797cc3	1	\N
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
26	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-resilient-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440010\\"}}\\n"}	{}	null	2025-10-14 16:49:20.845073+00	2025-10-14 16:49:20.845073+00	a4dcec85-7fdc-49f9-9d87-35703d488f57	1	\N
44	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T08:08:46Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T08:08:46Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T08:08:36Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-15T08:08:46Z"}]	2025-10-15 08:08:36.584648+00	2025-10-15 08:08:46.514543+00	90cdcdca-9424-4f9a-95a3-83b2447da100	1	\N
20	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-14T15:53:12Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-14T15:53:12Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-14T15:53:12Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-14T15:53:29Z"}]	2025-10-14 15:53:12.213535+00	2025-10-14 15:53:29.24533+00	802bb30a-1616-4bbb-93cd-b2123ff2c840	1	\N
34	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T07:53:11Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T07:53:11Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-15T07:53:11Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:53:14Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 07:53:11.963492+00	2025-10-15 07:53:14.495505+00	6f8a2b91-0290-4db8-8002-ed5cc8e2ec3f	1	2025-10-15 07:53:14.502672+00
35	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T07:53:14Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T07:53:11Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T07:53:14Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:53:34Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:53:34Z"}]	2025-10-15 07:53:14.502672+00	2025-10-15 07:53:34.495982+00	aec6bb45-2107-4a93-b664-b128ecfead5e	1	2025-10-15 07:53:24.510227+00
59	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:15:16Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:15:16Z"}]	2025-10-15 09:14:56.197641+00	2025-10-15 09:15:16.178932+00	156f73b0-8be2-4bb1-a736-597133ea843d	1	2025-10-15 09:15:06.201681+00
48	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T08:31:17Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:34:56Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 08:31:17.612431+00	2025-10-15 08:34:56.521732+00	b62aef20-bfd8-4778-866f-2a2295e535c2	1	2025-10-15 08:34:56.525071+00
60	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:15:16Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:15:16Z"}]	2025-10-15 09:14:56.210772+00	2025-10-15 09:15:16.187711+00	63cb7eff-01d7-4682-b59e-e84192221138	1	2025-10-15 09:15:06.21007+00
51	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Host committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T08:46:56Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T08:46:56Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Host is ready for use", "lastTransitionTime": "2025-10-15T08:46:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:47:06Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 08:46:56.591938+00	2025-10-15 08:47:06.503391+00	2e678ebf-36b5-4f4a-ac32-2c85d36aac7b	1	2025-10-15 08:47:06.512734+00
19	{"test": "cloud233", "scenario": "host-create-sync"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"scenario\\":\\"host-create-sync\\",\\"test\\":\\"cloud233\\"},\\"name\\":\\"test-host-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440001\\"}}\\n"}	{}	null	2025-10-14 15:53:12.204579+00	2025-10-14 15:53:12.204579+00	48962faa-c371-42ca-809c-0d965c37e31a	1	\N
58	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T09:11:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T09:11:38Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T09:11:38Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 09:11:38.000771+00	2025-10-15 09:14:56.218936+00	7059d37e-ac32-43f0-ad0b-6d47c3e77994	1	2025-10-15 09:14:56.22349+00
52	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T08:47:06Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-15T08:46:56Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T08:47:06Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:47:26Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:47:26Z"}]	2025-10-15 08:47:06.512734+00	2025-10-15 08:47:26.50385+00	6d2178a3-c310-48e3-ad0e-1a188019f714	1	2025-10-15 08:47:16.514998+00
61	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T09:11:38Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:15:16Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:15:16Z"}]	2025-10-15 09:14:56.22349+00	2025-10-15 09:15:16.193492+00	608fea93-f246-4b1e-a9f3-956ee01fdf84	1	2025-10-15 09:15:06.215907+00
63	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T11:02:54Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T11:02:54Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:04:48Z"}]	2025-10-15 11:02:54.181449+00	2025-10-15 11:04:48.025258+00	b417f972-a1ab-42e0-b998-4f92b9a183b2	1	\N
72	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T12:27:41Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T12:27:41Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T12:27:41Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:27:50Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 12:27:41.02136+00	2025-10-15 12:27:50.086604+00	c5494ccc-77fa-4c47-b233-3114a0655fce	1	2025-10-15 12:27:50.126809+00
38	{}	{}	{}	null	2025-10-15 07:53:54.479079+00	2025-10-15 07:53:54.479079+00	537b8d89-eb05-4a80-8fc5-4d62bd9b4ade	1	\N
66	{"app": "condition-manager-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"condition-manager-test\\"},\\"name\\":\\"test-network-ready-false\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.200.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T11:15:28Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T11:15:28Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T11:15:28Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:16:38Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 11:15:28.219745+00	2025-10-15 11:16:38.011506+00	3ce01e5c-a162-42b4-a8ae-48d3e9e37933	1	2025-10-15 11:16:38.034133+00
39	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 07:53:54.488311+00	2025-10-15 07:53:54.494917+00	cbd2d62d-1f43-4779-a349-81964fa74fa1	1	2025-10-15 07:53:54.502851+00
67	{"app": "condition-manager-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"condition-manager-test\\"},\\"name\\":\\"test-network-ready-false\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.200.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T11:16:38Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T11:16:38Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T11:15:28Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:16:58Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:16:58Z"}]	2025-10-15 11:16:38.034133+00	2025-10-15 11:16:58.015784+00	e79eab83-5a70-45b7-8d1f-8fd74831a9f6	1	2025-10-15 11:16:48.026069+00
46	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T08:34:56Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T08:34:56Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T08:09:54Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-15T08:34:56Z"}]	2025-10-15 08:09:54.226001+00	2025-10-15 08:34:56.516119+00	bf3d8bc0-92f8-415e-8e34-dc8ad0aa84be	1	\N
74	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T12:28:34Z"}]	2025-10-15 12:28:34.680018+00	2025-10-15 12:28:34.680018+00	9adede4d-7580-48db-98ee-0b161a932875	1	\N
75	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T12:28:34Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T12:28:34Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T12:28:34Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:28:40Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 12:28:34.683029+00	2025-10-15 12:28:40.090485+00	dd1c21ec-e09f-4bbc-b85d-7f8b1b9cb734	1	2025-10-15 12:28:40.112705+00
53	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T08:47:58Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T09:14:56Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 08:47:58.568646+00	2025-10-15 09:14:56.185216+00	7895a9e3-aab7-44aa-af92-004c79c87202	1	2025-10-15 09:14:56.197641+00
40	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T07:53:54Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:54:14Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T07:54:14Z"}]	2025-10-15 07:53:54.502851+00	2025-10-15 07:54:14.495494+00	8a9dd176-026b-4d0d-be0b-3c96e8048486	1	2025-10-15 07:54:04.504191+00
65	{"app": "condition-manager-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"condition-manager-test\\"},\\"name\\":\\"test-network-ready-false\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.200.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T11:15:28Z"}]	2025-10-15 11:15:28.195638+00	2025-10-15 11:15:28.195638+00	d54c4215-d45a-4d71-b3a0-ac05220ec80a	1	\N
68	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T11:34:39Z"}]	2025-10-15 11:34:39.333981+00	2025-10-15 11:34:39.333981+00	f18cfe23-469d-4e44-a8e5-95d839769721	1	\N
14	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-031\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T08:34:56Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T08:34:56Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-15T08:18:46Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-15T08:34:56Z"}]	2025-10-14 15:10:56.771655+00	2025-10-15 08:34:56.521365+00	48eabafa-2839-4145-a3f6-64c84b30269e	1	\N
69	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T11:34:39Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T11:34:39Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T11:34:39Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:34:48Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 11:34:39.344975+00	2025-10-15 11:34:48.016195+00	3e2d6695-d87c-467d-9de0-ca7d47de3b07	1	2025-10-15 11:34:48.028041+00
76	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T12:28:40Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T12:28:34Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T12:28:40Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:28:40Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 12:28:40.112705+00	2025-10-15 12:28:40.121173+00	1d4783c9-3634-40de-a4d9-cb4fe9e8a236	1	2025-10-15 12:28:40.189668+00
41	{}	{}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-15T08:07:47Z"}]	2025-10-15 08:07:47.142281+00	2025-10-15 08:07:47.142281+00	38d5de01-8fe8-4601-96cc-80e41696fe42	1	\N
42	{}	{}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Network is ready for use", "lastTransitionTime": "2025-10-15T08:07:47Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Network committed to backend and synced with sgroups successfully", "lastTransitionTime": "2025-10-15T08:07:47Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T08:07:47Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T08:07:56Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting SGROUP sync before deletion"}]	2025-10-15 08:07:47.152708+00	2025-10-15 08:07:56.495691+00	096db4bf-6720-4f4a-bde3-ece4c7636baa	1	2025-10-15 08:07:56.504148+00
70	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T11:34:48Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T11:34:48Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T11:34:39Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:35:08Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T11:35:08Z"}]	2025-10-15 11:34:48.028041+00	2025-10-15 11:35:08.022545+00	1d980c13-856a-447b-b704-ec344f688ecf	1	2025-10-15 11:34:58.022789+00
73	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T12:27:50Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T12:27:41Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T12:27:50Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:28:10Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:28:10Z"}]	2025-10-15 12:27:50.126809+00	2025-10-15 12:28:10.099412+00	1535759d-4adc-4efb-89e5-172e7e6f6b50	1	2025-10-15 12:28:00.098051+00
77	{"app": "cloud233-migration-test"}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"labels\\":{\\"app\\":\\"cloud233-migration-test\\"},\\"name\\":\\"test-network-cloud233-verification\\",\\"namespace\\":\\"default\\"},\\"spec\\":{\\"cidr\\":\\"192.168.233.0/24\\"}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-15T12:28:40Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-15T12:28:34Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-15T12:28:40Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:28:50Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-15T12:28:50Z"}]	2025-10-15 12:28:40.189668+00	2025-10-15 12:28:50.101918+00	3ffba11b-cb77-4080-8b89-5d94a9865073	1	2025-10-15 12:28:50.096443+00
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
\.


--
-- Data for Name: network_bindings; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.network_bindings (namespace, name, network_namespace, network_name, address_group_namespace, address_group_name, resource_version) FROM stdin;
\.


--
-- Data for Name: networks; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.networks (namespace, name, network_items, is_bound, binding_ref_namespace, binding_ref_name, address_group_ref_namespace, address_group_ref_name, resource_version, cidr, ready) FROM stdin;
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
\.


--
-- Data for Name: sync_outbox; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.sync_outbox (id, resource_type, resource_id, operation, target_system, payload, delta, affects_resources, status, attempts, max_retries, next_retry_at, last_error, error_category, created_at, updated_at, processed_at, resource_namespace, resource_name) FROM stdin;
21e14280-02f1-448c-994f-85ccbb23e168	Network	a6092315-657d-473f-add5-ae987d7dcb2e	CREATE	SGROUP	{"cidr": "192.168.233.0/24", "name": "test-network-cloud233-verification", "is_bound": false, "namespace": "default", "network_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-15 12:28:40.118954+00	2025-10-15 12:28:40.118954+00	\N	default	test-network-cloud233-verification
8e69f387-facf-47a1-a7ba-449bbd2afb28	Network	b41b6086-b605-4564-b03e-3581a899f591	CREATE	SGROUP	{"cidr": "192.168.233.0/24", "name": "test-network-cloud233-verification", "is_bound": false, "namespace": "default", "network_name": ""}	\N	\N	PENDING	0	5	\N	\N	\N	2025-10-15 12:28:40.192124+00	2025-10-15 12:28:40.192124+00	\N	default	test-network-cloud233-verification
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

SELECT pg_catalog.setval('public.k8s_metadata_resource_version_seq', 77, true);


--
-- Name: netguard_db_ver_id_seq; Type: SEQUENCE SET; Schema: public; Owner: netguard
--

SELECT pg_catalog.setval('public.netguard_db_ver_id_seq', 35, true);


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
-- Name: networks network_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER network_before_delete BEFORE DELETE ON public.networks FOR EACH ROW EXECUTE FUNCTION public.trigger_network_before_delete();


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
-- Name: hosts trg_update_ag_on_host_ready; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_update_ag_on_host_ready AFTER UPDATE ON public.hosts FOR EACH ROW WHEN (((new.ready = true) AND (old.ready = false))) EXECUTE FUNCTION public.trigger_update_ag_on_host_ready();


--
-- Name: TRIGGER trg_update_ag_on_host_ready ON hosts; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER trg_update_ag_on_host_ready ON public.hosts IS 'Fires ONLY when Host becomes Ready (ready: FALSE→TRUE transition).
Updates AG.aggregated_hosts and creates Outbox entry for SGROUP sync.
BREAKING CHANGE from migration 014: Replaces host_bindings trigger with conditional Host trigger.
Migration 026.';


--
-- Name: networks trg_update_ag_on_network_ready; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_update_ag_on_network_ready AFTER UPDATE ON public.networks FOR EACH ROW WHEN (((new.ready = true) AND (old.ready = false))) EXECUTE FUNCTION public.trigger_update_ag_on_network_ready();


--
-- Name: TRIGGER trg_update_ag_on_network_ready ON networks; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON TRIGGER trg_update_ag_on_network_ready ON public.networks IS 'Fires ONLY when Network becomes Ready (ready: FALSE→TRUE transition).
Updates ONE AG.networks (1:1 link) and creates Outbox entry for SGROUP sync.
BREAKING CHANGE from migration 020: Replaces TWO triggers with one conditional Network trigger.
Migration 027.';


--
-- Name: address_groups unbind_hosts_on_address_group_deletion_trigger; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER unbind_hosts_on_address_group_deletion_trigger BEFORE DELETE ON public.address_groups FOR EACH ROW EXECUTE FUNCTION public.unbind_hosts_on_address_group_deletion();


--
-- Name: address_group_bindings update_aggregated_ags_on_binding_change_trigger; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER update_aggregated_ags_on_binding_change_trigger AFTER INSERT OR DELETE OR UPDATE ON public.address_group_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_update_aggregated_ags_on_binding_change();


--
-- Name: services update_aggregated_ags_on_spec_change; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER update_aggregated_ags_on_spec_change AFTER INSERT OR UPDATE OF address_groups ON public.services FOR EACH ROW EXECUTE FUNCTION public.trigger_update_aggregated_ags_on_spec_change();


--
-- Name: address_groups update_aggregated_hosts_on_spec_change; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER update_aggregated_hosts_on_spec_change AFTER INSERT OR UPDATE OF hosts ON public.address_groups FOR EACH ROW EXECUTE FUNCTION public.trigger_update_aggregated_hosts_on_spec_change();


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

