package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// SyncAddressGroups implements hybrid sync strategy for address groups
func (w *Writer) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope, opts ...ports.Option) error {
	isConditionOnly := false
	for _, opt := range opts {
		if _, ok := opt.(ports.ConditionOnlyOperation); ok {
			isConditionOnly = true
			break
		}
	}

	if isConditionOnly {
		for i := range addressGroups {
			if err := w.updateAddressGroupConditionsOnly(ctx, addressGroups[i]); err != nil {
				return errors.Wrapf(err, "failed to update conditions for address group %s/%s", addressGroups[i].Namespace, addressGroups[i].Name)
			}
		}
		return nil
	}

	for i := range addressGroups {
		if addressGroups[i].Meta.UID == "" {
			addressGroups[i].Meta.TouchOnCreate()
		}

		if err := w.upsertAddressGroup(ctx, addressGroups[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert address group %s/%s", addressGroups[i].Namespace, addressGroups[i].Name)
		}
	}

	return nil
}

// upsertAddressGroup inserts or updates an address group with full K8s metadata support
func (w *Writer) upsertAddressGroup(ctx context.Context, ag models.AddressGroup) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(ag.Meta.Labels, ag.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	// Check if address group exists and get existing resource version + Networks field
	var existingResourceVersion sql.NullInt64
	var existingNetworks []byte
	existingQuery := `SELECT resource_version, networks FROM address_groups WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, existingQuery, ag.Namespace, ag.Name).Scan(&existingResourceVersion, &existingNetworks)
	if err != nil && err != sql.ErrNoRows {
		klog.V(4).InfoS("Failed to get existing AddressGroup fields", "namespace", ag.Namespace, "name", ag.Name, "error", err.Error())
	}

	// Ready will be set to True by Worker after successful SGROUP sync
	// Business Rule: Resources should NOT have Ready=True until synced to SGROUP
	conditions := ag.Meta.Conditions
	if !existingResourceVersion.Valid {
		// This is a new resource - force Pending status
		conditions = forcePendingSyncCondition(conditions)
		klog.V(4).InfoS("Forcing PendingSGROUPSync status for new AddressGroup",
			"namespace", ag.Namespace, "name", ag.Name)
	}

	conditionsJSON, err := json.Marshal(conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	var networksJSON []byte
	networks := ag.Networks
	if networks == nil {
		networks = []models.NetworkItem{}
	}

	var existingNetworksList []models.NetworkItem
	if len(existingNetworks) > 0 {
		_ = json.Unmarshal(existingNetworks, &existingNetworksList)
	}

	if len(networks) == 0 && len(existingNetworksList) > 0 {
		networksJSON = existingNetworks
	} else {
		networksJSON, err = json.Marshal(networks)
		if err != nil {
			return errors.Wrap(err, "failed to marshal networks")
		}
	}

	// Marshal Hosts field - user-managed, always use incoming value
	var hostsJSON []byte
	hosts := ag.Hosts
	if hosts == nil {
		hosts = []netguardv1beta1.NamespacedObjectReference{}
	}
	hostsJSON, err = json.Marshal(hosts)
	if err != nil {
		return errors.Wrap(err, "failed to marshal hosts")
	}

	// Migration 044: aggregated_hosts is now fully managed by database triggers
	// The trigger_update_aggregated_hosts_on_spec_change trigger automatically
	// calculates aggregated_hosts from spec.hosts + HostBindings.
	// No manual calculation needed in Go code.

	var resourceVersion int64
	if existingResourceVersion.Valid {
		// UPDATE existing K8s metadata
		metadataQuery := `
			UPDATE k8s_metadata
			SET labels = $1, annotations = $2, conditions = $3, updated_at = NOW()
			WHERE resource_version = $4
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, existingResourceVersion.Int64).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to update K8s metadata for address group %s/%s", ag.Namespace, ag.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
			VALUES ($1, $2, '{}', $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for address group %s/%s", ag.Namespace, ag.Name)
		}
	}

	// Upsert address group
	// Note: aggregated_hosts is calculated by database trigger after INSERT/UPDATE
	// We omit aggregated_hosts from the query and let the trigger handle it
	addressGroupQuery := `
		INSERT INTO address_groups (namespace, name, default_action, logs, trace, description, networks, hosts, resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (namespace, name) DO UPDATE SET
			default_action = $3,
			logs = $4,
			trace = $5,
			description = $6,
			hosts = $8,
			resource_version = $9`

	if err := w.exec(ctx, addressGroupQuery,
		ag.Namespace,
		ag.Name,
		string(ag.DefaultAction),
		ag.Logs,
		ag.Trace,
		"",
		networksJSON,
		hostsJSON,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert address group %s/%s", ag.Namespace, ag.Name)
	}

	// Outbox entries are produced by PostgreSQL trigger 'trg_address_group_upsert_outbox',
	// which uses the Kubernetes UID stored in k8s_metadata.

	return nil
}

