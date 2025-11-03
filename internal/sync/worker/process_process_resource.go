package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/domain/registry"
)

type AffectedResource struct {
	Type              string `json:"type"`
	ResourceType      string `json:"resourceType"`
	Namespace         string `json:"namespace"`
	ResourceNamespace string `json:"resourceNamespace"`
	Name              string `json:"name"`
	ResourceName      string `json:"resourceName"`
}

func normalizeAffectedResource(res *AffectedResource) {
	if res == nil {
		return
	}
	if res.Type == "" && res.ResourceType != "" {
		res.Type = res.ResourceType
	}
	if res.Namespace == "" && res.ResourceNamespace != "" {
		res.Namespace = res.ResourceNamespace
	}
	if res.Name == "" && res.ResourceName != "" {
		res.Name = res.ResourceName
	}
}

func extractAffectedResources(item *domain.OutboxEntry) ([]AffectedResource, error) {
	resources := make([]AffectedResource, 0)
	if len(item.AffectsResources) > 0 {
		if err := json.Unmarshal(item.AffectsResources, &resources); err != nil {
			return nil, fmt.Errorf("failed to parse affects_resources column: %w", err)
		}
	}
	if len(resources) == 0 && len(item.Payload) > 0 {
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(item.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse payload: %w", err)
		}
		if raw, ok := payload["affectedResources"]; ok && len(raw) > 0 {
			if err := json.Unmarshal(raw, &resources); err != nil {
				return nil, fmt.Errorf("failed to parse payload.affectedResources: %w", err)
			}
		}
	}
	return resources, nil
}

// processProcessResource handles binding resources by checking dependencies and updating readiness status
func (w *OutboxWorker) processProcessResource(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	w.logger.Info("processing process resource",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()),
		zap.String("operation", string(item.Operation)))

	// DELETE operations require coordinated deletion flow
	if item.Operation == domain.SyncOperationDelete {
		w.logger.Info("routing DELETE to coordinated deletion handler",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))
		return w.processProcessResourceDelete(ctx, item)
	}

	affectedResources, err := extractAffectedResources(item)
	if err != nil {
		w.logger.Error("failed to parse affected resources",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()),
			zap.Error(err))
		return err
	}

	if len(affectedResources) == 0 {
		w.logger.Warn("process resource has no affected resources",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))
		return w.markProcessResourceReady(ctx, item)
	}

	w.logger.Debug("checking affected resources",
		zap.String("resource_type", item.ResourceType),
		zap.Int("affected_count", len(affectedResources)))

	pendingResources := []string{}
	for i := range affectedResources {
		normalizeAffectedResource(&affectedResources[i])
		affected := affectedResources[i]
		ready, err := w.isResourceReady(ctx, affected.Type, affected.Namespace, affected.Name)
		if err != nil {
			w.logger.Error("failed to check resource readiness",
				zap.String("affected_type", affected.Type),
				zap.String("affected_namespace", affected.Namespace),
				zap.String("affected_name", affected.Name),
				zap.Error(err))
			return fmt.Errorf("failed to check resource readiness for %s/%s/%s: %w",
				affected.Type, affected.Namespace, affected.Name, err)
		}

		if !ready {
			pendingResources = append(pendingResources,
				fmt.Sprintf("%s/%s", affected.Type, affected.Name))

			if err := w.ensureOutboxEntryExists(ctx, affected); err != nil {
				w.logger.Warn("failed to ensure outbox entry",
					zap.String("affected_type", affected.Type),
					zap.String("affected_namespace", affected.Namespace),
					zap.String("affected_name", affected.Name),
					zap.Error(err))
			}
		}
	}

	if len(pendingResources) == 0 {
		w.logger.Info("all affected resources ready, marking process resource ready",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))

		return w.markProcessResourceReady(ctx, item)
	}

	if err := w.updatePendingSyncWaitingDependencies(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName, pendingResources); err != nil {
		w.logger.Warn("failed to update PendingSync condition", zap.Error(err))
	}

	w.logger.Info("process resource still pending dependencies",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()),
		zap.Strings("pending_resources", pendingResources))

	msg := fmt.Sprintf("Waiting for: %s", strings.Join(pendingResources, ", "))
	retryErr := fmt.Errorf("waiting for affected resources: %s", msg)
	return w.scheduleRetry(ctx, item, retryErr, 10*time.Second)
}

