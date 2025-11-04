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
-- Name: SCHEMA public; Type: COMMENT; Schema: -; Owner: netguard
--

COMMENT ON SCHEMA public IS '';


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
-- Name: target_system; Type: TYPE; Schema: public; Owner: netguard
--

CREATE TYPE public.target_system AS ENUM (
    'SGROUP',
    'INTERNAL'
);


ALTER TYPE public.target_system OWNER TO netguard;

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
-- Name: sync_address_group_networks_on_binding_change(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.sync_address_group_networks_on_binding_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    ag_namespace TEXT;
    ag_name TEXT;
    new_networks JSONB;
BEGIN
    IF TG_OP = 'DELETE' THEN
        ag_namespace := OLD.address_group_namespace;
        ag_name := OLD.address_group_name;
    ELSE
        ag_namespace := NEW.address_group_namespace;
        ag_name := NEW.address_group_name;
    END IF;

    new_networks := rebuild_address_group_networks(ag_namespace, ag_name);

    UPDATE address_groups
    SET networks = new_networks
    WHERE namespace = ag_namespace
      AND name = ag_name;

    -- If this was an UPDATE that changed the AddressGroup reference, also update the old AddressGroup
    IF TG_OP = 'UPDATE' AND (OLD.address_group_namespace != NEW.address_group_namespace OR OLD.address_group_name != NEW.address_group_name) THEN
        new_networks := rebuild_address_group_networks(OLD.address_group_namespace, OLD.address_group_name);
        UPDATE address_groups
        SET networks = new_networks
        WHERE namespace = OLD.address_group_namespace
          AND name = OLD.address_group_name;
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$;


ALTER FUNCTION public.sync_address_group_networks_on_binding_change() OWNER TO netguard;

--
-- Name: sync_address_group_networks_on_network_change(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.sync_address_group_networks_on_network_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    ag_record RECORD;
    new_networks JSONB;
BEGIN
    FOR ag_record IN
        SELECT DISTINCT nb.address_group_namespace, nb.address_group_name
        FROM network_bindings nb
        WHERE nb.network_namespace = COALESCE(NEW.namespace, OLD.namespace)
          AND nb.network_name = COALESCE(NEW.name, OLD.name)
    LOOP
        new_networks := rebuild_address_group_networks(ag_record.address_group_namespace, ag_record.address_group_name);

        UPDATE address_groups
        SET networks = new_networks
        WHERE namespace = ag_record.address_group_namespace
          AND name = ag_record.address_group_name;
    END LOOP;

    RETURN COALESCE(NEW, OLD);
END;
$$;


ALTER FUNCTION public.sync_address_group_networks_on_network_change() OWNER TO netguard;

--
-- Name: sync_service_rule_refs(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.sync_service_rule_refs() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_service_from_key TEXT;
    v_service_to_key TEXT;
    v_from_namespace TEXT;
    v_from_name TEXT;
    v_to_namespace TEXT;
    v_to_name TEXT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_from_namespace := NEW.service_from_ref->>'namespace';
        v_from_name := NEW.service_from_ref->>'name';
        v_service_from_key := v_from_namespace || '/' || v_from_name;

        v_to_namespace := NEW.service_to_ref->>'namespace';
        v_to_name := NEW.service_to_ref->>'name';
        v_service_to_key := v_to_namespace || '/' || v_to_name;

        INSERT INTO service_rule_refs (service_ref, rule_id, role)
        VALUES
            (v_service_from_key, NEW.id, 'SERVICE_FROM'),
            (v_service_to_key, NEW.id, 'SERVICE_TO');

        RETURN NEW;

    ELSIF TG_OP = 'UPDATE' THEN
        v_from_namespace := NEW.service_from_ref->>'namespace';
        v_from_name := NEW.service_from_ref->>'name';
        v_service_from_key := v_from_namespace || '/' || v_from_name;

        v_to_namespace := NEW.service_to_ref->>'namespace';
        v_to_name := NEW.service_to_ref->>'name';
        v_service_to_key := v_to_namespace || '/' || v_to_name;

        IF OLD.service_from_ref::text != NEW.service_from_ref::text OR
           OLD.service_to_ref::text != NEW.service_to_ref::text THEN
            RAISE EXCEPTION 'serviceFrom and serviceTo are immutable fields (validation #10, #11)';
        END IF;

        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$;


ALTER FUNCTION public.sync_service_rule_refs() OWNER TO netguard;

--
-- Name: trigger_address_group_before_delete(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_address_group_before_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
BEGIN
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

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
        v_uid,
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

    RETURN NULL;
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
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
BEGIN
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting internal processing before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

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
        v_uid,
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
            )
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RETURN NULL;
END;
$$;


ALTER FUNCTION public.trigger_address_group_binding_before_delete() OWNER TO netguard;

--
-- Name: trigger_address_group_binding_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_address_group_binding_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    -- Get metadata UID for this AddressGroupBinding
    SELECT m.uid INTO v_resource_id
    FROM k8s_metadata m
    WHERE m.resource_version = NEW.resource_version;

    IF v_resource_id IS NULL THEN
        RAISE EXCEPTION 'AddressGroupBinding metadata not found for resource_version=%', NEW.resource_version;
    END IF;

    -- Determine operation type
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
    END IF;

    -- Insert or update Outbox entry
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
        v_resource_id,
        v_operation_type,
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
            )
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

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_address_group_binding_upsert_outbox() OWNER TO netguard;

--
-- Name: trigger_address_group_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_address_group_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
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
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name
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

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_address_group_upsert_outbox() OWNER TO netguard;

--
-- Name: trigger_ag_update_on_binding_change(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_ag_update_on_binding_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_host_ready BOOLEAN;
    v_ag_ready BOOLEAN;
    v_ag_resource_version BIGINT;
    v_outbox_resource_id UUID;
    v_namespace TEXT;
    v_name TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v_namespace := OLD.address_group_namespace;
        v_name := OLD.address_group_name;
    ELSE
        v_namespace := NEW.address_group_namespace;
        v_name := NEW.address_group_name;
    END IF;

    IF TG_OP = 'DELETE' THEN
        SELECT EXISTS(
            SELECT 1 FROM hosts h
            JOIN k8s_metadata km ON h.resource_version = km.resource_version
            WHERE h.namespace = OLD.host_namespace
              AND h.name = OLD.host_name
              AND km.conditions @> '[{"type":"Ready","status":"True"}]'::jsonb
        ) INTO v_host_ready;
    ELSE
        SELECT EXISTS(
            SELECT 1 FROM hosts h
            JOIN k8s_metadata km ON h.resource_version = km.resource_version
            WHERE h.namespace = NEW.host_namespace
              AND h.name = NEW.host_name
              AND km.conditions @> '[{"type":"Ready","status":"True"}]'::jsonb
        ) INTO v_host_ready;
    END IF;

    IF v_host_ready IS NULL OR NOT v_host_ready THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        ELSE
            RETURN NEW;
        END IF;
    END IF;

    SELECT ag.resource_version INTO v_ag_resource_version
    FROM address_groups ag
    WHERE ag.namespace = v_namespace
      AND ag.name = v_name;

    IF v_ag_resource_version IS NULL THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        ELSE
            RETURN NEW;
        END IF;
    END IF;

    SELECT EXISTS(
        SELECT 1 FROM k8s_metadata km
        WHERE km.resource_version = v_ag_resource_version
          AND km.conditions @> '[{"type":"Ready","status":"True"}]'::jsonb
    ) INTO v_ag_ready;

    IF NOT v_ag_ready THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        ELSE
            RETURN NEW;
        END IF;
    END IF;

    v_outbox_resource_id := gen_random_uuid();

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
            v_outbox_resource_id,
            OLD.address_group_namespace,
            OLD.address_group_name,
            'UPDATE'::sync_operation,
            'SGROUP'::target_system,
            jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name,
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
            payload = EXCLUDED.payload;
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
            v_outbox_resource_id,
            NEW.address_group_namespace,
            NEW.address_group_name,
            'UPDATE'::sync_operation,
            'SGROUP'::target_system,
            jsonb_build_object(
                'namespace', NEW.address_group_namespace,
                'name', NEW.address_group_name,
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
            payload = EXCLUDED.payload;
    END IF;

    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(v_namespace::TEXT, v_name::TEXT)
    WHERE namespace = v_namespace AND name = v_name;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$;


ALTER FUNCTION public.trigger_ag_update_on_binding_change() OWNER TO netguard;

--
-- Name: trigger_host_before_delete(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_host_before_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
BEGIN
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

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
        v_uid,
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
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RETURN NULL;
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
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
    v_affected_resources JSONB;
BEGIN
    -- Get UID and check if already marked for deletion
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    -- Build affected_resources array: [Host, AddressGroup]
    -- IMPORTANT: HostBinding does NOT involve Network!
    v_affected_resources := jsonb_build_array(
        jsonb_build_object(
            'type', 'Host',
            'namespace', OLD.host_namespace,
            'name', OLD.host_name
        ),
        jsonb_build_object(
            'type', 'AddressGroup',
            'namespace', OLD.address_group_namespace,
            'name', OLD.address_group_name
        )
    );

    -- Mark metadata for deletion
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting internal processing before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    -- Create DELETE Outbox entry with affects_resources
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        affects_resources,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'HostBinding',
        v_uid,
        'DELETE'::sync_operation,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'host', jsonb_build_object(
                'namespace', OLD.host_namespace,
                'name', OLD.host_name
            ),
            'addressGroup', jsonb_build_object(
                'namespace', OLD.address_group_namespace,
                'name', OLD.address_group_name
            )
        ),
        OLD.namespace,
        OLD.name,
        v_affected_resources,
        'PENDING'::outbox_status,
        0,
        5,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO UPDATE SET
        payload = EXCLUDED.payload,
        resource_namespace = EXCLUDED.resource_namespace,
        resource_name = EXCLUDED.resource_name,
        affects_resources = EXCLUDED.affects_resources,
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = CURRENT_TIMESTAMP,
        updated_at = CURRENT_TIMESTAMP;

    RAISE NOTICE 'Created DELETE outbox entry for HostBinding %/%', OLD.namespace, OLD.name;

    RETURN NULL;
END;
$$;


ALTER FUNCTION public.trigger_host_binding_before_delete() OWNER TO netguard;

--
-- Name: trigger_host_binding_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_host_binding_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_affected_resources JSONB;
BEGIN
    -- Get metadata UID for this HostBinding
    SELECT m.uid INTO v_resource_id
    FROM k8s_metadata m
    WHERE m.resource_version = NEW.resource_version;

    IF v_resource_id IS NULL THEN
        RAISE EXCEPTION 'HostBinding metadata not found for resource_version=%', NEW.resource_version;
    END IF;

    -- Determine operation type
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
    END IF;

    -- Build affected_resources array: [Host, AddressGroup]
    -- IMPORTANT: HostBinding does NOT involve Network!
    v_affected_resources := jsonb_build_array(
        jsonb_build_object(
            'type', 'Host',
            'namespace', NEW.host_namespace,
            'name', NEW.host_name
        ),
        jsonb_build_object(
            'type', 'AddressGroup',
            'namespace', NEW.address_group_namespace,
            'name', NEW.address_group_name
        )
    );

    -- Insert or update Outbox entry
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        affects_resources,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'HostBinding',
        v_resource_id,
        v_operation_type,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'host', jsonb_build_object(
                'namespace', NEW.host_namespace,
                'name', NEW.host_name
            ),
            'addressGroup', jsonb_build_object(
                'namespace', NEW.address_group_namespace,
                'name', NEW.address_group_name
            )
        ),
        NEW.namespace,
        NEW.name,
        v_affected_resources,
        'PENDING'::outbox_status,
        0,
        5,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP
    )
    -- FIX: Add target_system to match UNIQUE constraint
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO UPDATE SET
        payload = EXCLUDED.payload,
        resource_namespace = EXCLUDED.resource_namespace,
        resource_name = EXCLUDED.resource_name,
        affects_resources = EXCLUDED.affects_resources,
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = CURRENT_TIMESTAMP,
        updated_at = CURRENT_TIMESTAMP;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_host_binding_upsert_outbox() OWNER TO netguard;

--
-- Name: trigger_host_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_host_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
    END IF;

    v_resource_id := NEW.uuid;

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
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'uuid', NEW.uuid::text
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

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_host_upsert_outbox() OWNER TO netguard;

--
-- Name: trigger_network_before_delete(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_network_before_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
BEGIN
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

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
        v_uid,
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

    RETURN NULL;
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
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
    v_affected_resources JSONB;
BEGIN
    -- Get UID and check if already marked for deletion
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    -- Build affected_resources array BEFORE DELETE
    -- CRITICAL: Include Network and AddressGroup that this binding connects
    v_affected_resources := jsonb_build_array(
        jsonb_build_object(
            'type', 'Network',
            'namespace', OLD.network_namespace,
            'name', OLD.network_name
        ),
        jsonb_build_object(
            'type', 'AddressGroup',
            'namespace', OLD.address_group_namespace,
            'name', OLD.address_group_name
        )
    );

    -- Mark metadata for deletion
    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting internal processing before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

    -- Create DELETE Outbox entry with affects_resources
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        resource_namespace,
        resource_name,
        operation,
        target_system,
        payload,
        affects_resources,  -- ADDED: affects_resources field
        status,
        attempts,
        max_retries,
        created_at,
        updated_at,
        next_retry_at
    )
    VALUES (
        'NetworkBinding',
        v_uid,
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
            )
        ),
        v_affected_resources,  -- ADDED: populated with Network and AddressGroup
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    );

    RETURN OLD;
