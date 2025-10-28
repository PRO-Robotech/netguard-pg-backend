package writers

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
	"netguard-pg-backend/internal/domain/ports"
	"sync/atomic"
)

type Writer struct {
	registry     ports.Registry
	tx           pgx.Tx
	ctx          context.Context
	affectedRows *int64
	committed    bool
}

func NewWriter(registry ports.Registry, tx pgx.Tx, ctx context.Context) *Writer {
	affectedRows := int64(0)
	return &Writer{
		registry:     registry,
		tx:           tx,
		ctx:          ctx,
		affectedRows: &affectedRows,
		committed:    false,
	}
}
func (w *Writer) addAffectedRows(count int64) {
	atomic.AddInt64(w.affectedRows, count)
}
func (w *Writer) getAffectedRows() int64 {
	return atomic.LoadInt64(w.affectedRows)
}
func (w *Writer) exec(ctx context.Context, query string, args ...interface{}) error {
	result, err := w.tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	w.addAffectedRows(result.RowsAffected())
	return nil
}
func (w *Writer) Commit() error {
	if w.committed {
		return errors.New("transaction already committed")
	}
	affectedRowsCount := w.getAffectedRows()
	if affectedRowsCount > 0 {
		syncStatusQuery := `
			INSERT INTO sync_status (updated_at, total_operations)
			VALUES (NOW(), $1)
			ON CONFLICT (id) DO UPDATE
			SET updated_at = NOW(), total_operations = total_operations + $1`
		if _, err := w.tx.Exec(w.ctx, syncStatusQuery, affectedRowsCount); err != nil {
			w.tx.Rollback(w.ctx)
			return errors.Wrap(err, "failed to update sync status")
		}
	}
	if err := w.tx.Commit(w.ctx); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}
	w.committed = true
	return nil
}
func (w *Writer) Abort() {
	if !w.committed {
		w.tx.Rollback(w.ctx)
	}
}
func (w *Writer) GetTx() pgx.Tx {
	return w.tx
}
func (w *Writer) MarkForDeletionWithStatus(namespace, name, kind string) error {
	tableMap := map[string]string{
		"AddressGroup":   "address_groups",
		"Host":           "hosts",
		"Network":        "networks",
		"Service":        "services",
		"HostBinding":    "host_bindings",
		"NetworkBinding": "network_bindings",
		"RuleS2S":        "rule_s2s",
		"IEAgAgRule":     "ie_ag_ag_rules",
	}
	tableName, ok := tableMap[kind]
	if !ok {
		return fmt.Errorf("unknown resource kind: %s", kind)
	}
	query := fmt.Sprintf(`
		UPDATE k8s_metadata
		SET
			deletion_timestamp = NOW(),
			conditions = jsonb_set(
				COALESCE(conditions, '[]'::jsonb),
				'{0}',
				jsonb_build_object(
					'type', 'Ready',
					'status', 'False',
					'reason', 'Terminating',
					'message', 'Resource is being deleted and synchronized with SGROUP',
					'lastTransitionTime', to_char(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
				),
				true
			)
		WHERE resource_version = (
			SELECT resource_version FROM %s WHERE namespace = $1 AND name = $2
		)
		AND deletion_timestamp IS NULL`, tableName)
	result, err := w.tx.Exec(w.ctx, query, namespace, name)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	var resourceUID string
	getUIDQuery := fmt.Sprintf(`
		SELECT m.uid
		FROM %s r
		INNER JOIN k8s_metadata m ON r.resource_version = m.resource_version
		WHERE r.namespace = $1 AND r.name = $2`, tableName)
	err = w.tx.QueryRow(w.ctx, getUIDQuery, namespace, name).Scan(&resourceUID)
	if err != nil {
		return fmt.Errorf("failed to get resource UID: %w", err)
	}
	insertOutboxQuery := `
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
			$1::text,
			$2::uuid,
			$3::text,
			$4::text,
			'DELETE'::sync_operation,
			'SGROUP'::target_system,
			jsonb_build_object(
				'namespace', $3::text,
				'name', $4::text
			),
			'PENDING'::outbox_status,
			0,
			5,
			NOW(),
			NOW(),
			NOW()
		)
		ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING`
	_, err = w.tx.Exec(w.ctx, insertOutboxQuery, kind, resourceUID, namespace, name)
	if err != nil {
		return fmt.Errorf("failed to create DELETE entry in sync_outbox: %w", err)
	}
	return nil
}
