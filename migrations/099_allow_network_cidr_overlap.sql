-- +goose Up
ALTER TABLE networks
    DROP CONSTRAINT IF EXISTS prevent_networks_cidr_overlap;

-- +goose Down
ALTER TABLE networks
    ADD CONSTRAINT prevent_networks_cidr_overlap
        EXCLUDE USING GIST (cidr inet_ops WITH &&)
        DEFERRABLE INITIALLY DEFERRED;

COMMENT ON CONSTRAINT prevent_networks_cidr_overlap ON networks IS
    'Prevents overlapping CIDR ranges using PostgreSQL GIST index. Error code: 23P01 (exclusion_violation)';