// deleteAddressGroupsInScope deletes address groups that match the provided scope
func (w *Writer) deleteAddressGroupsInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "ag")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`DELETE FROM address_groups ag WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address groups in scope")
	}

	return nil
}

// deleteAddressGroupsByIdentifiers deletes specific address groups by their identifiers
//
// This method triggers the BEFORE DELETE trigger (migration 026: trigger_address_group_before_delete)
// which automatically handles:
// - Soft delete (UPDATE k8s_metadata SET deletion_timestamp = NOW())
// - DELETE outbox entry creation
// - Prevention of physical deletion (RETURN NULL) until Worker syncs to SGROUP
func (w *Writer) deleteAddressGroupsByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
	if len(identifiers) == 0 {
		return nil
	}

	// Build DELETE query with parameter placeholders
	var conditions []string
	var args []interface{}
	argIndex := 1

	for _, id := range identifiers {
		conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", argIndex, argIndex+1))
		args = append(args, id.Namespace, id.Name)
		argIndex += 2
	}

	// Execute DELETE FROM address_groups
	// This will trigger the BEFORE DELETE trigger which:
	// 1. Checks if deletion_timestamp is NULL (first delete attempt)
	// 2. If NULL: soft delete + create DELETE outbox entry + prevent physical deletion
	// 3. If NOT NULL: allow physical deletion (Worker already synced to SGROUP)
	deleteQuery := fmt.Sprintf(`
		DELETE FROM address_groups
		WHERE %s
	`, strings.Join(conditions, " OR "))

	result, err := w.tx.Exec(ctx, deleteQuery, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete address groups")
	}

	rowsAffected := result.RowsAffected()
	klog.V(4).InfoS("Executed DELETE for address groups (BEFORE DELETE trigger handles soft delete + outbox)",
		"requested", len(identifiers),
		"rows_affected", rowsAffected)

	// Note: rowsAffected will be 0 for first delete attempt (trigger prevents physical deletion)
	// and > 0 for second delete attempt after Worker synced to SGROUP

	return nil
}

// SyncAddressGroupBindings implements hybrid sync strategy for address group bindings
func (w *Writer) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope, opts ...ports.Option) error {
	// Extract sync operation from options
	syncOp := models.SyncOpUpsert // Default operation
	var conditionOnly bool
	for _, opt := range opts {
		if syncOption, ok := opt.(ports.SyncOption); ok {
			syncOp = syncOption.Operation
			break
		}
		if _, ok := opt.(ports.ConditionOnlyOperation); ok {
			conditionOnly = true
			break
		}
	}

	if conditionOnly {
		return w.updateAddressGroupBindingConditionsOnly(ctx, bindings)
	}

	switch syncOp {
	case models.SyncOpDelete:
		// Use soft delete (set deletion_timestamp) so triggers can process the change
		// and OutboxWorker can perform the physical removal later.
		var identifiers []models.ResourceIdentifier
		for _, binding := range bindings {
			identifiers = append(identifiers, binding.SelfRef.ResourceIdentifier)
		}
		if err := w.markAddressGroupBindingsForDeletion(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to mark address group bindings for deletion")
		}
	case models.SyncOpUpsert, models.SyncOpFullSync:
		for i := range bindings {
			if bindings[i].Meta.UID == "" {
				bindings[i].Meta.TouchOnCreate()
			}

			if err := w.upsertAddressGroupBinding(ctx, bindings[i]); err != nil {
				return errors.Wrapf(err, "failed to upsert address group binding %s/%s", bindings[i].Namespace, bindings[i].Name)
			}
		}
	default:
		return errors.New(fmt.Sprintf("unsupported sync operation: %v", syncOp))
	}

	return nil
}

// upsertAddressGroupBinding inserts or updates an address group binding
func (w *Writer) upsertAddressGroupBinding(ctx context.Context, binding models.AddressGroupBinding) error {
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(binding.Meta.Labels, binding.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(binding.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	var resourceVersion int64
	var uid string
	metadataQuery := `
		INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
		VALUES ($1, $2, '{}', $3)
		RETURNING resource_version, uid`
	err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion, &uid)
	if err != nil {
		return errors.Wrapf(err, "failed to insert K8s metadata for address group binding %s/%s", binding.Namespace, binding.Name)
	}

	binding.Meta.UID = uid
	binding.Meta.TouchOnWrite(strconv.FormatInt(resourceVersion, 10))

	// Then, upsert the address group binding using the NEW resource version
	bindingQuery := `
		INSERT INTO address_group_bindings (namespace, name, service_namespace, service_name, address_group_namespace, address_group_name, resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (namespace, name) DO UPDATE SET
			service_namespace = $3,
			service_name = $4,
			address_group_namespace = $5,
			address_group_name = $6,
			resource_version = $7`

	if err := w.exec(ctx, bindingQuery,
		binding.Namespace,
		binding.Name,
		binding.ServiceRef.Namespace,
		binding.ServiceRef.Name,
		binding.AddressGroupRef.Namespace,
		binding.AddressGroupRef.Name,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert address group binding %s/%s", binding.Namespace, binding.Name)
	}

	return nil
}

// deleteAddressGroupBindingsInScope deletes address group bindings that match the provided scope
func (w *Writer) deleteAddressGroupBindingsInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "agb")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM address_group_bindings agb WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address group bindings in scope")
	}

	return nil
}

// deleteAddressGroupBindingsByIdentifiers deletes specific address group bindings by their identifiers
func (w *Writer) deleteAddressGroupBindingsByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
	if len(identifiers) == 0 {
		return nil
	}

	// Build parameter placeholders and collect args
	var conditions []string
	var args []interface{}
	argIndex := 1

	for _, id := range identifiers {
		conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", argIndex, argIndex+1))
		args = append(args, id.Namespace, id.Name)
		argIndex += 2
	}

	query := fmt.Sprintf(`
		DELETE FROM address_group_bindings WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address group bindings by identifiers")
	}

	return nil
}

