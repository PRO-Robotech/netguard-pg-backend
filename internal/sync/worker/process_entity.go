package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/domain/registry"
	v1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/syncers"
	"netguard-pg-backend/internal/sync/types"
)

// processEntityResource processes entity resources (Host, AddressGroup, Network, Service)
// This implements the Sync-First strategy with dependency checking:
// 1. Load resource from DB (current state, not snapshot)
// 2. Apply delta if present
// 2.5. Check entity dependencies (embedded references must be Ready)
// 3. Sync to SGROUP (only if all dependencies ready)
// 4. ONLY after successful sync: Update resource ready=true + delete outbox entry
func (w *OutboxWorker) processEntityResource(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	// P0-3: Record processing start time for metrics
	startTime := time.Now()
	operation := string(item.Operation)

	w.logger.Info("processing entity resource",
		zap.String("resource_type", item.ResourceType),
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName),
		zap.String("operation", operation))

	// Integration Point #1: Update PendingSync when sync starts
	if err := w.updatePendingSyncSyncing(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName, item.Attempts+1); err != nil {
		w.logger.Warn("failed to update PendingSync condition at sync start", zap.Error(err))
	}

	// BUG-004 FIX: For DELETE operations, resource is already deleted from DB
	// We must use data from outbox.payload instead of loading from DB
	var resource interface{}
	var err error

	if item.Operation == domain.SyncOperationDelete {
		// Step 1 (DELETE): Reconstruct resource from outbox payload
		// The payload contains minimal data needed for DELETE (namespace, name, UUID/UID)
		w.logger.Debug("DELETE operation detected, reconstructing resource from payload",
			zap.String("resource_type", item.ResourceType),
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))

		resource, err = w.reconstructResourceFromPayload(item)
		if err != nil {
			// P0-3: Record failure metric for payload reconstruction error
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "payload_reconstruction_error", duration)

			return fmt.Errorf("failed to reconstruct resource from payload: %w", err)
		}
	} else {
		// Step 1 (CREATE/UPDATE): Load resource from DB (current state, not Outbox snapshot)
		// FIXED (P0-2): Now loads by namespace+name instead of comparing UIDs
		resource, err = w.loadEntityResource(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ports.ErrNotFound) {
				// Resource was deleted - this is a permanent error
				w.logger.Warn("resource not found (deleted)",
					zap.String("resource_type", item.ResourceType),
					zap.String("namespace", item.ResourceNamespace),
					zap.String("name", item.ResourceName))

				// P0-3: Record failure metric for resource not found
				duration := time.Since(startTime)
				RecordProcessingFailure(item.ResourceType, operation, "resource_not_found", duration)

				// Return wrapped ErrResourceDeleted for proper error handling
				return fmt.Errorf("%w: %w", ErrResourceDeleted, err)
			}

			// P0-3: Record failure metric for load error
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "load_error", duration)

			return fmt.Errorf("failed to load resource: %w", err)
		}
	}

	// Step 2: Apply delta if present (prevents lost updates)
	if item.Delta != nil && len(item.Delta) > 0 {
		if err := applyDelta(resource, item.Operation, item.Delta); err != nil {
			w.logger.Error("failed to apply delta",
				zap.String("resource_type", item.ResourceType),
				zap.Error(err))

			// P0-3: Record failure metric for delta application error
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "delta_error", duration)

			return fmt.Errorf("failed to apply delta: %w", err)
		}
		w.logger.Debug("applied delta",
			zap.String("resource_type", item.ResourceType),
			zap.String("operation", operation))
	}

	// Step 2.5: Check entity dependencies (embedded references)
	if item.Operation != domain.SyncOperationDelete {
		allReady, missingDeps, err := w.checkEntityDependencies(ctx, item.ResourceType, resource)
		if err != nil {
			w.logger.Error("failed to check entity dependencies",
				zap.String("resource_type", item.ResourceType),
				zap.Error(err))

			// P0-3: Record failure metric for dependency check error
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "dependency_check_error", duration)

			return fmt.Errorf("failed to check entity dependencies: %w", err)
		}

		if !allReady {
			for _, dep := range missingDeps {
				w.logger.Warn("entity dependency not ready",
					zap.String("resource_type", item.ResourceType),
					zap.String("namespace", item.ResourceNamespace),
					zap.String("name", item.ResourceName),
					zap.String("dep_type", dep.Type),
					zap.String("dep_namespace", dep.Namespace),
					zap.String("dep_name", dep.Name),
					zap.String("dep_reason", dep.Reason))
			}

			if item.ResourceType == string(registry.TypeHost) && len(missingDeps) == 1 && missingDeps[0].Type == string(registry.TypeAddressGroup) {
				if err := w.updatePendingSyncWaitingAddressGroup(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName,
					missingDeps[0].Namespace, missingDeps[0].Name); err != nil {
					w.logger.Warn("failed to update PendingSync condition for AddressGroup dependency", zap.Error(err))
				}
			} else {
				// General entity dependencies (AddressGroup→Host, Network→AddressGroup, Service→AddressGroup)
				if err := w.updatePendingSyncWaitingEntityDeps(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName,
					len(missingDeps), missingDeps[0].Type, missingDeps[0].Name); err != nil {
					w.logger.Warn("failed to update PendingSync condition for entity dependencies", zap.Error(err))
				}
			}

			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "missing_dependencies", duration)

			return fmt.Errorf("waiting for %d dependencies to be Ready (e.g., %s/%s)",
				len(missingDeps),
				missingDeps[0].Type,
				missingDeps[0].Name)
		}

		w.logger.Debug("all entity dependencies ready, proceeding with sync",
			zap.String("resource_type", item.ResourceType))
	} else {
		w.logger.Debug("no entity dependency check for DELETE operation",
			zap.String("resource_type", item.ResourceType))
	}

	// Step 3: Get appropriate syncer based on resource type
	syncer, err := w.getSyncerForEntity(item.ResourceType)
	if err != nil {
		// P0-3: Record failure metric for syncer error
		duration := time.Since(startTime)
		RecordProcessingFailure(item.ResourceType, operation, "syncer_error", duration)

		return fmt.Errorf("no syncer for resource type %s: %w", item.ResourceType, err)
	}

	syncCtx, cancel := context.WithTimeout(ctx, w.config.SGROUPTimeout)
	defer cancel()

	// Convert domain operation to sync operation
	syncOperation := convertToSyncOperation(item.Operation)

	// Call syncer (resource must implement interfaces.SyncableEntity)
	syncableEntity, ok := resource.(interfaces.SyncableEntity)
	if !ok {
		// P0-3: Record failure metric for type assertion error
		duration := time.Since(startTime)
		RecordProcessingFailure(item.ResourceType, operation, "type_assertion_error", duration)

		return fmt.Errorf("resource does not implement SyncableEntity interface: %T", resource)
	}

	if err := syncer.Sync(syncCtx, syncableEntity, syncOperation); err != nil {
		w.logger.Error("SGROUP sync failed",
			zap.String("resource_type", item.ResourceType),
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName),
			zap.Error(err))

		// P0-3: Record failure metric for SGROUP sync error
		duration := time.Since(startTime)
		errorCategory := w.categorizeError(err)
		RecordProcessingFailure(item.ResourceType, operation, string(errorCategory), duration)

		return fmt.Errorf("SGROUP sync failed: %w", err)
	}

	w.logger.Info("SGROUP sync successful",
		zap.String("resource_type", item.ResourceType),
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))

	// Step 5: Update resource or delete from DB after SGROUP sync
	// SYNC-FIRST PRINCIPLE: After SGROUP sync succeeds, NOW we can delete from DB
	if item.Operation == domain.SyncOperationDelete {
		// For DELETE: SGROUP synced successfully, NOW delete resource from DB
		w.logger.Debug("DELETE operation - SGROUP sync complete, deleting from DB",
			zap.String("resource_type", item.ResourceType),
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))

		if err := w.deleteResourceFromDB(ctx, item); err != nil {
			// P0-3: Record failure metric for DB deletion error
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "db_delete_error", duration)
			return fmt.Errorf("failed to delete resource from DB: %w", err)
		}

		w.logger.Info("DELETE operation complete - resource deleted from DB",
			zap.String("resource_type", item.ResourceType),
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))
	} else {
		// For CREATE/UPDATE: Mark resource ready and delete outbox entry in transaction
		if err := w.markEntityResourceReady(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName, item.ID); err != nil {
			// SGROUP synced but DB update failed
			// This is OK - Worker will retry, SGROUP sync is idempotent
			w.logger.Error("failed to mark resource ready",
				zap.String("resource_type", item.ResourceType),
				zap.String("namespace", item.ResourceNamespace),
				zap.String("name", item.ResourceName),
				zap.Error(err))

			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "db_update_error", duration)

			return fmt.Errorf("failed to mark resource ready: %w", err)
		}

		// Integration Point #2: Update PendingSync when sync succeeds
		if err := w.updatePendingSyncComplete(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName); err != nil {
			w.logger.Warn("failed to update PendingSync condition on success", zap.Error(err))
		}
	}

	// P0-3: Record success metric
	duration := time.Since(startTime)
	RecordProcessingSuccess(item.ResourceType, operation, duration)

	w.logger.Info("entity resource processed successfully",
		zap.String("resource_type", item.ResourceType),
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName),
		zap.Duration("duration", duration))

	return nil
}

