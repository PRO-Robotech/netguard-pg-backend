-- +goose Up
-- Ensure DELETE notifications use a fresh resourceVersion so watch clients never miss events
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_k8s_resource_change()
RETURNS TRIGGER AS $$
DECLARE
    notification JSON;
    resource_type TEXT;
    operation TEXT;
    ns TEXT;
    n TEXT;
    rv BIGINT;
    data_payload JSON;
BEGIN
    -- Determine operation type
    IF (TG_OP = 'DELETE') THEN
        operation := 'DELETE';
        ns := OLD.namespace;
        n := OLD.name;
        -- Allocate a fresh resource version for delete events to keep watch streams monotonic
        rv := nextval('k8s_metadata_resource_version_seq');
        resource_type := TG_TABLE_NAME;
        
        -- For DELETE, include OLD data in notification
        data_payload := row_to_json(OLD);
    ELSIF (TG_OP = 'INSERT') THEN
        operation := 'INSERT';
        ns := NEW.namespace;
        n := NEW.name;
        rv := NEW.resource_version;
        resource_type := TG_TABLE_NAME;
        data_payload := row_to_json(NEW);
    ELSIF (TG_OP = 'UPDATE') THEN
        operation := 'UPDATE';
        ns := NEW.namespace;
        n := NEW.name;
        rv := NEW.resource_version;
        resource_type := TG_TABLE_NAME;
        data_payload := row_to_json(NEW);
    END IF;

    -- Build notification JSON
    notification := json_build_object(
        'operation', operation,
        'resource_type', resource_type,
        'namespace', ns,
        'name', n,
        'resource_version', rv,
        'data', data_payload,
        'timestamp', NOW()
    );

    -- Send notification to k8s_resource_changes channel
    PERFORM pg_notify('k8s_resource_changes', notification::TEXT);

    -- Return appropriate value based on operation
    IF (TG_OP = 'DELETE') THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- Restore previous behavior (DELETE events reuse OLD.resource_version)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_k8s_resource_change()
RETURNS TRIGGER AS $$
DECLARE
    notification JSON;
    resource_type TEXT;
    operation TEXT;
    ns TEXT;
    n TEXT;
    rv BIGINT;
    data_payload JSON;
BEGIN
    -- Determine operation type
    IF (TG_OP = 'DELETE') THEN
        operation := 'DELETE';
        ns := OLD.namespace;
        n := OLD.name;
        rv := OLD.resource_version;
        resource_type := TG_TABLE_NAME;
        
        -- For DELETE, include OLD data in notification
        data_payload := row_to_json(OLD);
    ELSIF (TG_OP = 'INSERT') THEN
        operation := 'INSERT';
        ns := NEW.namespace;
        n := NEW.name;
        rv := NEW.resource_version;
        resource_type := TG_TABLE_NAME;
        data_payload := row_to_json(NEW);
    ELSIF (TG_OP = 'UPDATE') THEN
        operation := 'UPDATE';
        ns := NEW.namespace;
        n := NEW.name;
        rv := NEW.resource_version;
        resource_type := TG_TABLE_NAME;
        data_payload := row_to_json(NEW);
    END IF;

    -- Build notification JSON
    notification := json_build_object(
        'operation', operation,
        'resource_type', resource_type,
        'namespace', ns,
        'name', n,
        'resource_version', rv,
        'data', data_payload,
        'timestamp', NOW()
    );

    -- Send notification to k8s_resource_changes channel
    PERFORM pg_notify('k8s_resource_changes', notification::TEXT);

    -- Return appropriate value based on operation
    IF (TG_OP = 'DELETE') THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

