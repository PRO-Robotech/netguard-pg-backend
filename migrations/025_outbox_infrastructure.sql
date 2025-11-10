-- +goose Up
-- +goose StatementBegin

CREATE TYPE sync_operation AS ENUM ('CREATE', 'UPDATE', 'DELETE');

CREATE TYPE target_system AS ENUM ('SGROUP', 'INTERNAL');

CREATE TYPE outbox_status AS ENUM (
    'PENDING',
    'PROCESSING',
    'SUCCESS',
    'FAILED_RETRYABLE',
    'FAILED_PERMANENT'
);

CREATE TABLE sync_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type VARCHAR(50) NOT NULL,
    resource_id UUID NOT NULL,
    resource_namespace VARCHAR(253) NOT NULL,
    resource_name VARCHAR(253) NOT NULL,
    operation sync_operation NOT NULL,
    target_system target_system NOT NULL,
    payload JSONB NOT NULL,
    delta JSONB,
    affects_resources JSONB,
    status outbox_status NOT NULL DEFAULT 'PENDING',
    attempts INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 5,
    next_retry_at TIMESTAMPTZ,
    last_error TEXT,
    error_category VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    UNIQUE (resource_type, resource_id, operation, target_system)
);

CREATE INDEX idx_sync_outbox_status_next_retry
    ON sync_outbox (status, next_retry_at)
    WHERE status IN ('PENDING', 'FAILED_RETRYABLE');

CREATE INDEX idx_sync_outbox_resource
    ON sync_outbox (resource_type, resource_id);

CREATE INDEX idx_sync_outbox_created_at
    ON sync_outbox (created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS sync_outbox;
DROP TYPE IF EXISTS outbox_status;
DROP TYPE IF EXISTS target_system;
DROP TYPE IF EXISTS sync_operation;

-- +goose StatementEnd