// markProcessResourceReady updates binding conditions and deletes outbox entry after dependencies are ready
func (w *OutboxWorker) markProcessResourceReady(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	lastErr := ""
	if item.LastError != nil {
		lastErr = *item.LastError
	}

	w.logger.Info("marking process resource ready",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()),
		zap.String("operation", string(item.Operation)))
	w.logger.Debug("process resource state before mark ready",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_namespace", item.ResourceNamespace),
		zap.String("resource_name", item.ResourceName),
		zap.String("outbox_status", string(item.Status)),
		zap.Int("attempts", item.Attempts),
		zap.String("last_error", lastErr))

	if item.Operation == domain.SyncOperationDelete {
		// DELETE operations are handled by processProcessResourceDelete
		// This path should not be reached for DELETE operations
		w.logger.Warn("DELETE operation reached markProcessResourceReady - this is unexpected",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))
		return fmt.Errorf("DELETE operation should be handled by processProcessResourceDelete")
	}

	writer, err := w.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get registry writer: %w", err)
	}
	defer writer.Abort()

	var resourceScope ports.Scope

	switch item.ResourceType {
	case string(registry.TypeHostBinding):
		var foundBinding *models.HostBinding
		err := w.loadResourceFromRegistry(ctx, item, func() error {
			reader, err := w.registry.Reader(ctx)
			if err != nil {
				return err
			}
			defer reader.Close()

			return reader.ListHostBindings(ctx, func(hb models.HostBinding) error {
				if hb.Meta.UID == item.ResourceID.String() {
					foundBinding = &hb
					return fmt.Errorf("found")
				}
				return nil
			}, nil)
		})

		if err != nil && err.Error() != "found" {
			return fmt.Errorf("failed to load HostBinding: %w", err)
		}
		if foundBinding == nil {
			w.logger.Warn("process resource binding not found in registry",
				zap.String("resource_type", item.ResourceType),
				zap.String("resource_id", item.ResourceID.String()),
				zap.String("resource_namespace", item.ResourceNamespace),
				zap.String("resource_name", item.ResourceName),
				zap.String("operation", string(item.Operation)),
				zap.Int("attempts", item.Attempts),
				zap.String("last_error", lastErr))
			if err := w.markProcessOutboxCompleted(ctx, item); err != nil {
				return fmt.Errorf("mark outbox completed for HostBinding: %w", err)
			}
			return nil
		}

		foundBinding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "All dependencies are ready")
		foundBinding.Meta.SetCondition(metav1.Condition{
			Type:               "PendingSync",
			Status:             metav1.ConditionFalse,
			Reason:             "AllDependenciesReady",
			Message:            "",
			LastTransitionTime: metav1.Now(),
		})

		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      foundBinding.Name,
			Namespace: foundBinding.Namespace,
		})

		if err := writer.SyncHostBindings(ctx, []models.HostBinding{*foundBinding}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			return fmt.Errorf("failed to update HostBinding: %w", err)
		}

	case string(registry.TypeNetworkBinding):
		var foundBinding *models.NetworkBinding
		err := w.loadResourceFromRegistry(ctx, item, func() error {
			reader, err := w.registry.Reader(ctx)
			if err != nil {
				return err
			}
			defer reader.Close()

			return reader.ListNetworkBindings(ctx, func(nb models.NetworkBinding) error {
				if nb.Meta.UID == item.ResourceID.String() {
					foundBinding = &nb
					return fmt.Errorf("found")
				}
				return nil
			}, nil)
		})

		if err != nil && err.Error() != "found" {
			return fmt.Errorf("failed to load NetworkBinding: %w", err)
		}
		if foundBinding == nil {
			w.logger.Warn("process resource network binding not found in registry",
				zap.String("resource_type", item.ResourceType),
				zap.String("resource_id", item.ResourceID.String()),
				zap.String("resource_namespace", item.ResourceNamespace),
				zap.String("resource_name", item.ResourceName),
				zap.String("operation", string(item.Operation)),
				zap.Int("attempts", item.Attempts),
				zap.String("last_error", lastErr))
			if err := w.markProcessOutboxCompleted(ctx, item); err != nil {
				return fmt.Errorf("mark outbox completed for NetworkBinding: %w", err)
			}
			return nil
		}

		foundBinding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "All dependencies are ready")
		foundBinding.Meta.SetCondition(metav1.Condition{
			Type:               "PendingSync",
			Status:             metav1.ConditionFalse,
			Reason:             "AllDependenciesReady",
			Message:            "",
			LastTransitionTime: metav1.Now(),
		})

		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      foundBinding.Name,
			Namespace: foundBinding.Namespace,
		})

		if err := writer.SyncNetworkBindings(ctx, []models.NetworkBinding{*foundBinding}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			return fmt.Errorf("failed to update NetworkBinding: %w", err)
		}

	case string(registry.TypeAddressGroupBinding):
		var foundBinding *models.AddressGroupBinding
		err := w.loadResourceFromRegistry(ctx, item, func() error {
			reader, err := w.registry.Reader(ctx)
			if err != nil {
				return err
			}
			defer reader.Close()

			return reader.ListAddressGroupBindings(ctx, func(agb models.AddressGroupBinding) error {
				if agb.Meta.UID == item.ResourceID.String() {
					foundBinding = &agb
					return fmt.Errorf("found")
				}
				return nil
			}, nil)
		})

		if err != nil && err.Error() != "found" {
			return fmt.Errorf("failed to load AddressGroupBinding: %w", err)
		}
		if foundBinding == nil {
			w.logger.Warn("process resource address group binding not found in registry",
				zap.String("resource_type", item.ResourceType),
				zap.String("resource_id", item.ResourceID.String()),
				zap.String("resource_namespace", item.ResourceNamespace),
				zap.String("resource_name", item.ResourceName),
				zap.String("operation", string(item.Operation)),
				zap.Int("attempts", item.Attempts),
				zap.String("last_error", lastErr))
			if err := w.markProcessOutboxCompleted(ctx, item); err != nil {
				return fmt.Errorf("mark outbox completed for AddressGroupBinding: %w", err)
			}
			return nil
		}

		foundBinding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "All dependencies are ready")
		foundBinding.Meta.SetCondition(metav1.Condition{
			Type:               "PendingSync",
			Status:             metav1.ConditionFalse,
			Reason:             "AllDependenciesReady",
			Message:            "",
			LastTransitionTime: metav1.Now(),
		})

		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      foundBinding.Name,
			Namespace: foundBinding.Namespace,
		})

		if err := writer.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{*foundBinding}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			return fmt.Errorf("failed to update AddressGroupBinding: %w", err)
		}

	default:
		return fmt.Errorf("unknown process resource type: %s", item.ResourceType)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	if err := w.markProcessOutboxCompleted(ctx, item); err != nil {
		return fmt.Errorf("mark outbox completed: %w", err)
	}

	w.logger.Info("process resource marked ready successfully",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()))

	return nil
}

