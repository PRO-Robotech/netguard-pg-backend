package worker

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// BindingInfo holds metadata about a binding resource
type BindingInfo struct {
	Namespace         string
	Name              string
	ResourceVersion   int64
	DeletionTimestamp *time.Time
}

// findHostBindingsForHost finds all HostBindings that reference a specific Host
// Used in Case 2: DELETE Host → CASCADE HostBinding
func (w *OutboxWorker) findHostBindingsForHost(
	ctx context.Context,
	hostNamespace string,
	hostName string,
) ([]BindingInfo, error) {
	query := `
		SELECT
			hb.namespace,
			hb.name,
			hb.resource_version,
			m.deletion_timestamp
		FROM host_bindings hb
		JOIN k8s_metadata m ON m.resource_version = hb.resource_version
		WHERE hb.host_namespace = $1 AND hb.host_name = $2
	`

	rows, err := w.pool.Query(ctx, query, hostNamespace, hostName)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var bindings []BindingInfo
	for rows.Next() {
		var binding BindingInfo
		if err := rows.Scan(
			&binding.Namespace,
			&binding.Name,
			&binding.ResourceVersion,
			&binding.DeletionTimestamp,
		); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		bindings = append(bindings, binding)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return bindings, nil
}

// findNetworkBindingsForNetwork finds all NetworkBindings that reference a specific Network
// Used in Case 2: DELETE Network → CASCADE NetworkBinding
func (w *OutboxWorker) findNetworkBindingsForNetwork(
	ctx context.Context,
	networkNamespace string,
	networkName string,
) ([]BindingInfo, error) {
	query := `
		SELECT
			nb.namespace,
			nb.name,
			nb.resource_version,
			m.deletion_timestamp
		FROM network_bindings nb
		JOIN k8s_metadata m ON m.resource_version = nb.resource_version
		WHERE nb.network_namespace = $1 AND nb.network_name = $2
	`

	rows, err := w.pool.Query(ctx, query, networkNamespace, networkName)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var bindings []BindingInfo
	for rows.Next() {
		var binding BindingInfo
		if err := rows.Scan(
			&binding.Namespace,
			&binding.Name,
			&binding.ResourceVersion,
			&binding.DeletionTimestamp,
		); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		bindings = append(bindings, binding)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return bindings, nil
}

// findHostBindingsForAddressGroup finds all HostBindings that reference a specific AddressGroup
// Used in Case 4: DELETE AddressGroup → CASCADE all HostBindings
func (w *OutboxWorker) findHostBindingsForAddressGroup(
	ctx context.Context,
	agNamespace string,
	agName string,
) ([]BindingInfo, error) {
	query := `
		SELECT
			hb.namespace,
			hb.name,
			hb.resource_version,
			m.deletion_timestamp
		FROM host_bindings hb
		JOIN k8s_metadata m ON m.resource_version = hb.resource_version
		WHERE hb.address_group_namespace = $1 AND hb.address_group_name = $2
	`

	rows, err := w.pool.Query(ctx, query, agNamespace, agName)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var bindings []BindingInfo
	for rows.Next() {
		var binding BindingInfo
		if err := rows.Scan(
			&binding.Namespace,
			&binding.Name,
			&binding.ResourceVersion,
			&binding.DeletionTimestamp,
		); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		bindings = append(bindings, binding)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return bindings, nil
}

// findNetworkBindingsForAddressGroup finds all NetworkBindings that reference a specific AddressGroup
// Used in Case 4: DELETE AddressGroup → CASCADE all NetworkBindings
func (w *OutboxWorker) findNetworkBindingsForAddressGroup(
	ctx context.Context,
	agNamespace string,
	agName string,
) ([]BindingInfo, error) {
	query := `
		SELECT
			nb.namespace,
			nb.name,
			nb.resource_version,
			m.deletion_timestamp
		FROM network_bindings nb
		JOIN k8s_metadata m ON m.resource_version = nb.resource_version
		WHERE nb.address_group_namespace = $1 AND nb.address_group_name = $2
	`

	rows, err := w.pool.Query(ctx, query, agNamespace, agName)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var bindings []BindingInfo
	for rows.Next() {
		var binding BindingInfo
		if err := rows.Scan(
			&binding.Namespace,
			&binding.Name,
			&binding.ResourceVersion,
			&binding.DeletionTimestamp,
		); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		bindings = append(bindings, binding)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return bindings, nil
}

// markBindingForDeletion sets deletion_timestamp on a binding resource
// This triggers the soft delete flow (Migration 044 triggers)
// Idempotent - safe to call multiple times
func (w *OutboxWorker) markBindingForDeletion(
	ctx context.Context,
	binding BindingInfo,
) error {
	// Skip if already marked
	if binding.DeletionTimestamp != nil {
		w.logger.Debug("binding already marked for deletion",
			zap.String("namespace", binding.Namespace),
			zap.String("name", binding.Name))
		return nil
	}

	w.logger.Info("marking binding for deletion (soft delete)",
		zap.String("namespace", binding.Namespace),
		zap.String("name", binding.Name),
		zap.Int64("resource_version", binding.ResourceVersion))

	query := `
		UPDATE k8s_metadata
		SET deletion_timestamp = NOW()
		WHERE resource_version = $1
		AND deletion_timestamp IS NULL
	`

	result, err := w.pool.Exec(ctx, query, binding.ResourceVersion)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	rowsAffected := result.RowsAffected()
	w.logger.Debug("binding marked for deletion",
		zap.String("namespace", binding.Namespace),
		zap.String("name", binding.Name),
		zap.Int64("rows_affected", rowsAffected))

	return nil
}

// checkAllHostBindingsDeleted checks if all specified HostBindings have been physically deleted
// Returns true if ALL bindings are gone from database
func (w *OutboxWorker) checkAllHostBindingsDeleted(
	ctx context.Context,
	bindings []BindingInfo,
) (bool, error) {
	if len(bindings) == 0 {
		return true, nil
	}

	// Check each binding
	for _, binding := range bindings {
		var count int
		query := `SELECT COUNT(*) FROM host_bindings WHERE namespace = $1 AND name = $2`
		err := w.pool.QueryRow(ctx, query, binding.Namespace, binding.Name).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("query failed for %s/%s: %w", binding.Namespace, binding.Name, err)
		}

		if count > 0 {
			// Binding still exists - not yet deleted
			w.logger.Debug("HostBinding still exists (not physically deleted)",
				zap.String("namespace", binding.Namespace),
				zap.String("name", binding.Name))
			return false, nil
		}
	}

	// All bindings physically deleted!
	w.logger.Info("all HostBindings physically deleted",
		zap.Int("count", len(bindings)))
	return true, nil
}

// checkAllNetworkBindingsDeleted checks if all specified NetworkBindings have been physically deleted
// Returns true if ALL bindings are gone from database
func (w *OutboxWorker) checkAllNetworkBindingsDeleted(
	ctx context.Context,
	bindings []BindingInfo,
) (bool, error) {
	if len(bindings) == 0 {
		return true, nil
	}

	// Check each binding
	for _, binding := range bindings {
		var count int
		query := `SELECT COUNT(*) FROM network_bindings WHERE namespace = $1 AND name = $2`
		err := w.pool.QueryRow(ctx, query, binding.Namespace, binding.Name).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("query failed for %s/%s: %w", binding.Namespace, binding.Name, err)
		}

		if count > 0 {
			// Binding still exists - not yet deleted
			w.logger.Debug("NetworkBinding still exists (not physically deleted)",
				zap.String("namespace", binding.Namespace),
				zap.String("name", binding.Name))
			return false, nil
		}
	}

	// All bindings physically deleted!
	w.logger.Info("all NetworkBindings physically deleted",
		zap.Int("count", len(bindings)))
	return true, nil
}

// findAddressGroupBindingsForAddressGroup finds all AddressGroupBindings that reference a specific AddressGroup
// Used in Case 4: DELETE AddressGroup → CASCADE AddressGroupBinding
func (w *OutboxWorker) findAddressGroupBindingsForAddressGroup(
	ctx context.Context,
	agNamespace string,
	agName string,
) ([]BindingInfo, error) {
	query := `
		SELECT
			agb.namespace,
			agb.name,
			agb.resource_version,
			m.deletion_timestamp
		FROM address_group_bindings agb
		JOIN k8s_metadata m ON m.resource_version = agb.resource_version
		WHERE agb.address_group_namespace = $1 AND agb.address_group_name = $2
	`

	rows, err := w.pool.Query(ctx, query, agNamespace, agName)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var bindings []BindingInfo
	for rows.Next() {
		var binding BindingInfo
		if err := rows.Scan(
			&binding.Namespace,
			&binding.Name,
			&binding.ResourceVersion,
			&binding.DeletionTimestamp,
		); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		bindings = append(bindings, binding)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return bindings, nil
}

// findAddressGroupBindingsForService finds all AddressGroupBindings that reference a specific Service
// Used in Case 2: DELETE Service → CASCADE AddressGroupBinding
func (w *OutboxWorker) findAddressGroupBindingsForService(
	ctx context.Context,
	serviceNamespace string,
	serviceName string,
) ([]BindingInfo, error) {
	query := `
		SELECT
			agb.namespace,
			agb.name,
			agb.resource_version,
			m.deletion_timestamp
		FROM address_group_bindings agb
		JOIN k8s_metadata m ON m.resource_version = agb.resource_version
		WHERE agb.service_namespace = $1 AND agb.service_name = $2
	`

	rows, err := w.pool.Query(ctx, query, serviceNamespace, serviceName)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var bindings []BindingInfo
	for rows.Next() {
		var binding BindingInfo
		if err := rows.Scan(
			&binding.Namespace,
			&binding.Name,
			&binding.ResourceVersion,
			&binding.DeletionTimestamp,
		); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		bindings = append(bindings, binding)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return bindings, nil
}

// checkAllAddressGroupBindingsDeleted checks if all specified AddressGroupBindings have been physically deleted
// Returns true if ALL bindings are gone from database
func (w *OutboxWorker) checkAllAddressGroupBindingsDeleted(
	ctx context.Context,
	bindings []BindingInfo,
) (bool, error) {
	if len(bindings) == 0 {
		return true, nil
	}

	// Check each binding
	for _, binding := range bindings {
		var count int
		query := `SELECT COUNT(*) FROM address_group_bindings WHERE namespace = $1 AND name = $2`
		err := w.pool.QueryRow(ctx, query, binding.Namespace, binding.Name).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("query failed for %s/%s: %w", binding.Namespace, binding.Name, err)
		}

		if count > 0 {
			// Binding still exists - not yet deleted
			w.logger.Debug("AddressGroupBinding still exists (not physically deleted)",
				zap.String("namespace", binding.Namespace),
				zap.String("name", binding.Name))
			return false, nil
		}
	}

	// All bindings physically deleted!
	w.logger.Info("all AddressGroupBindings physically deleted",
		zap.Int("count", len(bindings)))
	return true, nil
}