END;
$$;


ALTER FUNCTION public.trigger_network_binding_before_delete() OWNER TO netguard;

--
-- Name: trigger_network_binding_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_network_binding_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
    v_affected_resources JSONB;
BEGIN
    -- Get metadata UID for this NetworkBinding
    SELECT m.uid INTO v_resource_id
    FROM k8s_metadata m
    WHERE m.resource_version = NEW.resource_version;

    IF v_resource_id IS NULL THEN
        RAISE EXCEPTION 'NetworkBinding metadata not found for resource_version=%', NEW.resource_version;
    END IF;

    -- Determine operation type
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
    END IF;

    -- Build affected_resources array: [Network, AddressGroup]
    v_affected_resources := jsonb_build_array(
        jsonb_build_object(
            'type', 'Network',
            'namespace', NEW.network_namespace,
            'name', NEW.network_name
        ),
        jsonb_build_object(
            'type', 'AddressGroup',
            'namespace', NEW.address_group_namespace,
            'name', NEW.address_group_name
        )
    );

    -- Insert or update Outbox entry
    INSERT INTO sync_outbox (
        resource_type,
        resource_id,
        operation,
        target_system,
        payload,
        resource_namespace,
        resource_name,
        affects_resources,
        status,
        attempts,
        max_retries,
        next_retry_at,
        created_at,
        updated_at
    )
    VALUES (
        'NetworkBinding',
        v_resource_id,
        v_operation_type,
        'INTERNAL'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'network', jsonb_build_object(
                'namespace', NEW.network_namespace,
                'name', NEW.network_name
            ),
            'address_group', jsonb_build_object(
                'namespace', NEW.address_group_namespace,
                'name', NEW.address_group_name
            )
        ),
        NEW.namespace,
        NEW.name,
        v_affected_resources,
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system)
    DO UPDATE SET
        payload = EXCLUDED.payload,
        affects_resources = EXCLUDED.affects_resources,
        status = 'PENDING'::outbox_status,
        attempts = 0,
        next_retry_at = NOW(),
        updated_at = NOW();

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_network_binding_upsert_outbox() OWNER TO netguard;