// reconstructResourceFromPayload reconstructs a minimal resource from outbox payload
func (w *OutboxWorker) reconstructResourceFromPayload(item *domain.OutboxEntry) (interface{}, error) {
	// Parse payload JSON
	var payload map[string]interface{}
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	namespace, _ := payload["namespace"].(string)
	name, _ := payload["name"].(string)

	// Reconstruct minimal resource needed for DELETE sync
	switch item.ResourceType {
	case string(registry.TypeHost):
		uuid, _ := payload["uuid"].(string)
		return &models.Host{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      name,
				},
			},
			UUID: uuid,
		}, nil

	case string(registry.TypeNetwork):
		cidr, _ := payload["cidr"].(string)
		return &models.Network{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      name,
				},
			},
			CIDR: cidr,
		}, nil

	case string(registry.TypeAddressGroup):
		return &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      name,
				},
			},
		}, nil

	case string(registry.TypeService):
		return &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      name,
				},
			},
		}, nil

	case string(registry.TypeSvcSvcRule):
		// Extract service references from payload (they're JSONB objects)
		var serviceFromRef v1beta1.NamespacedObjectReference
		var serviceToRef v1beta1.NamespacedObjectReference

		if serviceFromData, ok := payload["service_from_ref"].(map[string]interface{}); ok {
			serviceFromRef = v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: getStringFromMap(serviceFromData, "apiVersion"),
					Kind:       getStringFromMap(serviceFromData, "kind"),
					Name:       getStringFromMap(serviceFromData, "name"),
				},
				Namespace: getStringFromMap(serviceFromData, "namespace"),
			}
		}

		if serviceToData, ok := payload["service_to_ref"].(map[string]interface{}); ok {
			serviceToRef = v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: getStringFromMap(serviceToData, "apiVersion"),
					Kind:       getStringFromMap(serviceToData, "kind"),
					Name:       getStringFromMap(serviceToData, "name"),
				},
				Namespace: getStringFromMap(serviceToData, "namespace"),
			}
		}

		return &models.SvcSvcRule{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      name,
				},
			},
			ServiceFromRef: serviceFromRef,
			ServiceToRef:   serviceToRef,
		}, nil

	default:
		return nil, fmt.Errorf("unknown resource type: %s", item.ResourceType)
	}
}

