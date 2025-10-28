package worker

import (
	"context"
	"encoding/json"
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
	Type      string `json:"type"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
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
		w.logger.Warn("process resource has no affected resources",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))
		return w.markProcessResourceReady(ctx, item)
	}

	w.logger.Debug("checking affected resources",
		zap.String("resource_type", item.ResourceType),
		zap.Int("affected_count", len(affectedResources)))

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
	w.logger.Info("marking process resource ready",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()),
		zap.String("operation", string(item.Operation)))

	if item.Operation == domain.SyncOperationDelete {
		w.logger.Info("DELETE operation for process resource - skipping condition update, NOT deleting outbox entry (debugging)",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))

		// TEMPORARY: Don't delete for debugging
		// if err := w.outboxRepo.Delete(ctx, item.ID); err != nil {
		// 	return fmt.Errorf("failed to delete outbox entry: %w", err)
		// }

		w.logger.Info("process resource DELETE operation completed successfully (outbox preserved)",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()))

		return nil
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
			return fmt.Errorf("HostBinding not found: %s", item.ResourceID)
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
			return fmt.Errorf("NetworkBinding not found: %s", item.ResourceID)
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
			return fmt.Errorf("AddressGroupBinding not found: %s", item.ResourceID)
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

	// TEMPORARY: Don't delete for debugging
	// if err := w.outboxRepo.Delete(ctx, item.ID); err != nil {
	// 	return fmt.Errorf("failed to delete outbox entry: %w", err)
	// }

	w.logger.Info("process resource marked ready successfully (outbox preserved for debugging)",
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