--
-- Name: trigger_network_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_network_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
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
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'cidr', NEW.cidr::text
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

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_network_upsert_outbox() OWNER TO netguard;

--
-- Name: trigger_service_before_delete(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_service_before_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
BEGIN
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

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
        v_uid,
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

    RETURN NULL;
END;
$$;


ALTER FUNCTION public.trigger_service_before_delete() OWNER TO netguard;

--
-- Name: trigger_service_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_service_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
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
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name
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

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_service_upsert_outbox() OWNER TO netguard;

--
-- Name: trigger_svcsvc_rule_before_delete(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_svcsvc_rule_before_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_uid UUID;
    v_already_marked_for_deletion BOOLEAN;
BEGIN
    SELECT m.uid, m.deletion_timestamp IS NOT NULL
    INTO v_uid, v_already_marked_for_deletion
    FROM k8s_metadata m
    WHERE m.resource_version = OLD.resource_version;

    IF v_already_marked_for_deletion THEN
        RETURN OLD;
    END IF;

    UPDATE k8s_metadata
    SET deletion_timestamp = NOW(),
        conditions = COALESCE(conditions, '[]'::jsonb) ||
            '[{"type":"PendingSync","status":"True","reason":"PendingDeletion","message":"Awaiting SGROUP sync before deletion"}]'::jsonb
    WHERE resource_version = OLD.resource_version;

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
        'SvcSvcRule',
        v_uid,
        OLD.namespace,
        OLD.name,
        'DELETE'::sync_operation,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', OLD.namespace,
            'name', OLD.name,
            'service_from_ref', OLD.service_from_ref,
            'service_to_ref', OLD.service_to_ref
        ),
        'PENDING'::outbox_status,
        0,
        5,
        NOW(),
        NOW(),
        NOW()
    )
    ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING;

    RETURN NULL;
END;
$$;


ALTER FUNCTION public.trigger_svcsvc_rule_before_delete() OWNER TO netguard;

