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

// SyncHostBindings syncs host bindings to PostgreSQL with K8s metadata support
func (w *Writer) SyncHostBindings(ctx context.Context, hostBindings []models.HostBinding, scope ports.Scope, options ...ports.Option) error {
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
		if err := w.deleteHostBindingsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete host bindings in scope")
		}
	}

	// Handle operations based on sync operation
	switch syncOp {
	case models.SyncOpDelete:
		// For DELETE operations, delete the specific bindings
		var identifiers []models.ResourceIdentifier
		for _, binding := range hostBindings {
			identifiers = append(identifiers, models.ResourceIdentifier{
				Namespace: binding.Namespace,
				Name:      binding.Name,
			})
		}
		if err := w.DeleteHostBindingsByIDs(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to delete host bindings")
		}
	case models.SyncOpUpsert, models.SyncOpFullSync:
		// For UPSERT/FULLSYNC operations, upsert all provided bindings
		for i := range hostBindings {
			// Initialize metadata fields if not set
			if hostBindings[i].Meta.UID == "" {
				hostBindings[i].Meta.TouchOnCreate()
			}

			if err := w.upsertHostBinding(ctx, &hostBindings[i]); err != nil {
				// Check for unique constraint violation (one binding per host)
				if isUniqueViolation(err, "host_bindings_host_namespace_host_name_key") {
					return errors.Errorf("host %s/%s is already bound to another address group", hostBindings[i].HostRef.Namespace, hostBindings[i].HostRef.Name)
				}
				return errors.Wrapf(err, "failed to upsert host binding %s/%s", hostBindings[i].Namespace, hostBindings[i].Name)
			}
		}
	default:
		return errors.Errorf("unsupported sync operation: %v", syncOp)
	}

	return nil
}

// upsertHostBinding inserts or updates a host binding with K8s metadata
func (w *Writer) upsertHostBinding(ctx context.Context, hostBinding *models.HostBinding) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(hostBinding.Meta.Labels, hostBinding.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(hostBinding.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// ALWAYS INSERT new K8s metadata (to get new ResourceVersion)
	// PostgreSQL BIGSERIAL primary key only increments on INSERT, not UPDATE
	var resourceVersion int64
	metadataQuery := `
		INSERT INTO k8s_metadata (labels, annotations, conditions)
		VALUES ($1, $2, $3)
		RETURNING resource_version`
	err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to insert K8s metadata for host binding %s/%s", hostBinding.Namespace, hostBinding.Name)
	}

	// Update domain model with new ResourceVersion from DB
	hostBinding.Meta.TouchOnWrite(strconv.FormatInt(resourceVersion, 10))

	// UPSERT host binding record with NEW resource version
	hostBindingQuery := `
		INSERT INTO host_bindings (
			namespace, name,
			host_namespace, host_name,
			address_group_namespace, address_group_name,
			resource_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (namespace, name) DO UPDATE SET
			host_namespace = EXCLUDED.host_namespace,
			host_name = EXCLUDED.host_name,
			address_group_namespace = EXCLUDED.address_group_namespace,
			address_group_name = EXCLUDED.address_group_name,
			resource_version = EXCLUDED.resource_version`

	_, err = w.tx.Exec(ctx, hostBindingQuery,
		hostBinding.Namespace, hostBinding.Name,
		hostBinding.HostRef.Namespace, hostBinding.HostRef.Name,
		hostBinding.AddressGroupRef.Namespace, hostBinding.AddressGroupRef.Name,
		resourceVersion,
	)
	if err != nil {
		return errors.Wrapf(err, "failed to upsert host binding %s/%s", hostBinding.Namespace, hostBinding.Name)
	}

	if err := w.createHostBindingOutboxEntry(ctx, hostBinding); err != nil {
		return errors.Wrap(err, "failed to create outbox entry for host binding")
	}

	return nil
}

// deleteHostBindingsInScope deletes all host bindings within the given scope
func (w *Writer) deleteHostBindingsInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	switch s := scope.(type) {
	case ports.ResourceIdentifierScope:
		if s.IsEmpty() {
			return nil
		}

		// Build IN clause for (namespace, name) pairs
		var values []string
		var args []interface{}
		argIndex := 1

		for _, id := range s.Identifiers {
			values = append(values, fmt.Sprintf("($%d, $%d)", argIndex, argIndex+1))
			args = append(args, id.Namespace, id.Name)
			argIndex += 2
		}

		query := fmt.Sprintf(`DELETE FROM host_bindings WHERE (namespace, name) IN (%s)`, strings.Join(values, ","))
		_, err := w.tx.Exec(ctx, query, args...)
		return errors.Wrap(err, "failed to delete host bindings by resource identifiers")

	default:
		return errors.Errorf("unsupported scope type for host bindings deletion: %T", scope)
	}
}

// DeleteHostBindingsByIDs deletes host bindings by their resource identifiers
func (w *Writer) DeleteHostBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, options ...ports.Option) error {
	if len(ids) == 0 {
		return nil
	}

	// Build IN clause for (namespace, name) pairs
	var values []string
	var args []interface{}
	argIndex := 1

	for _, id := range ids {
		values = append(values, fmt.Sprintf("($%d, $%d)", argIndex, argIndex+1))
		args = append(args, id.Namespace, id.Name)
		argIndex += 2
	}

	// Execute DELETE - trigger will intercept and apply Sync-First strategy
	query := fmt.Sprintf(`DELETE FROM host_bindings WHERE (namespace, name) IN (%s)`, strings.Join(values, ","))
	_, err := w.tx.Exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete host bindings by IDs")
	}

	return nil
}

// createHostBindingOutboxEntry creates an outbox entry for HostBinding resource (PROCESS RESOURCE)
func (w *Writer) createHostBindingOutboxEntry(ctx context.Context, binding *models.HostBinding) error {
	affectedResources := []map[string]string{
		{
			"type":      "Host",
			"namespace": binding.HostRef.Namespace,
			"name":      binding.HostRef.Name,
		},
		{
			"type":      "AddressGroup",
			"namespace": binding.AddressGroupRef.Namespace,
			"name":      binding.AddressGroupRef.Name,
		},
	}
	affectedResourcesJSON, err := json.Marshal(affectedResources)
	if err != nil {
		return errors.Wrap(err, "failed to marshal affected resources")
	}

	// Build payload
	payload := map[string]interface{}{
		"namespace": binding.Namespace,
		"name":      binding.Name,
		"host_ref":  binding.HostRef.Name,
		"ag_ref":    binding.AddressGroupRef.Name,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal host binding payload")
	}

	// Parse resource UUID from Meta.UID
	resourceUUID, err := uuid.Parse(binding.Meta.UID)
	if err != nil {
		return errors.Wrapf(err, "invalid host binding UID: %s", binding.Meta.UID)
	}

	// Create outbox entry
	outboxEntry := &domain.OutboxEntry{
		ResourceType:      "HostBinding",
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

	klog.V(4).InfoS("Created outbox entry for HostBinding",
		"namespace", binding.Namespace,
		"name", binding.Name,
		"outbox_id", outboxEntry.ID,
		"affected_resources", len(affectedResources))

	return nil
}
