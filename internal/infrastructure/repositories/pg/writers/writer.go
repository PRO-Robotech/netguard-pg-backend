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
		"AddressGroup":        "address_groups",
		"Host":                "hosts",
		"Network":             "networks",
		"Service":             "services",
		"HostBinding":         "host_bindings",
		"NetworkBinding":      "network_bindings",
		"AddressGroupBinding": "address_group_bindings",
		"RuleS2S":             "rule_s2s",
		"SvcSvcRule":          "svc_svc_rules",
		"IEAgAgRule":          "ie_ag_ag_rules",
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
				CASE
					WHEN conditions IS NULL OR jsonb_typeof(conditions) = 'array' THEN COALESCE(conditions, '[]'::jsonb)
					ELSE '[]'::jsonb
				END,
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
	// Step 1: Get resource UID + resource-specific fields
	var resourceUID string
	var payloadJSON string

	switch kind {
	case "Network":
		// For Network: get UID + CIDR
		getResourceQuery := fmt.Sprintf(`
			SELECT m.uid, r.cidr::text
			FROM %s r
			INNER JOIN k8s_metadata m ON r.resource_version = m.resource_version
			WHERE r.namespace = $1 AND r.name = $2`, tableName)
		var cidr string
		err = w.tx.QueryRow(w.ctx, getResourceQuery, namespace, name).Scan(&resourceUID, &cidr)
		if err != nil {
			return fmt.Errorf("failed to get Network UID and CIDR: %w", err)
		}
		// Build payload with CIDR
		payloadJSON = fmt.Sprintf(`{"namespace": "%s", "name": "%s", "cidr": "%s"}`, namespace, name, cidr)

	case "Host":
		// For Host: get UID + UUID
		getResourceQuery := fmt.Sprintf(`
			SELECT m.uid, r.uuid::text
			FROM %s r
			INNER JOIN k8s_metadata m ON r.resource_version = m.resource_version
			WHERE r.namespace = $1 AND r.name = $2`, tableName)
		var uuid string
		err = w.tx.QueryRow(w.ctx, getResourceQuery, namespace, name).Scan(&resourceUID, &uuid)
		if err != nil {
			return fmt.Errorf("failed to get Host UID and UUID: %w", err)
		}
		// Build payload with UUID
		payloadJSON = fmt.Sprintf(`{"namespace": "%s", "name": "%s", "uuid": "%s"}`, namespace, name, uuid)

	case "AddressGroup":
		// For AddressGroup: get UID + default_action
		getResourceQuery := fmt.Sprintf(`
			SELECT m.uid, r.default_action::text
			FROM %s r
			INNER JOIN k8s_metadata m ON r.resource_version = m.resource_version
			WHERE r.namespace = $1 AND r.name = $2`, tableName)
		var defaultAction string
		err = w.tx.QueryRow(w.ctx, getResourceQuery, namespace, name).Scan(&resourceUID, &defaultAction)
		if err != nil {
			return fmt.Errorf("failed to get AddressGroup UID and defaultAction: %w", err)
		}
		// Build payload with defaultAction
		payloadJSON = fmt.Sprintf(`{"namespace": "%s", "name": "%s", "defaultAction": "%s"}`, namespace, name, defaultAction)

	default:
		// For other resources (Service, Bindings, Rules): just get UID
		getUIDQuery := fmt.Sprintf(`
			SELECT m.uid
			FROM %s r
			INNER JOIN k8s_metadata m ON r.resource_version = m.resource_version
			WHERE r.namespace = $1 AND r.name = $2`, tableName)
		err = w.tx.QueryRow(w.ctx, getUIDQuery, namespace, name).Scan(&resourceUID)
		if err != nil {
			return fmt.Errorf("failed to get resource UID: %w", err)
		}
		// Build basic payload
		payloadJSON = fmt.Sprintf(`{"namespace": "%s", "name": "%s"}`, namespace, name)
	}

	// Step 2: Determine target_system based on resource type
	// PROCESS resources (all *Binding types) use INTERNAL - they don't sync to SGROUP directly
	// ENTITY resources (Host, Network, AddressGroup, Service, etc.) use SGROUP
	var targetSystem string
	switch kind {
	case "HostBinding", "NetworkBinding", "AddressGroupBinding":
		targetSystem = "INTERNAL"
	default:
		targetSystem = "SGROUP"
	}

	// Step 3: Insert DELETE entry into sync_outbox with payload
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
			$5::target_system,
			$6::jsonb,
			'PENDING'::outbox_status,
			0,
			5,
			NOW(),
			NOW(),
			NOW()
		)
		ON CONFLICT (resource_type, resource_id, operation, target_system) DO NOTHING`
	_, err = w.tx.Exec(w.ctx, insertOutboxQuery, kind, resourceUID, namespace, name, targetSystem, payloadJSON)
	if err != nil {
		return fmt.Errorf("failed to create DELETE entry in sync_outbox: %w", err)
	}
	return nil
}