// markAddressGroupBindingsForDeletion marks address group bindings for deletion (soft delete)
// by setting deletion_timestamp in k8s_metadata.
// Physical deletion will happen via OutboxWorker after AFTER UPDATE trigger processes the change.
// This ensures the trigger can read binding details before they are physically removed.
func (w *Writer) markAddressGroupBindingsForDeletion(ctx context.Context, identifiers []models.ResourceIdentifier) error {
	if len(identifiers) == 0 {
		return nil
	}

	// Build parameter placeholders and collect args
	var conditions []string
	var args []interface{}
	argIndex := 1

	for _, id := range identifiers {
		conditions = append(conditions, fmt.Sprintf("(agb.namespace = $%d AND agb.name = $%d)", argIndex, argIndex+1))
		args = append(args, id.Namespace, id.Name)
		argIndex += 2
	}

	// Set deletion_timestamp in k8s_metadata for matching bindings
	// This triggers the AFTER UPDATE trigger (migration 066/072) which:
	// 1. Reads binding details (binding still exists at this point!)
	// 2. Updates Service.aggregated_address_groups
	// 3. Service UPDATE trigger creates SGROUP outbox entry
	// Physical deletion happens later via OutboxWorker
	markDeleteQuery := fmt.Sprintf(`
		UPDATE k8s_metadata m
		SET deletion_timestamp = NOW()
		FROM address_group_bindings agb
		WHERE agb.resource_version = m.resource_version
		  AND (%s)
		  AND m.deletion_timestamp IS NULL`, strings.Join(conditions, " OR "))

	_, err := w.tx.Exec(ctx, markDeleteQuery, args...)
	if err != nil {
		return errors.Wrap(err, "failed to mark address group bindings for deletion in k8s_metadata")
	}

	klog.InfoS("Marked AddressGroupBindings for deletion (soft delete)",
		"count", len(identifiers))

	return nil
}

