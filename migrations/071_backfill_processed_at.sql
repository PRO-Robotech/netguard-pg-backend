-- +goose Up
-- Migration 071: Backfill processed_at for existing SUCCESS entries
--
-- ===========================================================================
-- CRITICAL DATA FIX: processed_at was NULL for all SUCCESS entries
-- ===========================================================================
--
-- Problem:
--   MarkCompleted() and deleteResourceFromDB() did not set processed_at field.
--   This caused:
--   - processed_at IS NULL for 56+ existing SUCCESS entries
--   - DeleteOldSuccessful cleanup job doesn't work (looks for processed_at)
--   - Cannot distinguish processed vs unprocessed SUCCESS entries
--
-- Root Cause:
--   1. outbox_repository.go:276 - MarkCompleted() missing processed_at
--   2. process_entity.go:642 - deleteResourceFromDB() missing processed_at
--
-- Solution:
--   1. Fixed MarkCompleted() to set processed_at = NOW()
--   2. Fixed deleteResourceFromDB() to set processed_at = NOW()
--   3. THIS MIGRATION: Backfill processed_at for existing SUCCESS entries
--
-- Backfill Strategy:
--   Set processed_at = updated_at for all SUCCESS entries with NULL processed_at
--   Rationale: updated_at is the closest timestamp to when processing completed
--
-- Impact:
--   - Fixes cleanup job functionality
--   - Enables proper monitoring of processed entries
--   - Restores data integrity for sync_outbox table
--
-- Related:
--   - Migration 025: Created sync_outbox table with processed_at field
--   - Fixed in: outbox_repository.go, process_entity.go
--
-- Date: 2025-10-31
-- ===========================================================================

-- +goose StatementBegin

DO $$
DECLARE
    v_affected_count INT;
BEGIN
    RAISE NOTICE '[Migration 071] Starting backfill of processed_at for SUCCESS entries...';

    -- Backfill processed_at = updated_at for all SUCCESS entries with NULL processed_at
    UPDATE sync_outbox
    SET processed_at = updated_at
    WHERE status = 'SUCCESS'
      AND processed_at IS NULL;

    GET DIAGNOSTICS v_affected_count = ROW_COUNT;

    RAISE NOTICE '[Migration 071] Backfilled processed_at for % SUCCESS entries', v_affected_count;

    -- Verification: Check if any SUCCESS entries still have NULL processed_at
    SELECT COUNT(*) INTO v_affected_count
    FROM sync_outbox
    WHERE status = 'SUCCESS' AND processed_at IS NULL;

    IF v_affected_count > 0 THEN
        RAISE WARNING '[Migration 071] Still found % SUCCESS entries with NULL processed_at!', v_affected_count;
    ELSE
        RAISE NOTICE '[Migration 071] Verification passed: All SUCCESS entries have processed_at set';
    END IF;

    RAISE NOTICE '[Migration 071] Migration complete. processed_at backfill successful.';
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Note: This is a data migration. Rolling back would set processed_at back to NULL,
-- which would break the system again. Therefore, we do NOT roll back the data change.
-- Instead, we just log a warning.

DO $$
BEGIN
    RAISE WARNING '[Migration 071 Rollback] This is a data fix migration. Rollback is NOT recommended.';
    RAISE WARNING '[Migration 071 Rollback] If you rollback, processed_at will remain set (no change).';
    RAISE WARNING '[Migration 071 Rollback] To truly rollback, manually run: UPDATE sync_outbox SET processed_at = NULL WHERE status = ''SUCCESS'';';
    RAISE NOTICE '[Migration 071 Rollback] No changes made - processed_at values preserved.';
END $$;

-- +goose StatementEnd
