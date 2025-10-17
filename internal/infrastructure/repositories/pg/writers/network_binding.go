package writers

import (
	"context"
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
	for _, opt := range options {
		if syncOption, ok := opt.(ports.SyncOption); ok {
			syncOp = syncOption.Operation
			break
		}
	}

	// Handle scoped sync - delete existing resources in scope first (for non-DELETE operations)
	if !scope.IsEmpty() && syncOp != models.SyncOpDelete {
		if err := w.deleteNetworkBindingsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete network bindings in scope")
		}
	}

	// Handle operations based on sync operation
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
			// Initialize metadata fields if not set
			if networkBindings[i].Meta.UID == "" {
				networkBindings[i].Meta.TouchOnCreate()
			}

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

	conditionsJSON, err := json.Marshal(binding.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// ALWAYS INSERT new K8s metadata (to get new ResourceVersion)
	// PostgreSQL BIGSERIAL primary key only increments on INSERT, not UPDATE
	var resourceVersion int64
	metadataQuery := `
		INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
		VALUES ($1, $2, '{}', $3)
		RETURNING resource_version`
	err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to insert K8s metadata for network binding %s/%s", binding.Namespace, binding.Name)
	}

	// Update domain model with new ResourceVersion from DB
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
		binding.Namespace, // NetworkRef is in same namespace as binding
		binding.NetworkRef.Name,
		binding.Namespace, // AddressGroupRef is in same namespace as binding
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
			"namespace": binding.Namespace,
			"name":      binding.NetworkRef.Name,
		},
		{
			"type":      "AddressGroup",
			"namespace": binding.Namespace,
			"name":      binding.AddressGroupRef.Name,
		},
	}
	affectedResourcesJSON, err := json.Marshal(affectedResources)
	if err != nil {
		return errors.Wrap(err, "failed to marshal affected resources")
	}

	// Build payload
	payload := map[string]interface{}{
		"namespace":   binding.Namespace,
		"name":        binding.Name,
		"network_ref": binding.NetworkRef.Name,
		"ag_ref":      binding.AddressGroupRef.Name,
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