// updateAddressGroupBindingConditionsOnly updates only conditions without creating Outbox entries
// Used by ConditionManager to update Ready/PendingSync status
func (w *Writer) updateAddressGroupBindingConditionsOnly(ctx context.Context, bindings []models.AddressGroupBinding) error {
	for i := range bindings {
		binding := &bindings[i]

		// Marshal conditions
		conditionsJSON, err := json.Marshal(binding.Meta.Conditions)
		if err != nil {
			return errors.Wrap(err, "failed to marshal conditions")
		}

		// Find existing resource_version
		var existingResourceVersion int64
		query := `SELECT resource_version FROM address_group_bindings WHERE namespace = $1 AND name = $2`
		err = w.tx.QueryRow(ctx, query, binding.Namespace, binding.Name).Scan(&existingResourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to find existing AddressGroupBinding %s/%s", binding.Namespace, binding.Name)
		}

		// Update only conditions in k8s_metadata (no new resource_version)
		updateQuery := `UPDATE k8s_metadata SET conditions = $1 WHERE resource_version = $2`
		if err := w.exec(ctx, updateQuery, conditionsJSON, existingResourceVersion); err != nil {
			return errors.Wrapf(err, "failed to update conditions for AddressGroupBinding %s/%s", binding.Namespace, binding.Name)
		}

		klog.V(4).InfoS("Updated AddressGroupBinding conditions only",
			"namespace", binding.Namespace,
			"name", binding.Name,
			"resource_version", existingResourceVersion)
	}

	return nil
}

// SyncAddressGroupPortMappings implements hybrid sync strategy for address group port mappings
func (w *Writer) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope, opts ...ports.Option) error {
	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteAddressGroupPortMappingsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete address group port mappings in scope")
		}
	}

	// Upsert all provided mappings
	for i := range mappings {
		if mappings[i].Meta.UID == "" {
			mappings[i].Meta.TouchOnCreate()
		}

		if err := w.upsertAddressGroupPortMapping(ctx, mappings[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert address group port mapping %s/%s", mappings[i].Namespace, mappings[i].Name)
		}
	}

	return nil
}

// upsertAddressGroupPortMapping inserts or updates an address group port mapping with complex map handling
func (w *Writer) upsertAddressGroupPortMapping(ctx context.Context, mapping models.AddressGroupPortMapping) error {
	// Marshal the complex AccessPorts map
	accessPortsJSON, err := w.marshalAccessPorts(mapping.AccessPorts)
	if err != nil {
		return errors.Wrap(err, "failed to marshal access ports")
	}

	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(mapping.Meta.Labels, mapping.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(mapping.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, check if address group port mapping exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM address_group_port_mappings WHERE namespace = $1 AND name = $2`
	_ = w.tx.QueryRow(ctx, existingQuery, mapping.Namespace, mapping.Name).Scan(&existingResourceVersion)

	var resourceVersion int64
	if existingResourceVersion.Valid {
		// UPDATE existing K8s metadata
		metadataQuery := `
			UPDATE k8s_metadata
			SET labels = $1, annotations = $2, conditions = $3, updated_at = NOW()
			WHERE resource_version = $4
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, existingResourceVersion.Int64).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to update K8s metadata for address group port mapping %s/%s", mapping.Namespace, mapping.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
			VALUES ($1, $2, '{}', $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for address group port mapping %s/%s", mapping.Namespace, mapping.Name)
		}
	}

	// Then, upsert the address group port mapping using the resource version
	portMappingQuery := `
		INSERT INTO address_group_port_mappings (namespace, name, access_ports, resource_version)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (namespace, name) DO UPDATE SET
			access_ports = $3,
			resource_version = $4`

	if err := w.exec(ctx, portMappingQuery,
		mapping.Namespace,
		mapping.Name,
		accessPortsJSON,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert address group port mapping %s/%s", mapping.Namespace, mapping.Name)
	}

	return nil
}

