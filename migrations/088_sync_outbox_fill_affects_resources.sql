-- +goose Up
-- +goose StatementBegin

-- Migration 088: Ensure sync_outbox.affects_resources mirrors payload.affectedResources
-- Purpose: HostBinding DELETE entries inserted by cascade trigger only embed affected resources
--          inside payload. This trigger copies the array into affects_resources so worker logic
--          can rely on either field. Backfill covers existing rows.

CREATE OR REPLACE FUNCTION sync_outbox_fill_affects_resources_fn()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.affects_resources IS NULL AND NEW.payload ? 'affectedResources' THEN
        NEW.affects_resources := NEW.payload -> 'affectedResources';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS sync_outbox_fill_affects_resources_trg ON sync_outbox;

CREATE TRIGGER sync_outbox_fill_affects_resources_trg
BEFORE INSERT ON sync_outbox
FOR EACH ROW
WHEN (NEW.affects_resources IS NULL)
EXECUTE FUNCTION sync_outbox_fill_affects_resources_fn();

-- +goose StatementEnd

-- +goose StatementBegin

UPDATE sync_outbox
SET affects_resources = payload -> 'affectedResources'
WHERE affects_resources IS NULL
  AND payload ? 'affectedResources';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS sync_outbox_fill_affects_resources_trg ON sync_outbox;
DROP FUNCTION IF EXISTS sync_outbox_fill_affects_resources_fn();

-- +goose StatementEnd