func (w *OutboxWorker) markProcessOutboxCompleted(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	if err := w.outboxRepo.MarkCompleted(ctx, item.ID); err != nil {
		return err
	}

	if err := w.updatePendingSyncComplete(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName); err != nil {
		w.logger.Warn("failed to update PendingSync to complete for process resource",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_namespace", item.ResourceNamespace),
			zap.String("resource_name", item.ResourceName),
			zap.String("resource_id", item.ResourceID.String()),
			zap.Error(err))
	}

	w.logger.Info("process resource outbox marked as completed",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()))
	return nil
}

func (w *OutboxWorker) loadResourceFromRegistry(
	ctx context.Context,
	item *domain.OutboxEntry,
	loadFunc func() error,
) error {
	err := loadFunc()
	if err != nil && err.Error() != "found" {
		return err
	}
	return nil
}

// processProcessResourceDelete handles DELETE operations for process resources (bindings)
// using the coordinated deletion architecture.
//
// ARCHITECTURE: Trigger-First Approach (Migration 044)
// 1. Database triggers already updated affected resources when deletion_timestamp was set
// 2. OutboxWorker only coordinates: wait for SGROUP sync, then physically delete
// 3. No manual updates needed - triggers handle everything!
//
// See: docs/architecture/COORDINATED_BINDING_DELETION.md
func (w *OutboxWorker) processProcessResourceDelete(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	w.logger.Info("processing process resource DELETE (coordinated deletion)",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()))

	// REMOVED: updateAffectedResourcesForBindingDelete()
	// Database triggers already did this when deletion_timestamp was set!

	if err := w.reprocessAffectedResourceConditions(ctx, item); err != nil {
		w.logger.Warn("failed to reprocess affected resource conditions",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()),
			zap.Error(err))
	}

	// Step 1: Check if all affected resources are synced to SGROUP
	synced, err := w.checkAffectedResourcesSynced(ctx, item)
	if err != nil {
		return fmt.Errorf("checking sync status: %w", err)
	}

	if !synced {
		// Not yet synced - return special error that won't increment attempts
		w.logger.Debug("affected resources not yet synced, waiting",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()),
			zap.Int("attempts", item.Attempts))
		return ErrWaitingForSync
	}

	// Step 2: All affected resources synced - physically delete the binding
	w.logger.Info("all affected resources synced, proceeding with physical deletion",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()))

	if err := w.deleteBindingFromDB(ctx, item); err != nil {
		return fmt.Errorf("deleting binding from DB: %w", err)
	}

	// Step 3: Delete outbox entry
	if err := w.outboxRepo.Delete(ctx, item.ID); err != nil {
		return fmt.Errorf("deleting outbox entry: %w", err)
	}

	w.logger.Info("process resource DELETE completed successfully",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()))

	return nil
}