// deleteAddressGroupPortMappingsInScope deletes address group port mappings that match the provided scope
func (w *Writer) deleteAddressGroupPortMappingsInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "agpm")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM address_group_port_mappings agpm WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address group port mappings in scope")
	}

	return nil
}

// deleteAddressGroupPortMappingsByIdentifiers deletes specific address group port mappings by their identifiers
func (w *Writer) deleteAddressGroupPortMappingsByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
	if len(identifiers) == 0 {
		return nil
	}

	// Build parameter placeholders and collect args
	var conditions []string
	var args []interface{}
	argIndex := 1

	for _, id := range identifiers {
		conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", argIndex, argIndex+1))
		args = append(args, id.Namespace, id.Name)
		argIndex += 2
	}

	// First, mark objects as being deleted in k8s_metadata to prevent re-creation by ListWatch
	markDeleteQuery := `
		UPDATE k8s_metadata m
		SET deletion_timestamp = NOW()
		FROM address_group_port_mappings agpm
		WHERE agpm.resource_version = m.resource_version
		  AND (%s)
		  AND m.deletion_timestamp IS NULL`

	markQuery := fmt.Sprintf(markDeleteQuery, strings.Join(conditions, " OR "))
	_, err := w.tx.Exec(ctx, markQuery, args...)
	if err != nil {
		// Log but don't fail - deletion_timestamp is optional for now
		klog.V(4).InfoS("Failed to mark address group port mappings as deleting in k8s_metadata", "error", err.Error())
	}

	// Then delete from address_group_port_mappings table
	query := fmt.Sprintf(`
		DELETE FROM address_group_port_mappings WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address group port mappings by identifiers")
	}

	return nil
}

// SyncAddressGroupBindingPolicies implements hybrid sync strategy for address group binding policies
func (w *Writer) SyncAddressGroupBindingPolicies(ctx context.Context, policies []models.AddressGroupBindingPolicy, scope ports.Scope, opts ...ports.Option) error {
	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteAddressGroupBindingPoliciesInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete address group binding policies in scope")
		}
	}

	for i := range policies {
		if policies[i].Meta.UID == "" {
			policies[i].Meta.TouchOnCreate()
		}

		if err := w.upsertAddressGroupBindingPolicy(ctx, policies[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert address group binding policy %s/%s", policies[i].Namespace, policies[i].Name)
		}
	}

	return nil
}

// upsertAddressGroupBindingPolicy inserts or updates an address group binding policy
func (w *Writer) upsertAddressGroupBindingPolicy(ctx context.Context, policy models.AddressGroupBindingPolicy) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(policy.Meta.Labels, policy.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(policy.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, check if address group binding policy exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM address_group_binding_policies WHERE namespace = $1 AND name = $2`
	_ = w.tx.QueryRow(ctx, existingQuery, policy.Namespace, policy.Name).Scan(&existingResourceVersion)

	var resourceVersion int64
	if existingResourceVersion.Valid {
		// UPDATE existing K8s metadata
		metadataQuery := `
			UPDATE k8s_metadata
			SET labels = $1, annotations = $2, conditions = $3, updated_at = NOW()
			WHERE resource_version = $4
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, existingResourceVersion.Int64).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to update K8s metadata for address group binding policy %s/%s", policy.Namespace, policy.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
			VALUES ($1, $2, '{}', $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for address group binding policy %s/%s", policy.Namespace, policy.Name)
		}
	}

	addressGroupRefJSON, err := json.Marshal(policy.AddressGroupRef)
	if err != nil {
		return errors.Wrap(err, "failed to marshal address group reference")
	}

	serviceRefJSON, err := json.Marshal(policy.ServiceRef)
	if err != nil {
		return errors.Wrap(err, "failed to marshal service reference")
	}

	// Then, upsert the address group binding policy using the resource version
	policyQuery := `
		INSERT INTO address_group_binding_policies (namespace, name, address_group_ref, service_ref, resource_version)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (namespace, name) DO UPDATE SET
			address_group_ref = $3,
			service_ref = $4,
			resource_version = $5`

	if err := w.exec(ctx, policyQuery,
		policy.Namespace,
		policy.Name,
		addressGroupRefJSON,
		serviceRefJSON,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert address group binding policy %s/%s", policy.Namespace, policy.Name)
	}

	return nil
}

// deleteAddressGroupBindingPoliciesInScope deletes address group binding policies that match the provided scope
func (w *Writer) deleteAddressGroupBindingPoliciesInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "agbp")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM address_group_binding_policies agbp WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address group binding policies in scope")
	}

	return nil
}

// Delete methods by IDs
func (w *Writer) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.deleteAddressGroupsByIdentifiers(ctx, ids)
}

func (w *Writer) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.markAddressGroupBindingsForDeletion(ctx, ids)
}

func (w *Writer) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.deleteAddressGroupPortMappingsByIdentifiers(ctx, ids)
}

func (w *Writer) DeleteAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if len(ids) == 0 {
		return nil
	}

	// Build parameter placeholders and collect args
	var conditions []string
	var args []interface{}
	argIndex := 1

	for _, id := range ids {
		conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", argIndex, argIndex+1))
		args = append(args, id.Namespace, id.Name)
		argIndex += 2
	}

	// First, mark objects as being deleted in k8s_metadata to prevent re-creation by ListWatch
	markDeleteQuery := `
		UPDATE k8s_metadata m
		SET deletion_timestamp = NOW()
		FROM address_group_binding_policies agbp
		WHERE agbp.resource_version = m.resource_version
		  AND (%s)
		  AND m.deletion_timestamp IS NULL`

	markQuery := fmt.Sprintf(markDeleteQuery, strings.Join(conditions, " OR "))
	_, err := w.tx.Exec(ctx, markQuery, args...)
	if err != nil {
		// Log but don't fail - deletion_timestamp is optional for now
		klog.V(4).InfoS("Failed to mark address group binding policies as deleting in k8s_metadata", "error", err.Error())
	}

	// Then delete from address_group_binding_policies table
	query := fmt.Sprintf(`
		DELETE FROM address_group_binding_policies WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address group binding policies by identifiers")
	}

	return nil
}

