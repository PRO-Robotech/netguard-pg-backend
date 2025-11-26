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
)

// SyncNetworkBindings syncs network bindings to PostgreSQL with K8s metadata support
func (w *Writer) SyncNetworkBindings(ctx context.Context, networkBindings []models.NetworkBinding, scope ports.Scope, options ...ports.Option) error {
	// Extract sync operation from options
	syncOp := models.SyncOpUpsert // Default operation
	var conditionOnly bool
	for _, opt := range options {
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
		return w.updateNetworkBindingConditionsOnly(ctx, networkBindings)
	}

	switch syncOp {
	case models.SyncOpDelete:
		// For DELETE operations, delete the specific bindings
		var identifiers []models.ResourceIdentifier
		for _, binding := range networkBindings {
			identifiers = append(identifiers, models.ResourceIdentifier{
				Namespace: binding.Namespace,
				Name:      binding.Name,
			})
		}
		if err := w.DeleteNetworkBindingsByIDs(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to delete network bindings")
		}
	case models.SyncOpUpsert, models.SyncOpFullSync:
		// For UPSERT/FULLSYNC operations, upsert all provided network bindings
		for i := range networkBindings {
			// Don't call TouchOnCreate() here - let upsertNetworkBinding handle UID generation
			// This ensures UID comes from database and matches Outbox entry

			if err := w.upsertNetworkBinding(ctx, &networkBindings[i]); err != nil {
				return errors.Wrapf(err, "failed to upsert network binding %s/%s", networkBindings[i].Namespace, networkBindings[i].Name)
			}
		}
	}

	return nil
}

// upsertNetworkBinding inserts or updates a network binding with full K8s metadata support
func (w *Writer) upsertNetworkBinding(ctx context.Context, binding *models.NetworkBinding) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(binding.Meta.Labels, binding.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM network_bindings WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, existingQuery, binding.Namespace, binding.Name).Scan(&existingResourceVersion)
	if err != nil && err != sql.ErrNoRows {
		return errors.Wrapf(err, "failed to check existing network binding %s/%s", binding.Namespace, binding.Name)
	}

	conditionsJSON, err := w.prepareConditionsJSON(ctx, existingResourceVersion, binding.Meta.Conditions, conditionMergeOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to prepare merged conditions")
	}

	var resourceVersion int64
	var uid string
	metadataQuery := `
		INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
		VALUES ($1, $2, '{}', $3)
		RETURNING resource_version, uid`
	err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion, &uid)
	if err != nil {
		return errors.Wrapf(err, "failed to insert K8s metadata for network binding %s/%s", binding.Namespace, binding.Name)
	}

	binding.Meta.UID = uid
	binding.Meta.TouchOnWrite(strconv.FormatInt(resourceVersion, 10))

	// Then, upsert the network binding using the NEW resource version
	bindingQuery := `
		INSERT INTO network_bindings (namespace, name, network_namespace, network_name, address_group_namespace, address_group_name, resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (namespace, name) DO UPDATE SET
			network_namespace = $3,
			network_name = $4,
			address_group_namespace = $5,
			address_group_name = $6,
			resource_version = $7`

	if err := w.exec(ctx, bindingQuery,
		binding.Namespace,
		binding.Name,
		binding.NetworkRef.Namespace,
		binding.NetworkRef.Name,
		binding.AddressGroupRef.Namespace,
		binding.AddressGroupRef.Name,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert network binding %s/%s", binding.Namespace, binding.Name)
	}

	if err := w.createNetworkBindingOutboxEntry(ctx, binding); err != nil {
		return errors.Wrap(err, "failed to create outbox entry for network binding")
	}

	return nil
}

// deleteNetworkBindingsInScope deletes network bindings that match the provided scope
func (w *Writer) deleteNetworkBindingsInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "nb")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM network_bindings nb WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete network bindings in scope")
	}

	return nil
}

