-- +goose Up
-- Add indexes for field selectors on services table

-- Index for metadata.name field selector
CREATE INDEX IF NOT EXISTS idx_services_name
    ON services(name);

-- Index for metadata.namespace field selector
CREATE INDEX IF NOT EXISTS idx_services_namespace
    ON services(namespace);

-- Index for spec.description field selector (useful for CONTAINS, STARTS_WITH, ENDS_WITH)
CREATE INDEX IF NOT EXISTS idx_services_description
    ON services(description);

-- GIN index for description text search (useful for CONTAINS operator)
CREATE INDEX IF NOT EXISTS idx_services_description_gin
    ON services USING GIN (to_tsvector('english', description));

-- Composite index for common queries (namespace + name)
-- Already exists as PRIMARY KEY (namespace, name), no need to add

-- +goose Down
-- Remove field selector indexes from services table

DROP INDEX IF EXISTS idx_services_description_gin;
DROP INDEX IF EXISTS idx_services_description;
DROP INDEX IF EXISTS idx_services_namespace;
DROP INDEX IF EXISTS idx_services_name;