func (w *OutboxWorker) reprocessAffectedResourceConditions(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	if w.conditionManager == nil {
		return nil
	}

	affectedResources, err := extractAffectedResources(item)
	if err != nil {
		return err
	}

	if len(affectedResources) == 0 {
		w.logger.Debug("no affected resources for condition reprocess",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))
		return nil
	}

	reader, err := w.registry.Reader(ctx)
	if err != nil {
		return fmt.Errorf("failed to get registry reader: %w", err)
	}
	defer reader.Close()

	for i := range affectedResources {
		normalizeAffectedResource(&affectedResources[i])
		affected := affectedResources[i]
		w.logger.Info("reprocessing affected resource conditions",
			zap.String("type", affected.Type),
			zap.String("namespace", affected.Namespace),
			zap.String("name", affected.Name))
		switch affected.Type {
		case string(registry.TypeAddressGroup):
			ag, err := reader.GetAddressGroupByID(ctx, models.ResourceIdentifier{Namespace: affected.Namespace, Name: affected.Name})
			if err != nil {
				if errors.Is(err, ports.ErrNotFound) {
					w.logger.Debug("address group removed before condition reprocess",
						zap.String("namespace", affected.Namespace),
						zap.String("name", affected.Name))
					continue
				}
				return fmt.Errorf("failed to load AddressGroup %s/%s: %w", affected.Namespace, affected.Name, err)
			}

			ag.Meta.DeduplicateConditions()
			if err := w.conditionManager.ProcessAddressGroupConditions(ctx, ag); err != nil {
				w.logger.Error("failed to process AddressGroup conditions",
					zap.String("namespace", ag.Namespace),
					zap.String("name", ag.Name),
					zap.Error(err))
				continue
			}
			if err := w.conditionManager.SaveAddressGroupConditions(ctx, ag); err != nil {
				w.logger.Error("failed to save AddressGroup conditions",
					zap.String("namespace", ag.Namespace),
					zap.String("name", ag.Name),
					zap.Error(err))
			} else {
				w.logger.Info("AddressGroup conditions reprocessed",
					zap.String("namespace", ag.Namespace),
					zap.String("name", ag.Name),
					zap.Int("conditions", len(ag.Meta.Conditions)))
			}

		case string(registry.TypeHost):
			host, err := reader.GetHostByID(ctx, models.ResourceIdentifier{Namespace: affected.Namespace, Name: affected.Name})
			if err != nil {
				if errors.Is(err, ports.ErrNotFound) {
					w.logger.Debug("host removed before condition reprocess",
						zap.String("namespace", affected.Namespace),
						zap.String("name", affected.Name))
					continue
				}
				return fmt.Errorf("failed to load Host %s/%s: %w", affected.Namespace, affected.Name, err)
			}

			host.Meta.DeduplicateConditions()
			if err := w.conditionManager.ProcessHostConditions(ctx, host, nil); err != nil {
				w.logger.Error("failed to process Host conditions",
					zap.String("namespace", host.Namespace),
					zap.String("name", host.Name),
					zap.Error(err))
				continue
			}
			if err := w.conditionManager.SaveHostConditions(ctx, host); err != nil {
				w.logger.Error("failed to save Host conditions",
					zap.String("namespace", host.Namespace),
					zap.String("name", host.Name),
					zap.Error(err))
			} else {
				w.logger.Info("Host conditions reprocessed",
					zap.String("namespace", host.Namespace),
					zap.String("name", host.Name),
					zap.Int("conditions", len(host.Meta.Conditions)))
			}
		default:
			w.logger.Debug("condition reprocess skipped for resource type",
				zap.String("type", affected.Type),
				zap.String("namespace", affected.Namespace),
				zap.String("name", affected.Name))
		}
	}

	return nil
}