--
-- Name: trigger_svcsvc_rule_upsert_outbox(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_svcsvc_rule_upsert_outbox() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_resource_id UUID;
    v_operation_type sync_operation;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_operation_type := 'CREATE'::sync_operation;
    ELSE
        v_operation_type := 'UPDATE'::sync_operation;
    END IF;

    v_resource_id := uuid_generate_v5(
        uuid_ns_dns(),
        'SvcSvcRule:' || NEW.namespace || '/' || NEW.name
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
        'SvcSvcRule',
        v_resource_id,
        v_operation_type,
        'SGROUP'::target_system,
        jsonb_build_object(
            'namespace', NEW.namespace,
            'name', NEW.name,
            'service_from', NEW.service_from_ref,
            'service_to', NEW.service_to_ref,
            'action', NEW.action,
            'priority', NEW.priority,
            'logs', NEW.logs,
            'trace', NEW.trace
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
        payload = EXCLUDED.payload;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_svcsvc_rule_upsert_outbox() OWNER TO netguard;

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
-- Name: trigger_update_aggregated_hosts_on_binding_change(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_update_aggregated_hosts_on_binding_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    -- Update aggregated_hosts for the affected AddressGroup(s)
    IF TG_OP = 'DELETE' THEN
        UPDATE address_groups 
        SET aggregated_hosts = aggregate_address_group_hosts(OLD.address_group_namespace, OLD.address_group_name)
        WHERE namespace = OLD.address_group_namespace AND name = OLD.address_group_name;
        RETURN OLD;
    ELSE
        UPDATE address_groups 
        SET aggregated_hosts = aggregate_address_group_hosts(NEW.address_group_namespace, NEW.address_group_name)
        WHERE namespace = NEW.address_group_namespace AND name = NEW.address_group_name;
        
        -- If UPDATE changed the AddressGroup reference, also update the old one
        IF TG_OP = 'UPDATE' AND (OLD.address_group_namespace != NEW.address_group_namespace OR OLD.address_group_name != NEW.address_group_name) THEN
            UPDATE address_groups 
            SET aggregated_hosts = aggregate_address_group_hosts(OLD.address_group_namespace, OLD.address_group_name)
            WHERE namespace = OLD.address_group_namespace AND name = OLD.address_group_name;
        END IF;
        
        RETURN NEW;
    END IF;
END;
$$;


ALTER FUNCTION public.trigger_update_aggregated_hosts_on_binding_change() OWNER TO netguard;

--
-- Name: trigger_update_aggregated_hosts_on_spec_change(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.trigger_update_aggregated_hosts_on_spec_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    -- Update aggregated_hosts with separate UPDATE statement after the main operation
    UPDATE address_groups
    SET aggregated_hosts = aggregate_address_group_hosts(NEW.namespace, NEW.name)
    WHERE namespace = NEW.namespace AND name = NEW.name;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_update_aggregated_hosts_on_spec_change() OWNER TO netguard;

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
-- Name: update_service_xsvcsvc_rules(); Type: FUNCTION; Schema: public; Owner: netguard
--

CREATE FUNCTION public.update_service_xsvcsvc_rules() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_rule_ref JSONB;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'SvcSvcRule',
            'name', svc_svc_rules.name,
            'namespace', svc_svc_rules.namespace
        ) INTO v_rule_ref
        FROM svc_svc_rules
        WHERE id = NEW.rule_id;

        IF NEW.role = 'SERVICE_FROM' THEN
            UPDATE services
            SET xsvcsvc_rules_as_from = xsvcsvc_rules_as_from || jsonb_build_array(v_rule_ref)
            WHERE services.namespace || '/' || services.name = NEW.service_ref;

        ELSIF NEW.role = 'SERVICE_TO' THEN
            UPDATE services
            SET xsvcsvc_rules_as_to = xsvcsvc_rules_as_to || jsonb_build_array(v_rule_ref)
            WHERE services.namespace || '/' || services.name = NEW.service_ref;
        END IF;

        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        SELECT jsonb_build_object(
            'apiVersion', 'netguard.sgroups.io/v1beta1',
            'kind', 'SvcSvcRule',
            'name', svc_svc_rules.name,
            'namespace', svc_svc_rules.namespace
        ) INTO v_rule_ref
        FROM svc_svc_rules
        WHERE id = OLD.rule_id;

        IF OLD.role = 'SERVICE_FROM' THEN
            UPDATE services
            SET xsvcsvc_rules_as_from = (
                SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
                FROM jsonb_array_elements(xsvcsvc_rules_as_from) AS elem
                WHERE elem != v_rule_ref
            )
            WHERE services.namespace || '/' || services.name = OLD.service_ref;

        ELSIF OLD.role = 'SERVICE_TO' THEN
            UPDATE services
            SET xsvcsvc_rules_as_to = (
                SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
                FROM jsonb_array_elements(xsvcsvc_rules_as_to) AS elem
                WHERE elem != v_rule_ref
            )
            WHERE services.namespace || '/' || services.name = OLD.service_ref;
        END IF;

        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$;


ALTER FUNCTION public.update_service_xsvcsvc_rules() OWNER TO netguard;

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
    CONSTRAINT check_ip_list_is_array CHECK (((ip_list IS NULL) OR (jsonb_typeof(ip_list) = 'array'::text)))
);


ALTER TABLE public.hosts OWNER TO netguard;

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
    cidr cidr NOT NULL
);


ALTER TABLE public.networks OWNER TO netguard;

--
-- Name: COLUMN networks.cidr; Type: COMMENT; Schema: public; Owner: netguard
--

COMMENT ON COLUMN public.networks.cidr IS 'Network CIDR block - must be unique across all networks';


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
-- Name: service_rule_refs; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.service_rule_refs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    service_ref character varying(510) NOT NULL,
    rule_id uuid NOT NULL,
    role character varying(50) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT service_rule_refs_role_check CHECK (((role)::text = ANY ((ARRAY['SERVICE_FROM'::character varying, 'SERVICE_TO'::character varying])::text[])))
);


ALTER TABLE public.service_rule_refs OWNER TO netguard;

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
    aggregated_address_groups jsonb DEFAULT '[]'::jsonb NOT NULL,
    xsvcsvc_rules_as_from jsonb DEFAULT '[]'::jsonb NOT NULL,
    xsvcsvc_rules_as_to jsonb DEFAULT '[]'::jsonb NOT NULL
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
-- Name: svc_svc_rules; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.svc_svc_rules (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    namespace character varying(253) NOT NULL,
    name character varying(253) NOT NULL,
    service_from_ref jsonb NOT NULL,
    service_to_ref jsonb NOT NULL,
    action character varying(50) NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    logs boolean DEFAULT false NOT NULL,
    trace boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    resource_version bigint,
    CONSTRAINT svc_svc_rules_action_check CHECK (((action)::text = ANY ((ARRAY['ACCEPT'::character varying, 'DROP'::character varying])::text[]))),
    CONSTRAINT svc_svc_rules_priority_check CHECK (((priority >= 0) AND (priority <= 1000)))
);


ALTER TABLE public.svc_svc_rules OWNER TO netguard;

--
-- Name: sync_outbox; Type: TABLE; Schema: public; Owner: netguard
--

CREATE TABLE public.sync_outbox (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    resource_type character varying(50) NOT NULL,
    resource_id uuid NOT NULL,
    resource_namespace character varying(253) NOT NULL,
    resource_name character varying(253) NOT NULL,
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
    processed_at timestamp with time zone
);


ALTER TABLE public.sync_outbox OWNER TO netguard;

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
incloud-sgroups	test-agb-final	incloud-sgroups	test-service-agb001	incloud-sgroups	test-ag-agb001	34
\.


--
-- Data for Name: address_group_port_mappings; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.address_group_port_mappings (namespace, name, access_ports, resource_version) FROM stdin;
incloud-sgroups	example-6	{}	26
incloud-sgroups	test-ag-agb001	{"incloud-sgroups/test-service-agb001": {"Ports": {"TCP": [{"End": 8080, "Start": 8080}]}}}	31
\.


--
-- Data for Name: address_groups; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.address_groups (namespace, name, default_action, logs, trace, description, resource_version, networks, hosts, aggregated_hosts) FROM stdin;
incloud-sgroups	test-ag-clean	ACCEPT	f	f		3	[]	[]	[]
incloud-sgroups	test-ag-hb001	ACCEPT	f	f		13	[]	[]	[{"ref": {"kind": "Host", "name": "test-host-hb001", "namespace": "incloud-sgroups", "apiVersion": "netguard.sgroups.io/v1beta1"}, "uuid": "550e8400-e29b-41d4-a716-446655440999", "source": "binding"}]
incloud-sgroups	example-6	DROP	t	t		23	[]	[]	[]
incloud-sgroups	test-ag-agb001	ACCEPT	f	f		28	[]	[]	[]
\.


--
-- Data for Name: host_bindings; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.host_bindings (namespace, name, host_namespace, host_name, address_group_namespace, address_group_name, resource_version) FROM stdin;
incloud-sgroups	test-hostbinding-hb001	incloud-sgroups	test-host-hb001	incloud-sgroups	test-ag-hb001	19
\.


--
-- Data for Name: hosts; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.hosts (namespace, name, uuid, host_name_sync, address_group_name, is_bound, binding_ref_namespace, binding_ref_name, address_group_ref_namespace, address_group_ref_name, resource_version, ip_list) FROM stdin;
incloud-sgroups	test-host-hb001	550e8400-e29b-41d4-a716-446655440999	\N	\N	f	\N	\N	\N	\N	22	\N
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
27	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-service-agb001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"ingressPorts\\":[{\\"description\\":\\"HTTP port\\",\\"port\\":\\"8080\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T16:45:05Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T16:45:05Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-26T16:45:00Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-26T16:47:35Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-26T16:47:35Z"}]	2025-10-26 16:45:00.45131+00	2025-10-26 16:47:35.372729+00	40e3dc38-56f3-4ea9-a4c0-312582c6fe07	1	2025-10-26 16:47:32.375908+00
22	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-host-hb001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440999\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T14:26:19Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-26T14:26:16Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T14:26:19Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T14:26:19Z"}]	2025-10-26 14:26:16.309354+00	2025-10-26 14:26:19.632756+00	fe68970f-3a5b-4ab6-80b8-1f4c592ddf87	1	\N
1	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-network-clean\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.200.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T12:22:41Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-26T12:22:40Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T12:22:41Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T12:22:41Z"}]	2025-10-26 12:22:40.212168+00	2025-10-26 12:22:41.423269+00	f36daaed-35dc-42f5-ba28-b622f6d41032	1	\N
9	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "NetworkBinding committed to backend successfully", "lastTransitionTime": "2025-10-26T12:49:50Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "NetworkBinding passed validation", "lastTransitionTime": "2025-10-26T12:49:50Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "All dependencies are ready", "lastTransitionTime": "2025-10-26T12:49:53Z"}, {"type": "PendingSync", "reason": "AllDependenciesReady", "status": "False", "message": "", "lastTransitionTime": null}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting internal processing before deletion"}]	2025-10-26 12:49:50.890447+00	2025-10-26 12:49:50.890447+00	2d9e9eaa-229f-4b88-a7de-7c973cbbc8bc	1	2025-10-26 12:50:04.755616+00
20	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-host-hb001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440999\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T14:22:14Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-26T14:22:11Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T14:22:14Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T14:22:14Z"}]	2025-10-26 14:22:11.714535+00	2025-10-26 14:22:14.796534+00	41cfb1eb-cc75-4b07-a636-5ac20c3d3443	1	\N
19	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"HostBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-hostbinding-hb001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"test-ag-hb001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"hostRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"name\\":\\"test-host-hb001\\",\\"namespace\\":\\"incloud-sgroups\\"}}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "HostBinding committed to backend successfully", "lastTransitionTime": "2025-10-26T14:22:11Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "HostBinding passed validation", "lastTransitionTime": "2025-10-26T14:22:11Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "All dependencies are ready", "lastTransitionTime": "2025-10-26T14:22:14Z"}, {"type": "PendingSync", "reason": "AllDependenciesReady", "status": "False", "message": "", "lastTransitionTime": null}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting internal processing before deletion"}]	2025-10-26 14:22:11.689605+00	2025-10-26 14:22:11.689605+00	2f6b20ac-50ad-4b28-9efa-f600a959244f	1	2025-10-26 14:26:16.295608+00
10	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-network-clean\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.200.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T12:49:53Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-26T12:22:40Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T12:49:53Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T12:49:53Z"}]	2025-10-26 12:49:50.904091+00	2025-10-26 12:49:53.731222+00	1ebeece8-8da6-4c62-a602-6b87dc260e34	1	\N
2	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-clean\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "PendingSGROUPSync", "status": "False", "message": "Awaiting synchronization with SGROUP before marking as ready", "lastTransitionTime": "2025-10-26T12:22:40Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-26T12:22:40Z"}, {"type": "PendingSync", "reason": "AwaitingSGroupSync", "status": "True", "message": "Syncing to SGROUP (attempt 1)", "lastTransitionTime": "2025-10-26T12:34:24Z"}]	2025-10-26 12:22:40.294523+00	2025-10-26 12:34:24.861907+00	1969f101-5e7e-4c9f-8fc5-11d4cd2104d1	1	2025-10-26 12:34:23.48862+00
11	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-network-clean\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.200.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T12:50:08Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-26T12:22:40Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T12:50:08Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T12:50:08Z"}]	2025-10-26 12:50:04.75931+00	2025-10-26 12:50:08.720403+00	3bc1e0c7-fa2c-4c13-9cb6-d9c2d97e3725	1	\N
24	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T18:35:43Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T18:35:43Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-26T16:33:05Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T18:35:43Z"}]	2025-10-26 16:33:05.179596+00	2025-10-26 18:35:43.624047+00	0e069fcd-7de7-48f9-a610-b912b2663c89	0	\N
31	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "AddressGroupPortMapping committed to backend successfully", "lastTransitionTime": "2025-10-26T18:57:40Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "AddressGroupPortMapping passed validation", "lastTransitionTime": "2025-10-26T18:57:40Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "AddressGroupPortMapping is ready, 1 access ports configured", "lastTransitionTime": "2025-10-26T18:57:40Z"}]	2025-10-26 16:48:13.896583+00	2025-10-26 18:57:40.144541+00	f097078d-fee4-43b7-afd1-6def6ce6fa7e	1	\N
23	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T16:32:54Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-26T16:32:49Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T16:32:54Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T16:32:54Z"}]	2025-10-26 16:32:49.563183+00	2025-10-26 16:32:54.665203+00	7fa56150-ff8a-4760-98ae-96600b3b6f38	1	\N
12	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Host\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-host-hb001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"uuid\\":\\"550e8400-e29b-41d4-a716-446655440999\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T13:34:37Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Host passed validation", "lastTransitionTime": "2025-10-26T13:34:28Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T13:34:37Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T13:34:37Z"}]	2025-10-26 13:34:28.500936+00	2025-10-26 13:34:37.262612+00	5d5f17e6-de3e-4e15-af5d-e314dfd64aac	1	\N
25	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "AddressGroupBinding committed to backend successfully", "lastTransitionTime": "2025-10-26T16:33:34Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "AddressGroupBinding passed validation", "lastTransitionTime": "2025-10-26T16:33:34Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "AddressGroupBinding is ready, 1 access ports configured", "lastTransitionTime": "2025-10-26T16:33:34Z"}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting internal processing before deletion"}]	2025-10-26 16:33:34.579599+00	2025-10-26 16:33:34.590425+00	ff93ddbe-5973-4ab7-b431-7236c7310684	1	2025-10-26 16:35:11.747434+00
26	{}	{}	{}	null	2025-10-26 16:33:34.585627+00	2025-10-26 16:35:11.752896+00	a0ab1b21-b800-47a0-bb4d-19f842824cfd	1	\N
28	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-agb001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T16:45:05Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-26T16:45:00Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T16:45:05Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T16:45:05Z"}]	2025-10-26 16:45:00.528691+00	2025-10-26 16:45:05.463378+00	371ef2ff-dce3-43d3-80bf-da3310ac02ee	1	\N
5	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-network-clean\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.200.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T12:36:10Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-26T12:22:40Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T12:36:10Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T12:36:10Z"}]	2025-10-26 12:36:08.37839+00	2025-10-26 12:36:10.057576+00	0425c250-14b6-4e6f-a0b4-7a2a01904097	1	\N
4	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"NetworkBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-binding-clean\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"test-ag-clean\\"},\\"networkRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"name\\":\\"test-network-clean\\"}}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "NetworkBinding committed to backend successfully", "lastTransitionTime": "2025-10-26T12:36:08Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "NetworkBinding passed validation", "lastTransitionTime": "2025-10-26T12:36:08Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "All dependencies are ready", "lastTransitionTime": "2025-10-26T12:36:09Z"}, {"type": "PendingSync", "reason": "AllDependenciesReady", "status": "False", "message": "", "lastTransitionTime": null}, {"type": "PendingSync", "reason": "PendingDeletion", "status": "True", "message": "Awaiting internal processing before deletion"}]	2025-10-26 12:36:08.37332+00	2025-10-26 12:36:08.37332+00	9fa73147-7831-4887-9ba7-68dbe0c1cd28	1	2025-10-26 12:40:09.995917+00
13	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-hb001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T14:22:14Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-26T13:34:51Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T14:22:14Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T14:22:14Z"}]	2025-10-26 13:34:51.968917+00	2025-10-26 14:22:14.781384+00	15ceba48-08bc-4443-bb2c-57a8608ea0ee	1	\N
30	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroupBinding\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-agb-001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"addressGroupRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"name\\":\\"test-ag-agb001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"serviceRef\\":{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"name\\":\\"test-service-agb001\\",\\"namespace\\":\\"incloud-sgroups\\"}}}\\n"}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "AddressGroupBinding committed to backend successfully", "lastTransitionTime": "2025-10-26T16:53:22Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "AddressGroupBinding passed validation", "lastTransitionTime": "2025-10-26T16:53:22Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "AddressGroupBinding is ready, 0 access ports configured", "lastTransitionTime": "2025-10-26T16:53:22Z"}]	2025-10-26 16:48:13.892675+00	2025-10-26 16:53:22.54351+00	ba6e5105-f3fc-4590-9572-91705109a223	1	2025-10-26 16:52:55.21286+00
3	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"AddressGroup\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-ag-clean\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"defaultAction\\":\\"ACCEPT\\",\\"logs\\":false,\\"trace\\":false}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T12:50:08Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Address group passed all validations", "lastTransitionTime": "2025-10-26T12:34:57Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T12:50:08Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T12:50:08Z"}]	2025-10-26 12:34:57.284507+00	2025-10-26 12:50:08.707585+00	a0516bd8-62e5-48ca-8e0b-eedf45bf54e1	1	\N
8	{}	{"kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Network\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-network-clean\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"cidr\\":\\"10.200.0.0/16\\"}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T12:40:14Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Network passed validation", "lastTransitionTime": "2025-10-26T12:22:40Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T12:40:14Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T12:40:14Z"}]	2025-10-26 12:40:09.999401+00	2025-10-26 12:40:14.89923+00	f526658e-eece-4583-b4de-d9e66d592217	1	\N
34	{}	{}	{}	[{"type": "Synced", "reason": "Synced", "status": "True", "message": "AddressGroupBinding committed to backend successfully", "lastTransitionTime": "2025-10-26T18:57:40Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "AddressGroupBinding passed validation", "lastTransitionTime": "2025-10-26T18:57:40Z"}, {"type": "Ready", "reason": "Ready", "status": "True", "message": "All dependencies are ready", "lastTransitionTime": "2025-10-26T18:57:44Z"}, {"type": "PendingSync", "reason": "AllDependenciesReady", "status": "False", "message": "", "lastTransitionTime": null}]	2025-10-26 18:57:40.12769+00	2025-10-26 18:57:44.104321+00	e9be74c1-62cc-45dd-b746-115ca292ca70	1	2025-10-26 18:57:44.104321+00
29	{"app.kubernetes.io/managed-by": "netguard-apiserver"}	{"netguard.sgroups.io/created-by": "aggregated-api", "kubectl.kubernetes.io/last-applied-configuration": "{\\"apiVersion\\":\\"netguard.sgroups.io/v1beta1\\",\\"kind\\":\\"Service\\",\\"metadata\\":{\\"annotations\\":{},\\"name\\":\\"test-service-agb001\\",\\"namespace\\":\\"incloud-sgroups\\"},\\"spec\\":{\\"ingressPorts\\":[{\\"description\\":\\"HTTP port\\",\\"port\\":\\"8080\\",\\"protocol\\":\\"TCP\\"}]}}\\n"}	{}	[{"type": "Ready", "reason": "Ready", "status": "True", "message": "Synced to SGROUP", "lastTransitionTime": "2025-10-26T18:57:44Z"}, {"type": "Synced", "reason": "Synced", "status": "True", "message": "Successfully synced", "lastTransitionTime": "2025-10-26T18:57:44Z"}, {"type": "Validated", "reason": "Validated", "status": "True", "message": "Service passed validation", "lastTransitionTime": "2025-10-26T16:47:42Z"}, {"type": "PendingSync", "reason": "Synced", "status": "False", "message": "All sync operations complete", "lastTransitionTime": "2025-10-26T18:57:44Z"}]	2025-10-26 16:47:42.734199+00	2025-10-26 18:57:44.146954+00	20f77827-ed42-4e33-b349-eda926098425	0	\N
\.


--
-- Data for Name: netguard_db_ver; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.netguard_db_ver (id, version_id, is_applied, tstamp) FROM stdin;
1	0	t	2025-10-26 12:21:15.440907
2	1	t	2025-10-26 12:21:15.452677
3	2	t	2025-10-26 12:21:15.467568
4	3	t	2025-10-26 12:21:15.469299
5	4	t	2025-10-26 12:21:15.471698
6	5	t	2025-10-26 12:21:15.473406
7	6	t	2025-10-26 12:21:15.474614
8	7	t	2025-10-26 12:21:15.518474
9	8	t	2025-10-26 12:21:15.520986
10	9	t	2025-10-26 12:21:15.522591
11	10	t	2025-10-26 12:21:15.523506
12	11	t	2025-10-26 12:21:15.528386
13	12	t	2025-10-26 12:21:15.531269
14	13	t	2025-10-26 12:21:15.538165
15	14	t	2025-10-26 12:21:15.539627
16	15	t	2025-10-26 12:21:15.541272
17	16	t	2025-10-26 12:21:15.541996
18	17	t	2025-10-26 12:21:15.542798
19	18	t	2025-10-26 12:21:15.543426
20	19	t	2025-10-26 12:21:15.559845
21	20	t	2025-10-26 12:21:15.561276
22	21	t	2025-10-26 12:21:15.562396
23	22	t	2025-10-26 12:21:15.562924
24	23	t	2025-10-26 12:21:15.563375
25	24	t	2025-10-26 12:21:15.564618
26	25	t	2025-10-26 12:21:15.565627
27	26	t	2025-10-26 12:21:15.568487
28	27	t	2025-10-26 12:21:15.570596
29	28	t	2025-10-26 12:21:15.571763
30	29	t	2025-10-26 12:21:15.572565
31	30	t	2025-10-26 12:21:15.626846
32	31	t	2025-10-26 12:21:15.627884
33	32	t	2025-10-26 12:21:15.629065
34	33	t	2025-10-26 13:26:19.044428
35	34	t	2025-10-26 13:53:10.684854
36	35	t	2025-10-26 14:20:44.541247
37	36	t	2025-10-26 14:25:13.754112
38	37	t	2025-10-26 16:51:52.154379
\.


--
-- Data for Name: network_bindings; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.network_bindings (namespace, name, network_namespace, network_name, address_group_namespace, address_group_name, resource_version) FROM stdin;
\.


--
-- Data for Name: networks; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.networks (namespace, name, network_items, is_bound, binding_ref_namespace, binding_ref_name, address_group_ref_namespace, address_group_ref_name, resource_version, cidr) FROM stdin;
incloud-sgroups	test-network-clean	[{"cidr": "10.200.0.0/16", "name": ""}]	f	\N	\N	\N	\N	11	10.200.0.0/16
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
-- Data for Name: service_rule_refs; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.service_rule_refs (id, service_ref, rule_id, role, created_at) FROM stdin;
\.


--
-- Data for Name: services; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.services (namespace, name, description, ingress_ports, resource_version, address_groups, aggregated_address_groups, xsvcsvc_rules_as_from, xsvcsvc_rules_as_to) FROM stdin;
incloud-sgroups	dynamic-19a215dc69a88bbb	Service  managed by netguard-apiserver	[]	24	[]	[]	[]	[]
incloud-sgroups	test-service-agb001	Service test-service-agb001 managed by netguard-apiserver	[{"port": "8080", "protocol": "TCP", "description": "HTTP port"}]	29	[]	[{"ref": {"kind": "AddressGroup", "name": "test-ag-agb001", "namespace": "incloud-sgroups", "apiVersion": "netguard.sgroups.io/v1beta1"}, "source": "binding"}]	[]	[]
\.


--
-- Data for Name: svc_svc_rules; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.svc_svc_rules (id, namespace, name, service_from_ref, service_to_ref, action, priority, logs, trace, created_at, updated_at, resource_version) FROM stdin;
\.


--
-- Data for Name: sync_outbox; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.sync_outbox (id, resource_type, resource_id, resource_namespace, resource_name, operation, target_system, payload, delta, affects_resources, status, attempts, max_retries, next_retry_at, last_error, error_category, created_at, updated_at, processed_at) FROM stdin;
f4ec14df-ecc4-454e-a4d1-72e3d498825f	AddressGroupBinding	e9be74c1-62cc-45dd-b746-115ca292ca70	incloud-sgroups	test-agb-final	UPDATE	INTERNAL	{"name": "test-agb-final", "service": {"name": "test-service-agb001", "namespace": "incloud-sgroups"}, "namespace": "incloud-sgroups", "address_group": {"name": "test-ag-agb001", "namespace": "incloud-sgroups"}}	\N	\N	PENDING	0	5	2025-10-26 18:57:44.104321+00	\N	\N	2025-10-26 18:57:40.148156+00	2025-10-26 18:57:44.104321+00	\N
\.


--
-- Data for Name: sync_status; Type: TABLE DATA; Schema: public; Owner: netguard
--

COPY public.sync_status (id, updated_at) FROM stdin;
1	2025-10-26 12:21:15.452677+00
\.


--
-- Name: k8s_metadata_resource_version_seq; Type: SEQUENCE SET; Schema: public; Owner: netguard
--

SELECT pg_catalog.setval('public.k8s_metadata_resource_version_seq', 34, true);


--
-- Name: netguard_db_ver_id_seq; Type: SEQUENCE SET; Schema: public; Owner: netguard
--

SELECT pg_catalog.setval('public.netguard_db_ver_id_seq', 38, true);


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
-- Name: host_bindings host_bindings_host_unique; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.host_bindings
    ADD CONSTRAINT host_bindings_host_unique UNIQUE (host_namespace, host_name);


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
-- Name: network_bindings network_bindings_network_unique; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.network_bindings
    ADD CONSTRAINT network_bindings_network_unique UNIQUE (network_namespace, network_name);


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
-- Name: service_rule_refs service_rule_refs_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.service_rule_refs
    ADD CONSTRAINT service_rule_refs_pkey PRIMARY KEY (id);


--
-- Name: services services_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.services
    ADD CONSTRAINT services_pkey PRIMARY KEY (namespace, name);


--
-- Name: svc_svc_rules svc_svc_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.svc_svc_rules
    ADD CONSTRAINT svc_svc_rules_pkey PRIMARY KEY (id);


--
-- Name: sync_outbox sync_outbox_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.sync_outbox
    ADD CONSTRAINT sync_outbox_pkey PRIMARY KEY (id);


--
-- Name: sync_outbox sync_outbox_resource_type_resource_id_operation_target_syst_key; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.sync_outbox
    ADD CONSTRAINT sync_outbox_resource_type_resource_id_operation_target_syst_key UNIQUE (resource_type, resource_id, operation, target_system);


--
-- Name: sync_status sync_status_pkey; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.sync_status
    ADD CONSTRAINT sync_status_pkey PRIMARY KEY (id);


--
-- Name: svc_svc_rules uk_svc_svc_rules_namespace_name; Type: CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.svc_svc_rules
    ADD CONSTRAINT uk_svc_svc_rules_namespace_name UNIQUE (namespace, name);


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
-- Name: idx_service_rule_refs_rule_id; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_service_rule_refs_rule_id ON public.service_rule_refs USING btree (rule_id);


--
-- Name: idx_service_rule_refs_service; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_service_rule_refs_service ON public.service_rule_refs USING btree (service_ref);


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
-- Name: idx_services_xsvcsvc_rules_as_from; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_services_xsvcsvc_rules_as_from ON public.services USING gin (xsvcsvc_rules_as_from);


--
-- Name: idx_services_xsvcsvc_rules_as_to; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_services_xsvcsvc_rules_as_to ON public.services USING gin (xsvcsvc_rules_as_to);


--
-- Name: idx_svc_svc_rules_action; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_svc_svc_rules_action ON public.svc_svc_rules USING btree (action);


--
-- Name: idx_svc_svc_rules_service_from_name; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_svc_svc_rules_service_from_name ON public.svc_svc_rules USING btree (((service_from_ref ->> 'name'::text)));


--
-- Name: idx_svc_svc_rules_service_from_namespace; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_svc_svc_rules_service_from_namespace ON public.svc_svc_rules USING btree (((service_from_ref ->> 'namespace'::text)));


--
-- Name: idx_svc_svc_rules_service_to_name; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_svc_svc_rules_service_to_name ON public.svc_svc_rules USING btree (((service_to_ref ->> 'name'::text)));


--
-- Name: idx_svc_svc_rules_service_to_namespace; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_svc_svc_rules_service_to_namespace ON public.svc_svc_rules USING btree (((service_to_ref ->> 'namespace'::text)));


--
-- Name: idx_svc_svc_rules_unique_pair; Type: INDEX; Schema: public; Owner: netguard
--

CREATE UNIQUE INDEX idx_svc_svc_rules_unique_pair ON public.svc_svc_rules USING btree (namespace, ((service_from_ref ->> 'namespace'::text)), ((service_from_ref ->> 'name'::text)), ((service_to_ref ->> 'namespace'::text)), ((service_to_ref ->> 'name'::text)));


--
-- Name: idx_sync_outbox_created_at; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_sync_outbox_created_at ON public.sync_outbox USING btree (created_at);


--
-- Name: idx_sync_outbox_resource; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_sync_outbox_resource ON public.sync_outbox USING btree (resource_type, resource_id);


--
-- Name: idx_sync_outbox_status_next_retry; Type: INDEX; Schema: public; Owner: netguard
--

CREATE INDEX idx_sync_outbox_status_next_retry ON public.sync_outbox USING btree (status, next_retry_at) WHERE (status = ANY (ARRAY['PENDING'::public.outbox_status, 'FAILED_RETRYABLE'::public.outbox_status]));


--
-- Name: address_groups address_group_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER address_group_before_delete BEFORE DELETE ON public.address_groups FOR EACH ROW EXECUTE FUNCTION public.trigger_address_group_before_delete();


--
-- Name: address_group_bindings address_group_binding_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER address_group_binding_before_delete BEFORE DELETE ON public.address_group_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_address_group_binding_before_delete();


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
-- Name: networks network_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER network_before_delete BEFORE DELETE ON public.networks FOR EACH ROW EXECUTE FUNCTION public.trigger_network_before_delete();


--
-- Name: network_bindings network_binding_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER network_binding_before_delete BEFORE DELETE ON public.network_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_network_binding_before_delete();


--
-- Name: services service_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER service_before_delete BEFORE DELETE ON public.services FOR EACH ROW EXECUTE FUNCTION public.trigger_service_before_delete();


--
-- Name: svc_svc_rules svcsvc_rule_before_delete; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER svcsvc_rule_before_delete BEFORE DELETE ON public.svc_svc_rules FOR EACH ROW EXECUTE FUNCTION public.trigger_svcsvc_rule_before_delete();


--
-- Name: address_group_bindings trg_address_group_binding_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_address_group_binding_upsert_outbox AFTER INSERT OR UPDATE ON public.address_group_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_address_group_binding_upsert_outbox();


--
-- Name: address_groups trg_address_group_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_address_group_upsert_outbox AFTER INSERT OR UPDATE ON public.address_groups FOR EACH ROW EXECUTE FUNCTION public.trigger_address_group_upsert_outbox();


--
-- Name: host_bindings trg_ag_update_on_binding_change; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_ag_update_on_binding_change AFTER INSERT OR DELETE OR UPDATE ON public.host_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_ag_update_on_binding_change();


--
-- Name: host_bindings trg_host_binding_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_host_binding_upsert_outbox AFTER INSERT OR UPDATE ON public.host_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_host_binding_upsert_outbox();


--
-- Name: hosts trg_host_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_host_upsert_outbox AFTER INSERT OR UPDATE ON public.hosts FOR EACH ROW EXECUTE FUNCTION public.trigger_host_upsert_outbox();


--
-- Name: network_bindings trg_network_binding_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_network_binding_upsert_outbox AFTER INSERT OR UPDATE ON public.network_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_network_binding_upsert_outbox();


--
-- Name: networks trg_network_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_network_upsert_outbox AFTER INSERT OR UPDATE ON public.networks FOR EACH ROW EXECUTE FUNCTION public.trigger_network_upsert_outbox();


--
-- Name: services trg_service_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_service_upsert_outbox AFTER INSERT OR UPDATE ON public.services FOR EACH ROW EXECUTE FUNCTION public.trigger_service_upsert_outbox();


--
-- Name: svc_svc_rules trg_svcsvc_rule_upsert_outbox; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_svcsvc_rule_upsert_outbox AFTER INSERT OR UPDATE ON public.svc_svc_rules FOR EACH ROW EXECUTE FUNCTION public.trigger_svcsvc_rule_upsert_outbox();


--
-- Name: network_bindings trg_sync_address_group_networks; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_sync_address_group_networks AFTER INSERT OR DELETE OR UPDATE ON public.network_bindings FOR EACH ROW EXECUTE FUNCTION public.sync_address_group_networks_on_binding_change();


--
-- Name: networks trg_sync_address_group_networks_on_network; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_sync_address_group_networks_on_network AFTER INSERT OR DELETE OR UPDATE ON public.networks FOR EACH ROW EXECUTE FUNCTION public.sync_address_group_networks_on_network_change();


--
-- Name: svc_svc_rules trg_sync_service_rule_refs; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_sync_service_rule_refs AFTER INSERT OR DELETE OR UPDATE ON public.svc_svc_rules FOR EACH ROW EXECUTE FUNCTION public.sync_service_rule_refs();


--
-- Name: service_rule_refs trg_update_service_xsvcsvc_rules; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER trg_update_service_xsvcsvc_rules AFTER INSERT OR DELETE ON public.service_rule_refs FOR EACH ROW EXECUTE FUNCTION public.update_service_xsvcsvc_rules();


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
-- Name: host_bindings update_aggregated_hosts_on_binding_change_trigger; Type: TRIGGER; Schema: public; Owner: netguard
--

CREATE TRIGGER update_aggregated_hosts_on_binding_change_trigger AFTER INSERT OR DELETE OR UPDATE ON public.host_bindings FOR EACH ROW EXECUTE FUNCTION public.trigger_update_aggregated_hosts_on_binding_change();


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
-- Name: service_rule_refs service_rule_refs_rule_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.service_rule_refs
    ADD CONSTRAINT service_rule_refs_rule_id_fkey FOREIGN KEY (rule_id) REFERENCES public.svc_svc_rules(id) ON DELETE CASCADE;


--
-- Name: services services_resource_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: netguard
--

ALTER TABLE ONLY public.services
    ADD CONSTRAINT services_resource_version_fkey FOREIGN KEY (resource_version) REFERENCES public.k8s_metadata(resource_version) ON DELETE CASCADE;


--
-- Name: SCHEMA public; Type: ACL; Schema: -; Owner: netguard
--

REVOKE USAGE ON SCHEMA public FROM PUBLIC;
GRANT ALL ON SCHEMA public TO PUBLIC;


--
-- PostgreSQL database dump complete
--