// loadEntityResource loads a resource from the registry by namespace+name
// FIXED (P0-2): Now uses namespace+name instead of comparing K8s UID with entity UUID
func (w *OutboxWorker) loadEntityResource(
	ctx context.Context,
	resourceType string,
	namespace string, // NEW: K8s namespace
	name string, // NEW: K8s name
) (interface{}, error) {
	// Get reader from registry
	reader, err := w.registry.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	// Query by namespace+name (natural key)
	// This fixes P0-2: No more UID vs UUID comparison!
	switch resourceType {
	case string(registry.TypeHost):
		// Check if registry has Get method, otherwise use List
		// Assuming registry has efficient GetHost method
		var foundHost *models.Host
		err := reader.ListHosts(ctx, func(host models.Host) error {
			if host.Namespace == namespace && host.Name == name {
				foundHost = &host
				return fmt.Errorf("found") // Stop iteration
			}
			return nil
		}, nil)
		if err != nil && err.Error() != "found" {
			return nil, err
		}
		if foundHost == nil {
			return nil, ports.ErrNotFound
		}
		return foundHost, nil

	case string(registry.TypeAddressGroup):
		var foundAG *models.AddressGroup
		err := reader.ListAddressGroups(ctx, func(ag models.AddressGroup) error {
			if ag.Namespace == namespace && ag.Name == name {
				foundAG = &ag
				return fmt.Errorf("found") // Stop iteration
			}
			return nil
		}, nil)
		if err != nil && err.Error() != "found" {
			return nil, err
		}
		if foundAG == nil {
			return nil, ports.ErrNotFound
		}
		return foundAG, nil

	case string(registry.TypeNetwork):
		var foundNetwork *models.Network
		err := reader.ListNetworks(ctx, func(network models.Network) error {
			if network.Namespace == namespace && network.Name == name {
				foundNetwork = &network
				return fmt.Errorf("found") // Stop iteration
			}
			return nil
		}, nil)
		if err != nil && err.Error() != "found" {
			return nil, err
		}
		if foundNetwork == nil {
			return nil, ports.ErrNotFound
		}
		return foundNetwork, nil

	case string(registry.TypeService):
		var foundService *models.Service
		err := reader.ListServices(ctx, func(service models.Service) error {
			if service.Namespace == namespace && service.Name == name {
				foundService = &service
				return fmt.Errorf("found") // Stop iteration
			}
			return nil
		}, nil)
		if err != nil && err.Error() != "found" {
			return nil, err
		}
		if foundService == nil {
			return nil, ports.ErrNotFound
		}
		return foundService, nil

	case string(registry.TypeSvcSvcRule):
		var foundRule *models.SvcSvcRule
		err := reader.ListSvcSvcRules(ctx, func(rule models.SvcSvcRule) error {
			if rule.Namespace == namespace && rule.Name == name {
				foundRule = &rule
				return fmt.Errorf("found") // Stop iteration
			}
			return nil
		}, nil)
		if err != nil && err.Error() != "found" {
			return nil, err
		}
		if foundRule == nil {
			return nil, ports.ErrNotFound
		}
		return foundRule, nil

	default:
		return nil, fmt.Errorf("unknown resource type: %s", resourceType)
	}
}