// updateAffectedResourcesForBindingDelete updates Host and AddressGroup when binding is deleted.
//
// DEPRECATED: This function is no longer needed with Migration 044.
// Database triggers (trigger_host_binding_on_deletion_mark) automatically update
// affected resources when deletion_timestamp is set.
//
// Kept for reference and potential rollback scenarios.
// TODO: Remove after Migration 044 is stable in production.
//
//lint:ignore U1000 Retained for rollback/reference scenarios.
func (w *OutboxWorker) updateAffectedResourcesForBindingDelete(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	// Parse affected resources from outbox entry
	affectedResources, err := extractAffectedResources(item)
	if err != nil {
		return err
	}

	if len(affectedResources) == 0 {
		w.logger.Warn("process resource DELETE has no affected resources",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))
		return nil
	}

	writer, err := w.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get registry writer: %w", err)
	}
	defer writer.Abort()

	// Update each affected resource
	for i := range affectedResources {
		normalizeAffectedResource(&affectedResources[i])
		affected := affectedResources[i]
		w.logger.Info("updating affected resource for binding delete",
			zap.String("affected_type", affected.Type),
			zap.String("affected_namespace", affected.Namespace),
			zap.String("affected_name", affected.Name))

		switch affected.Type {
		case string(registry.TypeHost):
			if err := w.updateHostForBindingDelete(ctx, writer, affected); err != nil {
				return fmt.Errorf("failed to update Host: %w", err)
			}

		case string(registry.TypeAddressGroup):
			w.logger.Debug("skipping direct AddressGroup aggregation update; handled by triggers",
				zap.String("namespace", affected.Namespace),
				zap.String("name", affected.Name))
		default:
			w.logger.Warn("unknown affected resource type",
				zap.String("type", affected.Type))
		}
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	w.logger.Info("affected resources updated successfully",
		zap.String("resource_type", item.ResourceType),
		zap.Int("count", len(affectedResources)))

	return nil
}

// updateHostForBindingDelete updates Host to clear binding references.
//
// DEPRECATED: This function is no longer needed with Migration 044.
// Database trigger (trigger_host_binding_on_deletion_mark) automatically updates Host.
// TODO: Remove after Migration 044 is stable in production.
//
//lint:ignore U1000 Retained for rollback/reference scenarios.
func (w *OutboxWorker) updateHostForBindingDelete(
	ctx context.Context,
	writer ports.Writer,
	affected AffectedResource,
) error {
	reader, err := w.registry.Reader(ctx)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Load Host
	var foundHost *models.Host
	err = reader.ListHosts(ctx, func(h models.Host) error {
		if h.Namespace == affected.Namespace && h.Name == affected.Name {
			foundHost = &h
			return fmt.Errorf("found")
		}
		return nil
	}, nil)

	if err != nil && err.Error() != "found" {
		return fmt.Errorf("failed to load Host: %w", err)
	}
	if foundHost == nil {
		w.logger.Warn("Host not found, may have been deleted",
			zap.String("namespace", affected.Namespace),
			zap.String("name", affected.Name))
		return nil // Not an error - Host may have been deleted
	}

	// Update Host status
	foundHost.IsBound = false
	foundHost.AddressGroupName = ""
	foundHost.AddressGroupRef = nil
	foundHost.BindingRef = nil

	// Update conditions
	foundHost.Meta.SetReadyCondition(metav1.ConditionFalse, "BindingDeleting", "Binding deleted, waiting for SGROUP sync")
	foundHost.Meta.SetCondition(metav1.Condition{
		Type:               "PendingSync",
		Status:             metav1.ConditionTrue,
		Reason:             "BindingCleanup",
		Message:            "Syncing binding cleanup to SGROUP",
		LastTransitionTime: metav1.Now(),
	})
	foundHost.Meta.SetCondition(metav1.Condition{
		Type:               "Synced",
		Status:             metav1.ConditionFalse,
		Reason:             "PendingSync",
		Message:            "Waiting for SGROUP sync",
		LastTransitionTime: metav1.Now(),
	})

	// Save Host (this will create UPDATE outbox entry)
	resourceScope := ports.NewResourceIdentifierScope(models.ResourceIdentifier{
		Name:      foundHost.Name,
		Namespace: foundHost.Namespace,
	})

	if err := writer.SyncHosts(ctx, []models.Host{*foundHost}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
		return fmt.Errorf("failed to update Host: %w", err)
	}

	w.logger.Info("Host updated for binding delete",
		zap.String("namespace", affected.Namespace),
		zap.String("name", affected.Name))

	return nil
}