// createAddressGroupOutboxEntry creates an outbox entry for AddressGroup resource
func (w *Writer) createAddressGroupOutboxEntry(ctx context.Context, ag models.AddressGroup) error {
	// Build payload
	payload := map[string]interface{}{
		"namespace":      ag.Namespace,
		"name":           ag.Name,
		"default_action": ag.DefaultAction,
		"logs":           ag.Logs,
		"trace":          ag.Trace,
		"networks":       ag.Networks,
		"hosts":          ag.Hosts,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal address group payload")
	}

	// Parse resource UUID from Meta.UID
	resourceUUID, err := uuid.Parse(ag.Meta.UID)
	if err != nil {
		return errors.Wrapf(err, "invalid address group UID: %s", ag.Meta.UID)
	}

	// Create outbox entry
	outboxEntry := &domain.OutboxEntry{
		ResourceType:      "AddressGroup",
		ResourceID:        resourceUUID,
		ResourceNamespace: ag.Namespace,
		ResourceName:      ag.Name,
		Operation:         domain.SyncOperationCreate,
		TargetSystem:      domain.TargetSystemSGROUP,
		Payload:           payloadJSON,
		Status:            domain.OutboxStatusPending,
		MaxRetries:        5,
	}

	// Use OutboxRepository with existing transaction
	outboxRepo := repositories.NewOutboxRepository(w.tx)
	if err := outboxRepo.Create(ctx, outboxEntry); err != nil {
		return errors.Wrap(err, "failed to persist outbox entry")
	}

	klog.V(4).InfoS("Created outbox entry for AddressGroup",
		"namespace", ag.Namespace,
		"name", ag.Name,
		"outbox_id", outboxEntry.ID)

	return nil
}

