package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	"time"
)

func (w *OutboxWorker) processEntityResource(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	startTime := time.Now()
	operation := string(item.Operation)
	w.logger.Info("processing entity resource",
		zap.String("resource_type", item.ResourceType),
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName),
		zap.String("operation", operation))
	if err := w.updatePendingSyncSyncing(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName, item.Attempts+1); err != nil {
		w.logger.Warn("failed to update PendingSync condition at sync start", zap.Error(err))
	}
	var resource interface{}
	var err error
	if item.Operation == domain.SyncOperationDelete {
		w.logger.Debug("DELETE operation detected, reconstructing resource from payload",
			zap.String("resource_type", item.ResourceType),
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))
		resource, err = w.reconstructResourceFromPayload(item)
		if err != nil {
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "payload_reconstruction_error", duration)
			return fmt.Errorf("failed to reconstruct resource from payload: %w", err)
		}
	} else {
		resource, err = w.loadEntityResource(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ports.ErrNotFound) {
				w.logger.Warn("resource not found (deleted)",
					zap.String("resource_type", item.ResourceType),
					zap.String("namespace", item.ResourceNamespace),
					zap.String("name", item.ResourceName))
				duration := time.Since(startTime)
				RecordProcessingFailure(item.ResourceType, operation, "resource_not_found", duration)
				return fmt.Errorf("%w: %w", ErrResourceDeleted, err)
			}
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "load_error", duration)
			return fmt.Errorf("failed to load resource: %w", err)
		}
	}
	if item.Delta != nil && len(item.Delta) > 0 {
		if err := applyDelta(resource, item.Operation, item.Delta); err != nil {
			w.logger.Error("failed to apply delta",
				zap.String("resource_type", item.ResourceType),
				zap.Error(err))
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "delta_error", duration)
			return fmt.Errorf("failed to apply delta: %w", err)
		}
		w.logger.Debug("applied delta",
			zap.String("resource_type", item.ResourceType),
			zap.String("operation", operation))
	}
	if item.Operation != domain.SyncOperationDelete {
		allReady, missingDeps, err := w.checkEntityDependencies(ctx, item.ResourceType, resource)
		if err != nil {
			w.logger.Error("failed to check entity dependencies",
				zap.String("resource_type", item.ResourceType),
				zap.Error(err))
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
	syncer, err := w.getSyncerForEntity(item.ResourceType)
	if err != nil {
		duration := time.Since(startTime)
		RecordProcessingFailure(item.ResourceType, operation, "syncer_error", duration)
		return fmt.Errorf("no syncer for resource type %s: %w", item.ResourceType, err)
	}
	syncCtx, cancel := context.WithTimeout(ctx, w.config.SGROUPTimeout)
	defer cancel()
	syncOperation := convertToSyncOperation(item.Operation)
	syncableEntity, ok := resource.(interfaces.SyncableEntity)
	if !ok {
		duration := time.Since(startTime)
		RecordProcessingFailure(item.ResourceType, operation, "type_assertion_error", duration)
		return fmt.Errorf("resource does not implement SyncableEntity interface: %T", resource)
	}
	if err := w.syncAndUpdateStatusAtomic(ctx, syncCtx, item, syncer, syncableEntity, syncOperation, startTime); err != nil {
		return err
	}
	if item.Operation != domain.SyncOperationDelete {
		if err := w.updatePendingSyncComplete(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName); err != nil {
			w.logger.Warn("failed to update PendingSync condition on success", zap.Error(err))
		}
	}
	duration := time.Since(startTime)
	RecordProcessingSuccess(item.ResourceType, operation, duration)
	w.logger.Info("entity resource processed successfully",
		zap.String("resource_type", item.ResourceType),
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName),
		zap.Duration("duration", duration))
	return nil
}
func (w *OutboxWorker) reconstructResourceFromPayload(item *domain.OutboxEntry) (interface{}, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	namespace, _ := payload["namespace"].(string)
	name, _ := payload["name"].(string)
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
func (w *OutboxWorker) loadEntityResource(
	ctx context.Context,
	resourceType string,
	namespace string,
	name string,
) (interface{}, error) {
	reader, err := w.registry.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()
	switch resourceType {
	case string(registry.TypeHost):
		var foundHost *models.Host
		err := reader.ListHosts(ctx, func(host models.Host) error {
			if host.Namespace == namespace && host.Name == name {
				foundHost = &host
				return fmt.Errorf("found")
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
				return fmt.Errorf("found")
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
				return fmt.Errorf("found")
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
				return fmt.Errorf("found")
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
				return fmt.Errorf("found")
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
func (w *OutboxWorker) getSyncerForEntity(resourceType string) (interfaces.EntitySyncer[interfaces.SyncableEntity], error) {
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
func (w *OutboxWorker) markEntityResourceReady(
	ctx context.Context,
	resourceType string,
	namespace string,
	name string,
	outboxID interface{},
) error {
	writer, err := w.registry.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()
	w.logger.Debug("markEntityResourceReady: got writer",
		zap.String("resource_type", resourceType),
		zap.String("namespace", namespace),
		zap.String("name", name))
	resource, err := w.loadEntityResource(ctx, resourceType, namespace, name)
	if err != nil {
		return fmt.Errorf("failed to load resource for update: %w", err)
	}
	w.logger.Debug("markEntityResourceReady: loaded resource",
		zap.String("resource_type", resourceType),
		zap.String("namespace", namespace),
		zap.String("name", name))
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
		w.logger.Info("[DIAG] markEntityResourceReady: AddressGroup - BEFORE SetReadyCondition",
			zap.String("namespace", r.Namespace),
			zap.String("name", r.Name),
			zap.Int("conditions_count", len(r.Meta.Conditions)),
			zap.Any("conditions_before", r.Meta.Conditions))
		startTime := time.Now()
		r.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Synced to SGROUP")
		r.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Successfully synced")
		setConditionDuration := time.Since(startTime)
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
	w.logger.Debug("markEntityResourceReady: marking outbox entry as SUCCESS",
		zap.String("outbox_id", outboxID.(uuid.UUID).String()))
	if err := w.outboxRepo.MarkCompleted(ctx, outboxID.(uuid.UUID)); err != nil {
		w.logger.Error("markEntityResourceReady: failed to mark outbox entry as completed",
			zap.String("outbox_id", outboxID.(uuid.UUID).String()),
			zap.Error(err))
		return fmt.Errorf("failed to mark outbox entry as completed: %w", err)
	}
	w.logger.Info("markEntityResourceReady: outbox entry marked as SUCCESS (preserved for debugging)",
		zap.String("resource_type", resourceType),
		zap.String("namespace", namespace),
		zap.String("name", name),
		zap.String("outbox_id", outboxID.(uuid.UUID).String()))
	return nil
}
func convertToSyncOperation(op domain.SyncOperation) types.SyncOperation {
	switch op {
	case domain.SyncOperationCreate:
		return types.SyncOperationUpsert
	case domain.SyncOperationUpdate:
		return types.SyncOperationUpsert
	case domain.SyncOperationDelete:
		return types.SyncOperationDelete
	default:
		return types.SyncOperationUpsert
	}
}

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
func (w *OutboxWorker) deleteResourceFromDB(ctx context.Context, item *domain.OutboxEntry) error {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
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
	cmdTag, err := tx.Exec(ctx, deleteQuery, item.ResourceNamespace, item.ResourceName)
	if err != nil {
		return fmt.Errorf("failed to delete %s from DB: %w", item.ResourceType, err)
	}
	w.logger.Debug("deleted resource from DB",
		zap.String("resource_type", item.ResourceType),
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName),
		zap.Int64("rows_affected", cmdTag.RowsAffected()))
	updateOutboxQuery := `UPDATE sync_outbox SET status = 'SUCCESS', updated_at = NOW() WHERE id = $1`
	_, err = tx.Exec(ctx, updateOutboxQuery, item.ID)
	if err != nil {
		return fmt.Errorf("failed to mark outbox entry as completed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit deletion transaction: %w", err)
	}
	w.logger.Info("resource and outbox entry deleted from DB",
		zap.String("resource_type", item.ResourceType),
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))
	return nil
}
func (w *OutboxWorker) syncAndUpdateStatusAtomic(
	ctx context.Context,
	syncCtx context.Context,
	item *domain.OutboxEntry,
	syncer interfaces.EntitySyncer[interfaces.SyncableEntity],
	syncableEntity interfaces.SyncableEntity,
	syncOperation types.SyncOperation,
	startTime time.Time,
) error {
	operation := string(item.Operation)
	if err := syncer.Sync(syncCtx, syncableEntity, syncOperation); err != nil {
		w.logger.Error("SGROUP sync failed",
			zap.String("resource_type", item.ResourceType),
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName),
			zap.Error(err))
		duration := time.Since(startTime)
		errorCategory := w.categorizeError(err)
		RecordProcessingFailure(item.ResourceType, operation, string(errorCategory), duration)
		return fmt.Errorf("SGROUP sync failed: %w", err)
	}
	w.logger.Info("SGROUP sync successful",
		zap.String("resource_type", item.ResourceType),
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))
	if item.Operation == domain.SyncOperationDelete {
		w.logger.Debug("DELETE operation - SGROUP sync complete, deleting from DB",
			zap.String("resource_type", item.ResourceType),
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))
		if err := w.deleteResourceFromDB(ctx, item); err != nil {
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "db_delete_error", duration)
			return fmt.Errorf("failed to delete resource from DB: %w", err)
		}
		w.logger.Info("DELETE operation complete - resource deleted from DB",
			zap.String("resource_type", item.ResourceType),
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))
	} else {
		if err := w.markEntityResourceReady(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName, item.ID); err != nil {
			w.logger.Error("failed to mark resource ready",
				zap.String("resource_type", item.ResourceType),
				zap.String("namespace", item.ResourceNamespace),
				zap.String("name", item.ResourceName),
				zap.Error(err))
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "db_update_error", duration)
			return fmt.Errorf("failed to mark resource ready: %w", err)
		}
	}
	return nil
}
func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}
