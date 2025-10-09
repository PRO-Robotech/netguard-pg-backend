-- +goose Up
-- +goose StatementBegin

DROP TRIGGER IF EXISTS cascade_host_from_address_groups ON hosts;

CREATE TRIGGER cascade_host_from_address_groups
    AFTER DELETE ON hosts
    FOR EACH ROW
    EXECUTE FUNCTION cascade_host_deletion();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS cascade_host_from_address_groups ON hosts;

CREATE TRIGGER cascade_host_from_address_groups
    BEFORE DELETE ON hosts
    FOR EACH ROW
    EXECUTE FUNCTION cascade_host_deletion();

-- +goose StatementEnd