// updateAddressGroupForBindingDelete updates AddressGroup to remove Host from xAggregatedHosts.
//
// DEPRECATED: This function is no longer needed with Migration 044.
// Database trigger (trigger_host_binding_on_deletion_mark) automatically updates AG.aggregatedHosts.
// TODO: Remove after Migration 044 is stable in production.
//
//lint:ignore U1000 Retained for rollback/reference scenarios.
func (w *OutboxWorker) updateAddressGroupForBindingDelete(
	ctx context.Context,
	writer ports.Writer,
	affected AffectedResource,
	bindingEntry *domain.OutboxEntry,
) error {
	reader, err := w.registry.Reader(ctx)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Load AddressGroup
	var foundAG *models.AddressGroup
	err = reader.ListAddressGroups(ctx, func(ag models.AddressGroup) error {
		if ag.Namespace == affected.Namespace && ag.Name == affected.Name {
			foundAG = &ag
			return fmt.Errorf("found")
		}
		return nil
	}, nil)

	if err != nil && err.Error() != "found" {
		return fmt.Errorf("failed to load AddressGroup: %w", err)
	}
	if foundAG == nil {
		w.logger.Warn("AddressGroup not found, may have been deleted",
			zap.String("namespace", affected.Namespace),
			zap.String("name", affected.Name))
		return nil // Not an error - AG may have been deleted
	}

	// Find and remove Host from AggregatedHosts
	// Get Host info from affects_resources (not from payload!)
	affectedResources, err := extractAffectedResources(bindingEntry)
	if err != nil {
		return err
	}

	// Find the Host resource in affected resources
	var hostNamespace, hostName string
	for _, res := range affectedResources {
		if res.Type == "Host" {
			hostNamespace = res.Namespace
			hostName = res.Name
			break
		}
	}

	if hostName == "" {
		return fmt.Errorf("host not found in affected resources")
	}

	// Remove Host from AggregatedHosts
	originalCount := len(foundAG.AggregatedHosts)
	newHosts := []models.HostReference{}
	for _, aggHost := range foundAG.AggregatedHosts {
		if aggHost.Ref.Namespace != hostNamespace || aggHost.Ref.Name != hostName {
			newHosts = append(newHosts, aggHost)
		}
	}
	foundAG.AggregatedHosts = newHosts

	w.logger.Info("removed Host from AddressGroup",
		zap.String("ag_namespace", affected.Namespace),
		zap.String("ag_name", affected.Name),
		zap.String("host_namespace", hostNamespace),
		zap.String("host_name", hostName),
		zap.Int("original_count", originalCount),
		zap.Int("new_count", len(newHosts)))

	// Update conditions
	foundAG.Meta.SetReadyCondition(metav1.ConditionFalse, "BindingDeleting", "Host binding removed, waiting for SGROUP sync")
	foundAG.Meta.SetCondition(metav1.Condition{
		Type:               "PendingSync",
		Status:             metav1.ConditionTrue,
		Reason:             "AggregationUpdate",
		Message:            "Syncing aggregation update to SGROUP",
		LastTransitionTime: metav1.Now(),
	})
	foundAG.Meta.SetCondition(metav1.Condition{
		Type:               "Synced",
		Status:             metav1.ConditionFalse,
		Reason:             "PendingSync",
		Message:            "Waiting for SGROUP sync",
		LastTransitionTime: metav1.Now(),
	})

	// Save AddressGroup (this will create UPDATE outbox entry)
	// NOTE: NOT using ConditionOnlyOperation because we need to update aggregatedHosts!
	resourceScope := ports.NewResourceIdentifierScope(models.ResourceIdentifier{
		Name:      foundAG.Name,
		Namespace: foundAG.Namespace,
	})

	if err := writer.SyncAddressGroups(ctx, []models.AddressGroup{*foundAG}, resourceScope); err != nil {
		return fmt.Errorf("failed to update AddressGroup: %w", err)
	}

	w.logger.Info("AddressGroup updated for binding delete",
		zap.String("namespace", affected.Namespace),
		zap.String("name", affected.Name))

	return nil
}