// getSyncerForEntity returns the appropriate syncer for the given entity resource type
func (w *OutboxWorker) getSyncerForEntity(resourceType string) (interfaces.EntitySyncer[interfaces.SyncableEntity], error) {
	// Map resource type to syncer
	switch resourceType {
	case string(registry.TypeHost):
		return w.hostSyncer, nil
	case string(registry.TypeAddressGroup):
		return w.addressGroupSyncer, nil
	case string(registry.TypeNetwork):
		return w.networkSyncer, nil
	case string(registry.TypeService):
		return w.serviceSyncer, nil
	case string(registry.TypeSvcSvcRule):
		return w.svcSvcRuleSyncer, nil
	default:
		return nil, fmt.Errorf("no syncer registered for type: %s", resourceType)
	}
}

// markEntityResourceReady marks the entity resource as ready and deletes the outbox entry
// This is done in a transaction to ensure atomicity
func (w *OutboxWorker) markEntityResourceReady(
	ctx context.Context,
	resourceType string,
	namespace string, // NEW: Use namespace+name
	name string, // NEW
	outboxID interface{},
) error {
	// Get writer from registry
	writer, err := w.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort() // Ensure cleanup if commit fails

	w.logger.Debug("markEntityResourceReady: got writer",
		zap.String("resource_type", resourceType),
		zap.String("namespace", namespace),
		zap.String("name", name))

	// Load the resource again
	resource, err := w.loadEntityResource(ctx, resourceType, namespace, name)
	if err != nil {
		return fmt.Errorf("failed to load resource for update: %w", err)
	}

	w.logger.Debug("markEntityResourceReady: loaded resource",
		zap.String("resource_type", resourceType),
		zap.String("namespace", namespace),
		zap.String("name", name))

	// Update the resource's Meta to set Ready condition
	var resourceScope ports.Scope
	switch r := resource.(type) {
	case *models.Host:
		r.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Synced to SGROUP")
		r.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Successfully synced")
		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      r.Name,
			Namespace: r.Namespace,
		})
		if err := writer.SyncHosts(ctx, []models.Host{*r}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			return fmt.Errorf("failed to update host: %w", err)
		}

	case *models.AddressGroup:
		// ДИАГНОСТИКА: Условия ДО SetReadyCondition
		w.logger.Info("[DIAG] markEntityResourceReady: AddressGroup - BEFORE SetReadyCondition",
			zap.String("namespace", r.Namespace),
			zap.String("name", r.Name),
			zap.Int("conditions_count", len(r.Meta.Conditions)),
			zap.Any("conditions_before", r.Meta.Conditions))

		startTime := time.Now()
		r.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Synced to SGROUP")
		r.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Successfully synced")
		setConditionDuration := time.Since(startTime)

		// ДИАГНОСТИКА: Условия ПОСЛЕ SetReadyCondition
		w.logger.Info("[DIAG] markEntityResourceReady: AddressGroup - AFTER SetReadyCondition",
			zap.String("namespace", r.Namespace),
			zap.String("name", r.Name),
			zap.Int("conditions_count", len(r.Meta.Conditions)),
			zap.Any("conditions_after", r.Meta.Conditions),
			zap.Duration("set_duration_ns", setConditionDuration))

		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      r.Name,
			Namespace: r.Namespace,
		})

		// ДИАГНОСТИКА: ДО вызова SyncAddressGroups
		w.logger.Info("[DIAG] markEntityResourceReady: AddressGroup - CALLING SyncAddressGroups",
			zap.String("namespace", r.Namespace),
			zap.String("name", r.Name),
			zap.String("operation_type", "ConditionOnlyOperation"),
			zap.Bool("has_writer", writer != nil),
			zap.Bool("has_scope", resourceScope != nil))

		syncStartTime := time.Now()
		if err := writer.SyncAddressGroups(ctx, []models.AddressGroup{*r}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			w.logger.Error("markEntityResourceReady: SyncAddressGroups FAILED",
				zap.String("namespace", r.Namespace),
				zap.String("name", r.Name),
				zap.Error(err),
				zap.Duration("failed_after_ns", time.Since(syncStartTime)))
			return fmt.Errorf("failed to update address group: %w", err)
		}
		syncDuration := time.Since(syncStartTime)

		// ДИАГНОСТИКА: ПОСЛЕ успешного SyncAddressGroups
		w.logger.Info("[DIAG] markEntityResourceReady: AddressGroup - SyncAddressGroups SUCCESS",
			zap.String("namespace", r.Namespace),
			zap.String("name", r.Name),
			zap.Duration("sync_duration_ns", syncDuration))

	case *models.Network:
		r.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Synced to SGROUP")
		r.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Successfully synced")
		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      r.Name,
			Namespace: r.Namespace,
		})
		if err := writer.SyncNetworks(ctx, []models.Network{*r}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			return fmt.Errorf("failed to update network: %w", err)
		}

	case *models.Service:
		r.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Synced to SGROUP")
		r.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Successfully synced")
		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      r.Name,
			Namespace: r.Namespace,
		})
		w.logger.Info("markEntityResourceReady: calling SyncServices with ConditionOnlyOperation",
			zap.String("namespace", r.Namespace),
			zap.String("name", r.Name))
		if err := writer.SyncServices(ctx, []models.Service{*r}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			w.logger.Error("markEntityResourceReady: SyncServices failed",
				zap.String("namespace", r.Namespace),
				zap.String("name", r.Name),
				zap.Error(err))
			return fmt.Errorf("failed to update service: %w", err)
		}
		w.logger.Info("markEntityResourceReady: SyncServices succeeded",
			zap.String("namespace", r.Namespace),
			zap.String("name", r.Name))

	case *models.SvcSvcRule:
		r.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Synced to SGROUP")
		r.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Successfully synced")
		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      r.Name,
			Namespace: r.Namespace,
		})
		if err := writer.SyncSvcSvcRules(ctx, []models.SvcSvcRule{*r}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			return fmt.Errorf("failed to update svcsvc rule: %w", err)
		}

	default:
		return fmt.Errorf("unknown resource type for update: %T", resource)
	}

	// Commit the changes
	w.logger.Info("markEntityResourceReady: committing transaction",
		zap.String("resource_type", resourceType),
		zap.String("namespace", namespace),
		zap.String("name", name))
	if err := writer.Commit(); err != nil {
		w.logger.Error("markEntityResourceReady: commit failed",
			zap.String("resource_type", resourceType),
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.Error(err))
		return fmt.Errorf("failed to commit changes: %w", err)
	}
	w.logger.Info("markEntityResourceReady: transaction committed",
		zap.String("resource_type", resourceType),
		zap.String("namespace", namespace),
		zap.String("name", name))

	// TEMPORARY: Don't delete outbox entries for debugging
	// Delete outbox entry using outbox repository
	// w.logger.Debug("markEntityResourceReady: deleting outbox entry",
	// 	zap.String("outbox_id", outboxID.(uuid.UUID).String()))
	// if err := w.outboxRepo.Delete(ctx, outboxID.(uuid.UUID)); err != nil {
	// 	w.logger.Error("markEntityResourceReady: failed to delete outbox entry",
	// 		zap.String("outbox_id", outboxID.(uuid.UUID).String()),
	// 		zap.Error(err))
	// 	return fmt.Errorf("failed to delete outbox entry: %w", err)
	// }
	w.logger.Info("markEntityResourceReady: outbox entry PRESERVED (not deleted for debugging)",
		zap.String("resource_type", resourceType),
		zap.String("namespace", namespace),
		zap.String("name", name),
		zap.String("outbox_id", outboxID.(uuid.UUID).String()))

	return nil
}