// Removed createAddressGroupDeleteOutboxEntry - no longer needed!
// DELETE outbox entries are now automatically created by BEFORE DELETE trigger
// (migration 026: trigger_address_group_before_delete)

func (w *Writer) updateAddressGroupConditionsOnly(ctx context.Context, ag models.AddressGroup) error {
	// ДИАГНОСТИКА: Начало обновления conditions
	klog.InfoS("[DIAG] updateAddressGroupConditionsOnly: ENTRY",
		"namespace", ag.Namespace,
		"name", ag.Name,
		"conditions_count", len(ag.Meta.Conditions))

	var resourceVersion int64
	getVersionQuery := `SELECT resource_version FROM address_groups WHERE namespace = $1 AND name = $2`

	klog.InfoS("[DIAG] updateAddressGroupConditionsOnly: querying resource_version",
		"namespace", ag.Namespace,
		"name", ag.Name,
		"query", getVersionQuery)

	err := w.tx.QueryRow(ctx, getVersionQuery, ag.Namespace, ag.Name).Scan(&resourceVersion)
	if err != nil {
		if err == sql.ErrNoRows {
			klog.ErrorS(err, "[DIAG] updateAddressGroupConditionsOnly: address group NOT FOUND",
				"namespace", ag.Namespace,
				"name", ag.Name)
			return errors.Errorf("address group %s/%s not found", ag.Namespace, ag.Name)
		}
		klog.ErrorS(err, "[DIAG] updateAddressGroupConditionsOnly: query FAILED",
			"namespace", ag.Namespace,
			"name", ag.Name)
		return errors.Wrapf(err, "failed to get resource_version for address group %s/%s", ag.Namespace, ag.Name)
	}

	klog.InfoS("[DIAG] updateAddressGroupConditionsOnly: resource_version retrieved",
		"namespace", ag.Namespace,
		"name", ag.Name,
		"resource_version", resourceVersion)

	conditionsJSON, err := json.Marshal(ag.Meta.Conditions)
	if err != nil {
		klog.ErrorS(err, "[DIAG] updateAddressGroupConditionsOnly: marshal FAILED",
			"namespace", ag.Namespace,
			"name", ag.Name)
		return errors.Wrap(err, "failed to marshal conditions")
	}

	klog.InfoS("[DIAG] updateAddressGroupConditionsOnly: conditions marshaled",
		"namespace", ag.Namespace,
		"name", ag.Name,
		"json_length", len(conditionsJSON))

	updateQuery := `
		UPDATE k8s_metadata
		SET conditions = $1, updated_at = NOW()
		WHERE resource_version = $2`

	klog.InfoS("[DIAG] updateAddressGroupConditionsOnly: executing UPDATE k8s_metadata",
		"namespace", ag.Namespace,
		"name", ag.Name,
		"resource_version", resourceVersion,
		"query", updateQuery)

	result, err := w.tx.Exec(ctx, updateQuery, conditionsJSON, resourceVersion)
	if err != nil {
		klog.ErrorS(err, "[DIAG] updateAddressGroupConditionsOnly: UPDATE FAILED",
			"namespace", ag.Namespace,
			"name", ag.Name,
			"resource_version", resourceVersion)
		return errors.Wrapf(err, "failed to update conditions for address group %s/%s", ag.Namespace, ag.Name)
	}

	rowsAffected := result.RowsAffected()
	klog.InfoS("[DIAG] updateAddressGroupConditionsOnly: UPDATE SUCCESS",
		"namespace", ag.Namespace,
		"name", ag.Name,
		"resource_version", resourceVersion,
		"rows_affected", rowsAffected)

	if rowsAffected == 0 {
		klog.ErrorS(nil, "[DIAG] updateAddressGroupConditionsOnly: WARNING - no rows affected",
			"namespace", ag.Namespace,
			"name", ag.Name,
			"resource_version", resourceVersion)
	}

	return nil
}