// DeleteNetworkBindingsByIDs deletes network bindings by their identifiers
func (w *Writer) DeleteNetworkBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
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

	// Execute DELETE - trigger will intercept and apply Sync-First strategy
	query := fmt.Sprintf(`
		DELETE FROM network_bindings WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete network bindings by identifiers")
	}

	return nil
}

// createNetworkBindingOutboxEntry creates an outbox entry for NetworkBinding resource (PROCESS RESOURCE)
func (w *Writer) createNetworkBindingOutboxEntry(ctx context.Context, binding *models.NetworkBinding) error {
	affectedResources := []map[string]string{
		{
			"type":      "Network",
			"namespace": binding.NetworkRef.Namespace,
			"name":      binding.NetworkRef.Name,
		},
	}
	affectedResourcesJSON, err := json.Marshal(affectedResources)
	if err != nil {
		return errors.Wrap(err, "failed to marshal affected resources")
	}

	// Build payload
	payload := map[string]interface{}{
		"namespace":         binding.Namespace,
		"name":              binding.Name,
		"network_ref":       binding.NetworkRef.Name,
		"network_namespace": binding.NetworkRef.Namespace,
		"ag_ref":            binding.AddressGroupRef.Name,
		"ag_namespace":      binding.AddressGroupRef.Namespace,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal network binding payload")
	}

	// Parse resource UUID from Meta.UID
	resourceUUID, err := uuid.Parse(binding.Meta.UID)
	if err != nil {
		return errors.Wrapf(err, "invalid network binding UID: %s", binding.Meta.UID)
	}

	// Create outbox entry
	outboxEntry := &domain.OutboxEntry{
		ResourceType:      "NetworkBinding",
		ResourceID:        resourceUUID,
		ResourceNamespace: binding.Namespace,
		ResourceName:      binding.Name,
		Operation:         domain.SyncOperationCreate,
		TargetSystem:      domain.TargetSystemInternal,
		Payload:           payloadJSON,
		AffectsResources:  affectedResourcesJSON,
		Status:            domain.OutboxStatusPending,
		MaxRetries:        5,
	}

	// Use OutboxRepository with existing transaction
	outboxRepo := repositories.NewOutboxRepository(w.tx)
	if err := outboxRepo.Create(ctx, outboxEntry); err != nil {
		return errors.Wrap(err, "failed to persist outbox entry")
	}

	klog.V(4).InfoS("Created outbox entry for NetworkBinding",
		"namespace", binding.Namespace,
		"name", binding.Name,
		"outbox_id", outboxEntry.ID,
		"affected_resources", len(affectedResources))

	return nil
}

// updateNetworkBindingConditionsOnly updates only conditions without creating Outbox entries
// Used by ConditionManager to update Ready/PendingSync status
func (w *Writer) updateNetworkBindingConditionsOnly(ctx context.Context, bindings []models.NetworkBinding) error {
	for i := range bindings {
		binding := &bindings[i]

		// Find existing resource_version
		var existingResourceVersion int64
		query := `SELECT resource_version FROM network_bindings WHERE namespace = $1 AND name = $2`
		err := w.tx.QueryRow(ctx, query, binding.Namespace, binding.Name).Scan(&existingResourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to find existing NetworkBinding %s/%s", binding.Namespace, binding.Name)
		}

		mergedVersion := sql.NullInt64{Int64: existingResourceVersion, Valid: true}

		conditionsJSON, err := w.prepareConditionsJSON(ctx, mergedVersion, binding.Meta.Conditions, conditionMergeOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to prepare merged conditions")
		}

		// Update only conditions in k8s_metadata (no new resource_version)
		updateQuery := `UPDATE k8s_metadata SET conditions = $1 WHERE resource_version = $2`
		if err := w.exec(ctx, updateQuery, conditionsJSON, existingResourceVersion); err != nil {
			return errors.Wrapf(err, "failed to update conditions for NetworkBinding %s/%s", binding.Namespace, binding.Name)
		}

		klog.V(4).InfoS("Updated NetworkBinding conditions only",
			"namespace", binding.Namespace,
			"name", binding.Name,
			"resource_version", existingResourceVersion)
	}

	return nil
}