// convertToSyncOperation converts domain SyncOperation to sync.types.SyncOperation
func convertToSyncOperation(op domain.SyncOperation) types.SyncOperation {
	switch op {
	case domain.SyncOperationCreate:
		return types.SyncOperationUpsert
	case domain.SyncOperationUpdate:
		// FIXED: SGROUP API doesn't have "Update" operation, use Upsert instead
		return types.SyncOperationUpsert
	case domain.SyncOperationDelete:
		return types.SyncOperationDelete
	default:
		return types.SyncOperationUpsert
	}
}

// EntitySyncerAdapter wraps the existing syncers to match the EntitySyncer interface
type EntitySyncerAdapter struct {
	hostSyncer         *syncers.HostSyncer
	addressGroupSyncer *syncers.AddressGroupSyncer
	networkSyncer      *syncers.NetworkSyncer
	serviceSyncer      *syncers.ServiceSyncer
	svcSvcRuleSyncer   *syncers.SvcSvcRuleSyncer
	subjectType        types.SyncSubjectType
}

func (a *EntitySyncerAdapter) Sync(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	switch a.subjectType {
	case types.SyncSubjectTypeHosts:
		return a.hostSyncer.Sync(ctx, entity, operation)
	case types.SyncSubjectTypeGroups:
		return a.addressGroupSyncer.Sync(ctx, entity, operation)
	case types.SyncSubjectTypeNetworks:
		return a.networkSyncer.Sync(ctx, entity, operation)
	case types.SyncSubjectTypeServices:
		return a.serviceSyncer.Sync(ctx, entity, operation)
	case types.SyncSubjectTypeSvcSvcRules:
		return a.svcSvcRuleSyncer.Sync(ctx, entity, operation)
	default:
		return fmt.Errorf("unsupported subject type: %s", a.subjectType)
	}
}