// checkAffectedResourcesSynced checks if all affected resources have been synced to SGROUP

func (w *OutboxWorker) checkAffectedResourcesSynced(
	ctx context.Context,
	item *domain.OutboxEntry,
) (bool, error) {
	affectedResources, err := extractAffectedResources(item)
	if err != nil {
		return false, err
	}

	if len(affectedResources) == 0 {
		w.logger.Debug("no affected resources tracked for process resource",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))
		// No affected resources - considered synced
		return true, nil
	}

	// Check each affected resource
	for i := range affectedResources {
		normalizeAffectedResource(&affectedResources[i])
		affected := affectedResources[i]
		ready, err := w.isResourceReady(ctx, affected.Type, affected.Namespace, affected.Name)
		if err != nil {
			return false, fmt.Errorf("checking readiness for %s/%s: %w",
				affected.Type, affected.Name, err)
		}

		if !ready {
			exists, err := w.entityExists(ctx, affected.Type, affected.Namespace, affected.Name)
			if err != nil {
				return false, fmt.Errorf("checking existence for %s/%s: %w",
					affected.Type, affected.Name, err)
			}
			if !exists {
				w.logger.Debug("resource no longer exists, treating as synced",
					zap.String("type", affected.Type),
					zap.String("namespace", affected.Namespace),
					zap.String("name", affected.Name))
				continue
			}

			w.logger.Debug("resource not yet ready",
				zap.String("type", affected.Type),
				zap.String("namespace", affected.Namespace),
				zap.String("name", affected.Name))
			return false, nil
		}

		w.logger.Debug("resource ready",
			zap.String("type", affected.Type),
			zap.String("namespace", affected.Namespace),
			zap.String("name", affected.Name))
	}

	// All resources ready and synced!
	w.logger.Info("all affected resources synced to SGROUP",
		zap.String("resource_type", item.ResourceType),
		zap.Int("count", len(affectedResources)))

	return true, nil
}

// deleteBindingFromDB physically deletes the binding from database
func (w *OutboxWorker) deleteBindingFromDB(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	var tableName string
	switch item.ResourceType {
	case string(registry.TypeHostBinding):
		tableName = "host_bindings"
	case string(registry.TypeNetworkBinding):
		tableName = "network_bindings"
	case string(registry.TypeAddressGroupBinding):
		tableName = "address_group_bindings"
	default:
		return fmt.Errorf("unknown binding type: %s", item.ResourceType)
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE namespace = $1 AND name = $2", tableName)

	w.logger.Info("physically deleting binding from database",
		zap.String("table", tableName),
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))

	result, err := w.pool.Exec(ctx, query, item.ResourceNamespace, item.ResourceName)
	if err != nil {
		return fmt.Errorf("failed to delete from %s: %w", tableName, err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		w.logger.Warn("binding not found in database, may have been deleted already",
			zap.String("table", tableName),
			zap.String("id", item.ResourceID.String()))
		return nil // Not an error - binding may have been deleted manually
	}

	w.logger.Info("binding physically deleted from database",
		zap.String("table", tableName),
		zap.String("id", item.ResourceID.String()),
		zap.Int64("rows_affected", rowsAffected))

	return nil
}
