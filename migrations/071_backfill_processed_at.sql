-- +goose Up

-- +goose StatementBegin

DO $$
DECLARE
    v_affected_count INT;
BEGIN
    RAISE NOTICE '[Migration 071] Starting backfill of processed_at for SUCCESS entries...';

    UPDATE sync_outbox
    SET processed_at = updated_at
    WHERE status = 'SUCCESS'
      AND processed_at IS NULL;

    GET DIAGNOSTICS v_affected_count = ROW_COUNT;

    RAISE NOTICE '[Migration 071] Backfilled processed_at for % SUCCESS entries', v_affected_count;

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


DO $$
BEGIN
    RAISE WARNING '[Migration 071 Rollback] This is a data fix migration. Rollback is NOT recommended.';
    RAISE WARNING '[Migration 071 Rollback] If you rollback, processed_at will remain set (no change).';
    RAISE WARNING '[Migration 071 Rollback] To truly rollback, manually run: UPDATE sync_outbox SET processed_at = NULL WHERE status = ''SUCCESS'';';
    RAISE NOTICE '[Migration 071 Rollback] No changes made - processed_at values preserved.';
END $$;

-- +goose StatementEnd