func (a *EntitySyncerAdapter) SyncBatch(ctx context.Context, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	switch a.subjectType {
	case types.SyncSubjectTypeHosts:
		return a.hostSyncer.SyncBatch(ctx, entities, operation)
	case types.SyncSubjectTypeGroups:
		return a.addressGroupSyncer.SyncBatch(ctx, entities, operation)
	case types.SyncSubjectTypeNetworks:
		return a.networkSyncer.SyncBatch(ctx, entities, operation)
	case types.SyncSubjectTypeServices:
		return a.serviceSyncer.SyncBatch(ctx, entities, operation)
	case types.SyncSubjectTypeSvcSvcRules:
		return a.svcSvcRuleSyncer.SyncBatch(ctx, entities, operation)
	default:
		return fmt.Errorf("unsupported subject type: %s", a.subjectType)
	}
}

func (a *EntitySyncerAdapter) GetSupportedSubjectType() types.SyncSubjectType {
	return a.subjectType
}

// deleteResourceFromDB deletes the resource from database after successful SGROUP sync
// SYNC-FIRST PRINCIPLE: This is called ONLY after SGROUP sync succeeds
// Deletes: resource table row + k8s_metadata + outbox entry (all in one transaction)
func (w *OutboxWorker) deleteResourceFromDB(ctx context.Context, item *domain.OutboxEntry) error {
	// Get connection from pool for transactional deletion
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	// Begin transaction
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // Ensure cleanup if commit fails

	// Step 1: Delete the resource from its table (hosts/networks/address_groups)
	// Use raw SQL to delete by namespace+name
	var deleteQuery string
	switch item.ResourceType {
	case string(registry.TypeHost):
		deleteQuery = `DELETE FROM hosts WHERE namespace = $1 AND name = $2`
	case string(registry.TypeNetwork):
		deleteQuery = `DELETE FROM networks WHERE namespace = $1 AND name = $2`
	case string(registry.TypeAddressGroup):
		deleteQuery = `DELETE FROM address_groups WHERE namespace = $1 AND name = $2`
	case string(registry.TypeService):
		deleteQuery = `DELETE FROM services WHERE namespace = $1 AND name = $2`
	case string(registry.TypeSvcSvcRule):
		deleteQuery = `DELETE FROM svc_svc_rules WHERE namespace = $1 AND name = $2`
	default:
		return fmt.Errorf("unknown resource type for deletion: %s", item.ResourceType)
	}

	// Execute resource deletion
	cmdTag, err := tx.Exec(ctx, deleteQuery, item.ResourceNamespace, item.ResourceName)
	if err != nil {
		return fmt.Errorf("failed to delete %s from DB: %w", item.ResourceType, err)
	}

	w.logger.Debug("deleted resource from DB",
		zap.String("resource_type", item.ResourceType),
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName),
		zap.Int64("rows_affected", cmdTag.RowsAffected()))

	// Step 2: Delete k8s_metadata (cascade should handle this, but explicit is safer)
	// Note: We can't easily identify the exact metadata row without resource_version
	// But PostgreSQL CASCADE DELETE from resource table should handle this automatically
	// If metadata is orphaned, it's cleaned up by periodic maintenance

	// Step 3: Delete outbox entry
	deleteOutboxQuery := `DELETE FROM sync_outbox WHERE id = $1`
	_, err = tx.Exec(ctx, deleteOutboxQuery, item.ID)
	if err != nil {
		return fmt.Errorf("failed to delete outbox entry: %w", err)
	}

	// Commit transaction (resource deleted + outbox entry deleted)
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit deletion transaction: %w", err)
	}

	w.logger.Info("resource and outbox entry deleted from DB",
		zap.String("resource_type", item.ResourceType),
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))

	return nil
}

// getStringFromMap safely extracts a string value from a map[string]interface{}
// Returns empty string if key doesn't exist or value is not a string
func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}
