package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/domain/registry"
)

// AffectedResource represents a resource that a process resource depends on
type AffectedResource struct {
	Type      string `json:"type"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// processProcessResource processes process resources (HostBinding, NetworkBinding, AddressGroupBinding)
// Process resources don't sync to SGROUP directly. Instead, they:
// 1. Parse affects_resources from Outbox entry
// 2. Check if ALL affected resources are Ready
// 3. If all Ready: mark process resource Ready, delete Outbox
// 4. If NOT all Ready: ensure Outbox exists for pending resources, schedule retry
func (w *OutboxWorker) processProcessResource(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	w.logger.Info("processing process resource",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()),
		zap.String("operation", string(item.Operation)))

	// Step 1: Parse affected resources
	var affectedResources []AffectedResource
	if len(item.AffectsResources) > 0 {
		if err := json.Unmarshal(item.AffectsResources, &affectedResources); err != nil {
			w.logger.Error("failed to parse affected resources",
				zap.String("resource_type", item.ResourceType),
				zap.String("resource_id", item.ResourceID.String()),
				zap.Error(err))
			return fmt.Errorf("failed to parse affected resources: %w", err)
		}
	}

	if len(affectedResources) == 0 {
		// Process resource has no dependencies - this shouldn't happen normally
		// but we can handle it by just marking it ready
		w.logger.Warn("process resource has no affected resources",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))
		return w.markProcessResourceReady(ctx, item)
	}

	w.logger.Debug("checking affected resources",
		zap.String("resource_type", item.ResourceType),
		zap.Int("affected_count", len(affectedResources)))

	// Step 2: Check if ALL affected resources are Ready
	pendingResources := []string{}
	for _, affected := range affectedResources {
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

			// Ensure Outbox entry exists for this resource
			if err := w.ensureOutboxEntryExists(ctx, affected); err != nil {
				w.logger.Warn("failed to ensure outbox entry",
					zap.String("affected_type", affected.Type),
					zap.String("affected_namespace", affected.Namespace),
					zap.String("affected_name", affected.Name),
					zap.Error(err))
				// Don't fail the entire operation - just log and continue
			}
		}
	}

	// Step 3: If all Ready → mark process resource Ready
	if len(pendingResources) == 0 {
		w.logger.Info("all affected resources ready, marking process resource ready",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))

		return w.markProcessResourceReady(ctx, item)
	}

	// Step 4: Still waiting → update PendingSync condition
	// Use the new condition updater with properly formatted message
	if err := w.updatePendingSyncWaitingDependencies(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName, pendingResources); err != nil {
		w.logger.Warn("failed to update PendingSync condition", zap.Error(err))
		// Don't fail - this is just status reporting
	}

	w.logger.Info("process resource still pending dependencies",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()),
		zap.Strings("pending_resources", pendingResources))

	// Return error to trigger retry with short interval (10s)
	msg := fmt.Sprintf("Waiting for: %s", strings.Join(pendingResources, ", "))
	retryErr := fmt.Errorf("waiting for affected resources: %s", msg)
	return w.scheduleRetry(ctx, item, retryErr, 10*time.Second)
}

// markProcessResourceReady marks a process resource as ready and deletes the outbox entry
// This is done in a transaction to ensure atomicity:
// 1. Load the resource from registry
// 2. Update Ready=True, PendingSync=False conditions
// 3. Update resource using registry writer
// 4. Delete Outbox entry
func (w *OutboxWorker) markProcessResourceReady(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	w.logger.Info("marking process resource ready",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()),
		zap.String("operation", string(item.Operation)))

	if item.Operation == domain.SyncOperationDelete {
		w.logger.Info("DELETE operation for process resource - skipping condition update, deleting outbox entry",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))

		if err := w.outboxRepo.Delete(ctx, item.ID); err != nil {
			return fmt.Errorf("failed to delete outbox entry: %w", err)
		}

		w.logger.Info("process resource DELETE operation completed successfully",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))

		return nil
	}

	// For UPSERT operations (CREATE/UPDATE), proceed with normal flow:
	// Get writer from registry to perform transaction
	writer, err := w.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get registry writer: %w", err)
	}
	defer writer.Abort() // Ensure cleanup if commit fails

	// Load the process resource
	var resourceScope ports.Scope

	switch item.ResourceType {
	case string(registry.TypeHostBinding):
		// Find the HostBinding
		var foundBinding *models.HostBinding
		err := w.loadResourceFromRegistry(ctx, item, func() error {
			reader, err := w.registry.Reader(ctx)
			if err != nil {
				return err
			}
			defer reader.Close()

			return reader.ListHostBindings(ctx, func(hb models.HostBinding) error {
				klog.Infof("[Worker] Comparing HostBinding UIDs: hb.Meta.UID=%s, item.ResourceID=%s, match=%t",
					hb.Meta.UID, item.ResourceID.String(), hb.Meta.UID == item.ResourceID.String())
				if hb.Meta.UID == item.ResourceID.String() {
					foundBinding = &hb
					return fmt.Errorf("found") // Stop iteration
				}
				return nil
			}, nil)
		})

		if err != nil && err.Error() != "found" {
			return fmt.Errorf("failed to load HostBinding: %w", err)
		}
		if foundBinding == nil {
			return fmt.Errorf("HostBinding not found: %s", item.ResourceID)
		}

		// Update conditions
		foundBinding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "All dependencies are ready")
		foundBinding.Meta.SetCondition(metav1.Condition{
			Type:    "PendingSync",
			Status:  metav1.ConditionFalse,
			Reason:  "AllDependenciesReady",
			Message: "",
		})

		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      foundBinding.Name,
			Namespace: foundBinding.Namespace,
		})

		// Update using registry writer
		if err := writer.SyncHostBindings(ctx, []models.HostBinding{*foundBinding}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			return fmt.Errorf("failed to update HostBinding: %w", err)
		}

	case string(registry.TypeNetworkBinding):
		// Find the NetworkBinding
		var foundBinding *models.NetworkBinding
		err := w.loadResourceFromRegistry(ctx, item, func() error {
			reader, err := w.registry.Reader(ctx)
			if err != nil {
				return err
			}
			defer reader.Close()

			return reader.ListNetworkBindings(ctx, func(nb models.NetworkBinding) error {
				klog.Infof("[Worker] Comparing NetworkBinding UIDs: nb.Meta.UID=%s, item.ResourceID=%s, match=%t",
					nb.Meta.UID, item.ResourceID.String(), nb.Meta.UID == item.ResourceID.String())
				if nb.Meta.UID == item.ResourceID.String() {
					foundBinding = &nb
					return fmt.Errorf("found") // Stop iteration
				}
				return nil
			}, nil)
		})

		if err != nil && err.Error() != "found" {
			return fmt.Errorf("failed to load NetworkBinding: %w", err)
		}
		klog.Infof("[Worker] After ListNetworkBindings: foundBinding=%v, err=%v", foundBinding != nil, err)
		if foundBinding == nil {
			return fmt.Errorf("NetworkBinding not found: %s", item.ResourceID)
		}

		// Update conditions
		foundBinding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "All dependencies are ready")
		foundBinding.Meta.SetCondition(metav1.Condition{
			Type:    "PendingSync",
			Status:  metav1.ConditionFalse,
			Reason:  "AllDependenciesReady",
			Message: "",
		})

		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      foundBinding.Name,
			Namespace: foundBinding.Namespace,
		})

		// Update using registry writer
		if err := writer.SyncNetworkBindings(ctx, []models.NetworkBinding{*foundBinding}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			return fmt.Errorf("failed to update NetworkBinding: %w", err)
		}

	case string(registry.TypeAddressGroupBinding):
		// Find the AddressGroupBinding
		var foundBinding *models.AddressGroupBinding
		err := w.loadResourceFromRegistry(ctx, item, func() error {
			reader, err := w.registry.Reader(ctx)
			if err != nil {
				return err
			}
			defer reader.Close()

			return reader.ListAddressGroupBindings(ctx, func(agb models.AddressGroupBinding) error {
				klog.Infof("[Worker] Comparing AddressGroupBinding UIDs: agb.Meta.UID=%s, item.ResourceID=%s, match=%t",
					agb.Meta.UID, item.ResourceID.String(), agb.Meta.UID == item.ResourceID.String())
				if agb.Meta.UID == item.ResourceID.String() {
					foundBinding = &agb
					return fmt.Errorf("found") // Stop iteration
				}
				return nil
			}, nil)
		})

		if err != nil && err.Error() != "found" {
			return fmt.Errorf("failed to load AddressGroupBinding: %w", err)
		}
		klog.Infof("[Worker] After ListAddressGroupBindings: foundBinding=%v, err=%v", foundBinding != nil, err)
		if foundBinding == nil {
			return fmt.Errorf("AddressGroupBinding not found: %s", item.ResourceID)
		}

		// Update conditions
		foundBinding.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "All dependencies are ready")
		foundBinding.Meta.SetCondition(metav1.Condition{
			Type:    "PendingSync",
			Status:  metav1.ConditionFalse,
			Reason:  "AllDependenciesReady",
			Message: "",
		})

		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      foundBinding.Name,
			Namespace: foundBinding.Namespace,
		})

		// Update using registry writer
		if err := writer.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{*foundBinding}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			return fmt.Errorf("failed to update AddressGroupBinding: %w", err)
		}

	default:
		return fmt.Errorf("unknown process resource type: %s", item.ResourceType)
	}

	// Commit the registry changes
	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	// Delete outbox entry
	if err := w.outboxRepo.Delete(ctx, item.ID); err != nil {
		return fmt.Errorf("failed to delete outbox entry: %w", err)
	}

	w.logger.Info("process resource marked ready successfully",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()))

	return nil
}

// loadResourceFromRegistry is a helper to load resources from registry with proper error handling
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
