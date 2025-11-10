package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/domain/registry"
	v1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/syncers"
	"netguard-pg-backend/internal/sync/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// Prefer the trigger-captured payload when it exists so the worker processes the exact snapshot
	// rather than a potentially diverged record read from the database later.
	if item.Payload != nil && len(item.Payload) > 0 {
		w.logger.Debug("reconstructing resource from payload (deterministic sync)",
			zap.String("resource_type", item.ResourceType),
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName),
			zap.String("operation", operation))
		resource, err = w.reconstructResourceFromPayload(ctx, item)
		if err != nil {
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "payload_reconstruction_error", duration)
			return fmt.Errorf("failed to reconstruct resource from payload: %w", err)
		}

		if service, ok := resource.(*models.Service); ok {
			w.logger.Debug("reconstructed service from payload",
				zap.String("namespace", service.Namespace),
				zap.String("name", service.Name),
				zap.Int("aggregated_ags_count", len(service.AggregatedAddressGroups)),
				zap.Any("aggregated_ags", service.AggregatedAddressGroups))
		}
	} else {
		// Fallback to database (for legacy entries or empty payloads)
		w.logger.Debug("no payload available, loading from database (fallback)",
			zap.String("resource_type", item.ResourceType),
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))
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
	if item.Operation == domain.SyncOperationUpdate {
		exists, err := w.entityExists(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName)
		if err != nil {
			w.logger.Error("failed to verify entity existence",
				zap.String("resource_type", item.ResourceType),
				zap.String("namespace", item.ResourceNamespace),
				zap.String("name", item.ResourceName),
				zap.Error(err))
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "existence_check_error", duration)
			return fmt.Errorf("failed to verify resource existence: %w", err)
		}
		if !exists {
			w.logger.Info("skipping UPDATE for deleted resource",
				zap.String("resource_type", item.ResourceType),
				zap.String("namespace", item.ResourceNamespace),
				zap.String("name", item.ResourceName))
			if err := w.outboxRepo.MarkCompleted(ctx, item.ID); err != nil {
				w.logger.Error("failed to mark outbox entry completed while skipping deleted resource",
					zap.String("resource_type", item.ResourceType),
					zap.String("namespace", item.ResourceNamespace),
					zap.String("name", item.ResourceName),
					zap.Error(err))
				duration := time.Since(startTime)
				RecordProcessingFailure(item.ResourceType, operation, "mark_completed_error", duration)
				return fmt.Errorf("failed to mark outbox entry completed: %w", err)
			}
			duration := time.Since(startTime)
			RecordProcessingSuccess(item.ResourceType, operation, duration)
			return nil
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
		w.logger.Debug("DELETE operation detected, checking for binding coordination",
			zap.String("resource_type", item.ResourceType))

		// Case 2: Coordinate binding deletion BEFORE syncing entity DELETE to SGROUP
		// This ensures bindings are deleted first, preventing CASCADE from bypassing triggers
		if err := w.coordinateEntityDeleteWithBindings(ctx, item); err != nil {
			// If waiting for bindings to be deleted, don't treat as error
			if IsWaitingForSync(err) {
				w.logger.Debug("waiting for bindings to be deleted before entity deletion",
					zap.String("resource_type", item.ResourceType),
					zap.String("namespace", item.ResourceNamespace),
					zap.String("name", item.ResourceName))
				duration := time.Since(startTime)
				RecordProcessingFailure(item.ResourceType, operation, "waiting_for_bindings", duration)
				return ErrWaitingForSync
			}
			// Other errors are failures
			duration := time.Since(startTime)
			RecordProcessingFailure(item.ResourceType, operation, "binding_coordination_error", duration)
			return fmt.Errorf("failed to coordinate binding deletion: %w", err)
		}

		w.logger.Debug("binding coordination complete or not needed, proceeding with entity DELETE",
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
func (w *OutboxWorker) reconstructResourceFromPayload(ctx context.Context, item *domain.OutboxEntry) (interface{}, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	namespace, _ := payload["namespace"].(string)
	name, _ := payload["name"].(string)
	switch item.ResourceType {
	case string(registry.TypeHost):
		uuid := getStringFromMap(payload, "uuid")
		host := models.NewHost(name, namespace, uuid)

		host.HostName = getStringFromMap(payload, "host_name_sync")
		host.AddressGroupName = getStringFromMap(payload, "address_group_name")
		host.IsBound = getBoolFromMap(payload, "is_bound")

		agNs := getStringFromMap(payload, "address_group_ref_namespace")
		agName := getStringFromMap(payload, "address_group_ref_name")
		if agName == "" {
			agName = host.AddressGroupName
		}
		if agName != "" {
			host.AddressGroupRef = &v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       agName,
				},
				Namespace: agNs,
			}
		}

		bindNs := getStringFromMap(payload, "binding_ref_namespace")
		bindName := getStringFromMap(payload, "binding_ref_name")
		if bindName != "" {
			host.BindingRef = &v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "HostBinding",
					Name:       bindName,
				},
				Namespace: bindNs,
			}
		}

		if ipRaw, ok := payload["ip_list"].([]interface{}); ok {
			ips := make([]string, 0, len(ipRaw))
			for _, ipVal := range ipRaw {
				if ipStr, ok := ipVal.(string); ok {
					ips = append(ips, ipStr)
				}
			}
			host.SetIpList(ips)
		}

		return host, nil
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
		var base *models.AddressGroup
		if item.Operation != domain.SyncOperationCreate {
			if existing, err := w.loadEntityResource(ctx, item.ResourceType, namespace, name); err == nil {
				if existingAG, ok := existing.(*models.AddressGroup); ok {
					if copyRes := existingAG.DeepCopy(); copyRes != nil {
						if copyAG, ok := copyRes.(*models.AddressGroup); ok {
							base = copyAG
						}
					}
				}
			}
		}

		ag := &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      name,
				},
			},
		}

		if base != nil {
			ag.DefaultAction = base.DefaultAction
			ag.Logs = base.Logs
			ag.Trace = base.Trace
			ag.Networks = base.Networks
			ag.AggregatedHosts = base.AggregatedHosts
		}

		if actionRaw, ok := payload["defaultAction"]; ok {
			if actionStr, ok := actionRaw.(string); ok && actionStr != "" {
				switch models.RuleAction(actionStr) {
				case models.ActionDrop:
					ag.DefaultAction = models.ActionDrop
				default:
					ag.DefaultAction = models.ActionAccept
				}
			}
		}

		if logsRaw, ok := payload["logs"]; ok {
			if val, ok := logsRaw.(bool); ok {
				ag.Logs = val
			}
		}
		if traceRaw, ok := payload["trace"]; ok {
			if val, ok := traceRaw.(bool); ok {
				ag.Trace = val
			}
		}

		if networksRaw, ok := payload["networks"]; ok {
			switch typed := networksRaw.(type) {
			case []interface{}:
				networkItems := make([]models.NetworkItem, 0, len(typed))
				for _, entry := range typed {
					switch netVal := entry.(type) {
					case map[string]interface{}:
						item := models.NetworkItem{
							Name:       getStringFromMap(netVal, "name"),
							Namespace:  getStringFromMap(netVal, "namespace"),
							CIDR:       getStringFromMap(netVal, "cidr"),
							ApiVersion: getStringFromMap(netVal, "apiVersion"),
							Kind:       getStringFromMap(netVal, "kind"),
						}
						if item.ApiVersion == "" {
							item.ApiVersion = "netguard.sgroups.io/v1beta1"
						}
						if item.Kind == "" {
							item.Kind = "Network"
						}
						if item.Name != "" {
							networkItems = append(networkItems, item)
						}
					case string:
						// legacy payloads may store networks as []string
						if netVal != "" {
							networkItems = append(networkItems, models.NetworkItem{Name: netVal})
						}
					}
				}
				ag.Networks = networkItems
			case []string:
				networkItems := make([]models.NetworkItem, 0, len(typed))
				for _, netName := range typed {
					if netName == "" {
						continue
					}
					networkItems = append(networkItems, models.NetworkItem{Name: netName})
				}
				ag.Networks = networkItems
			}
		}
		return ag, nil
	case string(registry.TypeService):
		var base *models.Service
		if item.Operation != domain.SyncOperationCreate {
			if existing, err := w.loadEntityResource(ctx, item.ResourceType, namespace, name); err == nil {
				if existingSvc, ok := existing.(*models.Service); ok {
					copySvc := *existingSvc
					base = &copySvc
				}
			} else {
				w.logger.Debug("failed to load existing service state",
					zap.String("namespace", namespace),
					zap.String("name", name),
					zap.Error(err))
			}
		}

		w.logger.Debug("service payload snapshot",
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.String("operation", string(item.Operation)),
			zap.Any("payload", payload))

		svc := &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      name,
				},
			},
		}

		if base != nil {
			w.logger.Debug("service base state before merge",
				zap.String("namespace", namespace),
				zap.String("name", name),
				zap.Int("base_ingress_port_count", len(base.IngressPorts)),
				zap.Any("base_ingress_ports", base.IngressPorts),
				zap.Int("base_address_group_spec_count", len(base.AddressGroups)),
				zap.Int("base_aggregated_address_group_count", len(base.AggregatedAddressGroups)),
				zap.Any("base_aggregated_address_groups", base.AggregatedAddressGroups))
			svc.Description = base.Description
			svc.IngressPorts = base.IngressPorts
			svc.AddressGroups = base.AddressGroups
			svc.AggregatedAddressGroups = base.AggregatedAddressGroups
		} else {
			w.logger.Debug("service base state missing (treating as fresh payload)",
				zap.String("namespace", namespace),
				zap.String("name", name))
		}

		if desc, ok := payload["description"].(string); ok {
			svc.Description = desc
		}

		if portsRaw, ok := payload["ingress_ports"]; ok {
			w.logger.Debug("service payload ingress_ports raw",
				zap.String("namespace", namespace),
				zap.String("name", name),
				zap.String("raw_type", fmt.Sprintf("%T", portsRaw)),
				zap.Any("raw", portsRaw))
			svc.IngressPorts = parseIngressPorts(portsRaw)
			w.logger.Debug("service ingress_ports parsed",
				zap.String("namespace", namespace),
				zap.String("name", name),
				zap.Int("parsed_count", len(svc.IngressPorts)),
				zap.Any("parsed_ports", svc.IngressPorts))
			if len(svc.IngressPorts) == 0 {
				w.logger.Warn("service ingress_ports parsed empty",
					zap.String("namespace", namespace),
					zap.String("name", name))
			}
		} else {
			w.logger.Debug("service payload ingress_ports missing",
				zap.String("namespace", namespace),
				zap.String("name", name))
		}

		if agSpecRaw, ok := payload["address_groups"]; ok {
			w.logger.Debug("service payload address_groups raw",
				zap.String("namespace", namespace),
				zap.String("name", name),
				zap.String("raw_type", fmt.Sprintf("%T", agSpecRaw)),
				zap.Any("raw", agSpecRaw))
			svc.AddressGroups = parseServiceAddressGroups(agSpecRaw)
			w.logger.Debug("service address_groups parsed",
				zap.String("namespace", namespace),
				zap.String("name", name),
				zap.Int("parsed_count", len(svc.AddressGroups)),
				zap.Any("parsed_address_groups", svc.AddressGroups))
		} else {
			w.logger.Debug("service payload address_groups missing",
				zap.String("namespace", namespace),
				zap.String("name", name))
		}

		if agsRaw, ok := payload["aggregated_address_groups"]; ok {
			w.logger.Debug("service payload aggregated_address_groups raw",
				zap.String("namespace", namespace),
				zap.String("name", name),
				zap.String("raw_type", fmt.Sprintf("%T", agsRaw)),
				zap.Any("raw", agsRaw))
			if agsArray, ok := agsRaw.([]interface{}); ok {
				aggregatedAGs := make([]models.AddressGroupReference, 0, len(agsArray))
				for _, entry := range agsArray {
					if agMap, ok := entry.(map[string]interface{}); ok {
						var agRef models.AddressGroupReference
						if refMap, ok := agMap["ref"].(map[string]interface{}); ok {
							agRef.Ref = v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: getStringFromMap(refMap, "apiVersion"),
									Kind:       getStringFromMap(refMap, "kind"),
									Name:       getStringFromMap(refMap, "name"),
								},
								Namespace: getStringFromMap(refMap, "namespace"),
							}
						}
						if source, ok := agMap["source"].(string); ok {
							agRef.Source = models.AddressGroupRegistrationSource(source)
						}
						aggregatedAGs = append(aggregatedAGs, agRef)
					}
				}
				svc.AggregatedAddressGroups = aggregatedAGs
				w.logger.Debug("service aggregated_address_groups parsed",
					zap.String("namespace", namespace),
					zap.String("name", name),
					zap.Int("parsed_count", len(svc.AggregatedAddressGroups)),
					zap.Any("parsed_aggregated_address_groups", svc.AggregatedAddressGroups))
			}
		} else {
			w.logger.Debug("service payload aggregated_address_groups missing",
				zap.String("namespace", namespace),
				zap.String("name", name))
		}

		w.logger.Debug("service reconstruction complete",
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.String("operation", string(item.Operation)),
			zap.String("description", svc.Description),
			zap.Int("final_ingress_port_count", len(svc.IngressPorts)),
			zap.Any("final_ingress_ports", svc.IngressPorts),
			zap.Int("final_address_group_spec_count", len(svc.AddressGroups)),
			zap.Any("final_address_groups_spec", svc.AddressGroups),
			zap.Int("final_aggregated_address_group_count", len(svc.AggregatedAddressGroups)),
			zap.Any("final_aggregated_address_groups", svc.AggregatedAddressGroups))

		return svc, nil
	case string(registry.TypeSvcSvcRule):
		var base *models.SvcSvcRule
		if item.Operation != domain.SyncOperationCreate {
			if existing, err := w.loadEntityResource(ctx, item.ResourceType, namespace, name); err == nil {
				if existingRule, ok := existing.(*models.SvcSvcRule); ok {
					temp := *existingRule
					base = &temp
				}
			}
		}
		rule := &models.SvcSvcRule{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      name,
				},
			},
		}
		if base != nil {
			rule.ServiceFromRef = base.ServiceFromRef
			rule.ServiceToRef = base.ServiceToRef
			rule.Action = base.Action
			rule.Priority = base.Priority
			rule.Logs = base.Logs
			rule.Trace = base.Trace
		}
		serviceFromData, hasFrom := payload["service_from_ref"].(map[string]interface{})
		if !hasFrom {
			serviceFromData, hasFrom = payload["service_from"].(map[string]interface{})
		}
		if hasFrom {
			rule.ServiceFromRef = v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: getStringFromMap(serviceFromData, "apiVersion"),
					Kind:       getStringFromMap(serviceFromData, "kind"),
					Name:       getStringFromMap(serviceFromData, "name"),
				},
				Namespace: getStringFromMap(serviceFromData, "namespace"),
			}
		}
		serviceToData, hasTo := payload["service_to_ref"].(map[string]interface{})
		if !hasTo {
			serviceToData, hasTo = payload["service_to"].(map[string]interface{})
		}
		if hasTo {
			rule.ServiceToRef = v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: getStringFromMap(serviceToData, "apiVersion"),
					Kind:       getStringFromMap(serviceToData, "kind"),
					Name:       getStringFromMap(serviceToData, "name"),
				},
				Namespace: getStringFromMap(serviceToData, "namespace"),
			}
		}
		if actionRaw, ok := payload["action"].(string); ok && actionRaw != "" {
			rule.Action = models.RuleAction(actionRaw)
		}
		if priorityRaw, ok := payload["priority"]; ok {
			switch v := priorityRaw.(type) {
			case float64:
				rule.Priority = int32(v)
			case int:
				rule.Priority = int32(v)
			case int32:
				rule.Priority = v
			case int64:
				rule.Priority = int32(v)
			}
		}
		if logsRaw, ok := payload["logs"].(bool); ok {
			rule.Logs = logsRaw
		}
		if traceRaw, ok := payload["trace"].(bool); ok {
			rule.Trace = traceRaw
		}
		if rule.ServiceFromRef.Namespace == "" && rule.ServiceFromRef.Name != "" {
			rule.ServiceFromRef.Namespace = namespace
		}
		return rule, nil
	case string(registry.TypeSvcFqdnRule):
		var base *models.SvcFqdnRule
		if item.Operation != domain.SyncOperationCreate {
			if existing, err := w.loadEntityResource(ctx, item.ResourceType, namespace, name); err == nil {
				if existingRule, ok := existing.(*models.SvcFqdnRule); ok {
					temp := *existingRule
					base = &temp
				}
			}
		}
		rule := &models.SvcFqdnRule{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      name,
				},
			},
		}
		if base != nil {
			rule.ServiceFromRef = base.ServiceFromRef
			rule.FQDN = base.FQDN
			rule.Transport = base.Transport
			rule.Ports = append([]models.FqdnPortSpec(nil), base.Ports...)
			rule.Logs = base.Logs
			rule.Trace = base.Trace
			rule.Action = base.Action
			rule.Priority = base.Priority
			rule.Description = base.Description
		}
		if refData, ok := payload["service_from_ref"].(map[string]interface{}); ok {
			rule.ServiceFromRef = v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: getStringFromMap(refData, "apiVersion"),
					Kind:       getStringFromMap(refData, "kind"),
					Name:       getStringFromMap(refData, "name"),
				},
				Namespace: getStringFromMap(refData, "namespace"),
			}
		}
		if rule.ServiceFromRef.Namespace == "" && rule.ServiceFromRef.Name != "" {
			rule.ServiceFromRef.Namespace = namespace
		}
		if fqdn, ok := payload["fqdn"].(string); ok && fqdn != "" {
			rule.FQDN = fqdn
		}
		if transportRaw, ok := payload["transport"].(string); ok && transportRaw != "" {
			switch strings.ToUpper(transportRaw) {
			case string(models.UDP):
				rule.Transport = models.UDP
			default:
				rule.Transport = models.TCP
			}
		}
		if portsRaw, ok := payload["ports"]; ok {
			parsedPorts := make([]models.FqdnPortSpec, 0)
			switch typed := portsRaw.(type) {
			case []interface{}:
				for _, entry := range typed {
					if entryMap, ok := entry.(map[string]interface{}); ok {
						parsedPorts = append(parsedPorts, models.FqdnPortSpec{
							Port: getStringFromMap(entryMap, "port"),
						})
					} else if entryStr, ok := entry.(string); ok {
						parsedPorts = append(parsedPorts, models.FqdnPortSpec{Port: entryStr})
					}
				}
			case []map[string]interface{}:
				for _, entryMap := range typed {
					parsedPorts = append(parsedPorts, models.FqdnPortSpec{
						Port: getStringFromMap(entryMap, "port"),
					})
				}
			case string:
				if typed != "" {
					parsedPorts = append(parsedPorts, models.FqdnPortSpec{Port: typed})
				}
			}
			rule.Ports = parsedPorts
		}
		if logsRaw, ok := payload["logs"].(bool); ok {
			rule.Logs = logsRaw
		}
		if traceRaw, ok := payload["trace"].(bool); ok {
			rule.Trace = traceRaw
		}
		if actionRaw, ok := payload["action"].(string); ok && actionRaw != "" {
			rule.Action = models.RuleAction(actionRaw)
		}
		if priorityRaw, ok := payload["priority"]; ok {
			switch v := priorityRaw.(type) {
			case float64:
				rule.Priority = int32(v)
			case int:
				rule.Priority = int32(v)
			case int32:
				rule.Priority = v
			case int64:
				rule.Priority = int32(v)
			}
		}
		if desc, ok := payload["description"].(string); ok {
			rule.Description = desc
		}
		if rule.Transport == "" {
			rule.Transport = models.TCP
		}
		return rule, nil
	case string(registry.TypeHostBinding):
		// For DELETE operations on HostBinding (process resource), we only need basic identification
		// The coordinated deletion handler will load the full resource if needed
		return &models.HostBinding{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      name,
				},
			},
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
	case string(registry.TypeSvcFqdnRule):
		rule, err := reader.GetSvcFqdnRuleByID(ctx, models.ResourceIdentifier{Namespace: namespace, Name: name})
		if err != nil {
			return nil, err
		}
		return rule, nil
	default:
		return nil, fmt.Errorf("unknown resource type: %s", resourceType)
	}
}

func (w *OutboxWorker) entityExists(
	ctx context.Context,
	resourceType string,
	namespace string,
	name string,
) (bool, error) {
	_, err := w.loadEntityResource(ctx, resourceType, namespace, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ports.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
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
	case string(registry.TypeSvcFqdnRule):
		return w.svcFqdnRuleSyncer, nil
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
	case *models.SvcFqdnRule:
		r.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "Synced to SGROUP")
		r.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "Successfully synced")
		resourceScope = ports.NewResourceIdentifierScope(models.ResourceIdentifier{
			Name:      r.Name,
			Namespace: r.Namespace,
		})
		if err := writer.SyncSvcFqdnRules(ctx, []models.SvcFqdnRule{*r}, resourceScope, ports.ConditionOnlyOperation{}); err != nil {
			return fmt.Errorf("failed to update svcfqdn rule: %w", err)
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
	svcFqdnRuleSyncer  *syncers.SvcFqdnRuleSyncer
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
	case types.SyncSubjectTypeSvcFqdnRules:
		return a.svcFqdnRuleSyncer.Sync(ctx, entity, operation)
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
	case types.SyncSubjectTypeSvcFqdnRules:
		return a.svcFqdnRuleSyncer.SyncBatch(ctx, entities, operation)
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
		if err := w.deleteAddressGroupPortMapping(ctx, tx, item.ResourceNamespace, item.ResourceName); err != nil {
			return fmt.Errorf("failed to delete address group port mapping: %w", err)
		}
		deleteQuery = `DELETE FROM address_groups WHERE namespace = $1 AND name = $2`
	case string(registry.TypeService):
		deleteQuery = `DELETE FROM services WHERE namespace = $1 AND name = $2`
	case string(registry.TypeSvcSvcRule):
		deleteQuery = `DELETE FROM svc_svc_rules WHERE namespace = $1 AND name = $2`
	case string(registry.TypeSvcFqdnRule):
		deleteQuery = `DELETE FROM svc_fqdn_rules WHERE namespace = $1 AND name = $2`
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
	updateOutboxQuery := `UPDATE sync_outbox SET status = 'SUCCESS', processed_at = NOW(), updated_at = NOW() WHERE id = $1`
	_, err = tx.Exec(ctx, updateOutboxQuery, item.ID)
	if err != nil {
		return fmt.Errorf("failed to mark outbox entry as completed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit deletion transaction: %w", err)
	}
	w.logger.Debug("resource and outbox entry deleted from DB",
		zap.String("resource_type", item.ResourceType),
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))
	return nil
}

func (w *OutboxWorker) deleteAddressGroupPortMapping(ctx context.Context, tx pgx.Tx, namespace, name string) error {
	w.logger.Debug("️ [DELETE_FROM_DB] Deleting AddressGroupPortMapping prior to AddressGroup removal",
		zap.String("namespace", namespace),
		zap.String("name", name))

	if _, err := tx.Exec(ctx, `DELETE FROM address_group_port_mappings WHERE namespace = $1 AND name = $2`, namespace, name); err != nil {
		return err
	}

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
		w.handlePostDeleteRegeneration(ctx, item, syncableEntity)
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

func (w *OutboxWorker) handlePostDeleteRegeneration(ctx context.Context, item *domain.OutboxEntry, entity interfaces.SyncableEntity) {
	if w.portMappingRegenerator == nil {
		return
	}

	switch item.ResourceType {
	case string(registry.TypeService):
		service, ok := entity.(*models.Service)
		if !ok || service == nil {
			return
		}
		w.regeneratePortMappingsAfterServiceDeletion(ctx, service)
	}
}

func (w *OutboxWorker) regeneratePortMappingsAfterServiceDeletion(ctx context.Context, service *models.Service) {
	if w.portMappingRegenerator == nil || service == nil {
		return
	}

	serviceID := models.NewResourceIdentifier(service.Name, models.WithNamespace(service.Namespace))
	if err := w.portMappingRegenerator.RegeneratePortMappingsForService(ctx, serviceID); err != nil {
		w.logger.Error("failed to regenerate service port mappings after deletion",
			zap.String("service", serviceID.Key()),
			zap.Error(err))
	}

	seen := make(map[string]struct{})
	for _, ag := range service.AddressGroups {
		name := ag.Name
		if name == "" {
			continue
		}
		namespace := ag.Namespace
		if namespace == "" {
			namespace = service.Namespace
		}
		agID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
		key := agID.Key()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := w.portMappingRegenerator.RegeneratePortMappingsForAddressGroup(ctx, agID); err != nil {
			w.logger.Error("failed to regenerate address group port mapping after service deletion",
				zap.String("address_group", key),
				zap.Error(err))
		}
	}
}
func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func getBoolFromMap(m map[string]interface{}, key string) bool {
	if val, ok := m[key].(bool); ok {
		return val
	}
	return false
}

func parseIngressPorts(raw interface{}) []models.IngressPort {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]models.IngressPort, 0, len(arr))
	for _, entry := range arr {
		if portMap, ok := entry.(map[string]interface{}); ok {
			result = append(result, models.IngressPort{
				Protocol:    models.TransportProtocol(getStringFromMap(portMap, "protocol")),
				Port:        getStringFromMap(portMap, "port"),
				Description: getStringFromMap(portMap, "description"),
			})
		}
	}
	return result
}

func parseServiceAddressGroups(raw interface{}) []models.AddressGroupRef {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]models.AddressGroupRef, 0, len(arr))
	for _, entry := range arr {
		if agMap, ok := entry.(map[string]interface{}); ok {
			ref := v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: getStringFromMap(agMap, "apiVersion"),
					Kind:       getStringFromMap(agMap, "kind"),
					Name:       getStringFromMap(agMap, "name"),
				},
				Namespace: getStringFromMap(agMap, "namespace"),
			}
			if ref.ObjectReference.APIVersion == "" {
				ref.ObjectReference.APIVersion = "netguard.sgroups.io/v1beta1"
			}
			if ref.ObjectReference.Kind == "" {
				ref.ObjectReference.Kind = "AddressGroup"
			}
			if ref.ObjectReference.Name != "" {
				result = append(result, models.AddressGroupRef(ref))
			}
		}
	}
	return result
}

// coordinateEntityDeleteWithBindings handles Case 2 (DELETE Entity → CASCADE bindings)
// for resource types that have binding relationships:
//   - Host → HostBinding
//   - Network → NetworkBinding
//   - AddressGroup → HostBinding + NetworkBinding
//   - Service → AddressGroupBinding (stub for future)
//
// Algorithm:
//  1. Find all bindings for this entity
//  2. If no bindings → return nil (proceed with sync)
//  3. If bindings exist:
//     a. Mark each binding with deletion_timestamp (soft delete)
//     b. Check if all bindings are physically deleted
//     c. If not → return ErrWaitingForSync (wait for next poll)
//     d. If yes → return nil (proceed with sync)
//
// See: docs/architecture/COORDINATED_BINDING_DELETION.md - Case 2
func (w *OutboxWorker) coordinateEntityDeleteWithBindings(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	// Dispatch to specific handler based on resource type
	switch item.ResourceType {
	case string(registry.TypeHost):
		return w.coordinateHostDelete(ctx, item)

	case string(registry.TypeNetwork):
		return w.coordinateNetworkDelete(ctx, item)

	case string(registry.TypeAddressGroup):
		return w.coordinateAddressGroupDelete(ctx, item)

	case string(registry.TypeService):
		return w.coordinateServiceDelete(ctx, item)

	default:
		// No binding coordination needed for other types
		return nil
	}
}

// coordinateHostDelete handles Case 2 for Host deletion
// See: docs/architecture/COORDINATED_BINDING_DELETION.md - Case 2
func (w *OutboxWorker) coordinateHostDelete(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	w.logger.Info("coordinating Host DELETE with HostBinding CASCADE",
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))

	// Step 1: Find all HostBindings for this Host
	bindings, err := w.findHostBindingsForHost(ctx, item.ResourceNamespace, item.ResourceName)
	if err != nil {
		return fmt.Errorf("finding host bindings: %w", err)
	}

	if len(bindings) == 0 {
		w.logger.Debug("no HostBindings found, proceeding with Host DELETE",
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))
		return nil // No bindings to coordinate
	}

	w.logger.Info("found HostBindings that need coordination",
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName),
		zap.Int("binding_count", len(bindings)))

	// Step 2: Mark all bindings for deletion (soft delete)
	for _, binding := range bindings {
		if err := w.markBindingForDeletion(ctx, binding); err != nil {
			return fmt.Errorf("marking binding for deletion: %w", err)
		}
	}

	// Step 3: Check if all bindings are physically deleted
	allDeleted, err := w.checkAllHostBindingsDeleted(ctx, bindings)
	if err != nil {
		return fmt.Errorf("checking binding deletion status: %w", err)
	}

	if !allDeleted {
		w.logger.Debug("HostBindings not yet physically deleted, waiting",
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName),
			zap.Int("binding_count", len(bindings)))
		return ErrWaitingForSync // Don't increment attempts
	}

	w.logger.Info("all HostBindings deleted, proceeding with Host DELETE",
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))

	return nil // Ready to proceed with Host deletion
}

// coordinateNetworkDelete handles Case 2 for Network deletion
// See: docs/architecture/COORDINATED_BINDING_DELETION.md - Case 2
func (w *OutboxWorker) coordinateNetworkDelete(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	w.logger.Info("coordinating Network DELETE with NetworkBinding CASCADE",
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))

	// Step 1: Find all NetworkBindings for this Network
	bindings, err := w.findNetworkBindingsForNetwork(ctx, item.ResourceNamespace, item.ResourceName)
	if err != nil {
		return fmt.Errorf("finding network bindings: %w", err)
	}

	if len(bindings) == 0 {
		w.logger.Debug("no NetworkBindings found, proceeding with Network DELETE",
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))
		return nil // No bindings to coordinate
	}

	w.logger.Info("found NetworkBindings that need coordination",
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName),
		zap.Int("binding_count", len(bindings)))

	// Step 2: Mark all bindings for deletion (soft delete)
	for _, binding := range bindings {
		if err := w.markBindingForDeletion(ctx, binding); err != nil {
			return fmt.Errorf("marking binding for deletion: %w", err)
		}
	}

	// Step 3: Check if all bindings are physically deleted
	allDeleted, err := w.checkAllNetworkBindingsDeleted(ctx, bindings)
	if err != nil {
		return fmt.Errorf("checking binding deletion status: %w", err)
	}

	if !allDeleted {
		w.logger.Debug("NetworkBindings not yet physically deleted, waiting",
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName),
			zap.Int("binding_count", len(bindings)))
		return ErrWaitingForSync // Don't increment attempts
	}

	w.logger.Info("all NetworkBindings deleted, proceeding with Network DELETE",
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))

	return nil // Ready to proceed with Network deletion
}

// coordinateAddressGroupDelete handles Case 4 for AddressGroup deletion
// AddressGroup can have BOTH HostBindings AND NetworkBindings
// See: docs/architecture/COORDINATED_BINDING_DELETION.md - Case 4
func (w *OutboxWorker) coordinateAddressGroupDelete(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	w.logger.Info("coordinating AddressGroup DELETE with binding CASCADE",
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))

	// Step 1: Find HostBindings
	hostBindings, err := w.findHostBindingsForAddressGroup(ctx,
		item.ResourceNamespace, item.ResourceName)
	if err != nil {
		return fmt.Errorf("finding host bindings: %w", err)
	}

	// Step 2: Find NetworkBindings
	netBindings, err := w.findNetworkBindingsForAddressGroup(ctx,
		item.ResourceNamespace, item.ResourceName)
	if err != nil {
		return fmt.Errorf("finding network bindings: %w", err)
	}

	totalBindings := len(hostBindings) + len(netBindings)
	if totalBindings == 0 {
		w.logger.Debug("no bindings found, proceeding with AddressGroup DELETE",
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))
		return nil // No bindings
	}

	w.logger.Info("found bindings for AddressGroup",
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName),
		zap.Int("host_bindings", len(hostBindings)),
		zap.Int("network_bindings", len(netBindings)))

	// Step 3: Mark all for deletion
	for _, binding := range hostBindings {
		if err := w.markBindingForDeletion(ctx, binding); err != nil {
			return fmt.Errorf("marking host binding for deletion: %w", err)
		}
	}
	for _, binding := range netBindings {
		if err := w.markBindingForDeletion(ctx, binding); err != nil {
			return fmt.Errorf("marking network binding for deletion: %w", err)
		}
	}

	// Step 4: Check all deleted
	hostDeleted, err := w.checkAllHostBindingsDeleted(ctx, hostBindings)
	if err != nil {
		return fmt.Errorf("checking host binding deletion: %w", err)
	}
	netDeleted, err := w.checkAllNetworkBindingsDeleted(ctx, netBindings)
	if err != nil {
		return fmt.Errorf("checking network binding deletion: %w", err)
	}

	if !hostDeleted || !netDeleted {
		w.logger.Debug("bindings not yet physically deleted, waiting",
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName),
			zap.Bool("host_deleted", hostDeleted),
			zap.Bool("net_deleted", netDeleted))
		return ErrWaitingForSync
	}

	w.logger.Info("all bindings deleted for AddressGroup",
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))
	return nil
}

// coordinateServiceDelete handles Case 4 for Service deletion
// Service can have AddressGroupBindings
// TODO: Implement when AddressGroupBinding coordination is added
func (w *OutboxWorker) coordinateServiceDelete(
	ctx context.Context,
	item *domain.OutboxEntry,
) error {
	w.logger.Info("coordinating Service DELETE with dependency cascade",
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))

	pending := false

	// Step 1: AddressGroupBindings
	bindings, err := w.findAddressGroupBindingsForService(ctx, item.ResourceNamespace, item.ResourceName)
	if err != nil {
		return fmt.Errorf("finding address group bindings: %w", err)
	}

	if len(bindings) == 0 {
		w.logger.Debug("no AddressGroupBindings found for Service",
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))
	} else {
		w.logger.Info("found AddressGroupBindings that need coordination",
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName),
			zap.Int("binding_count", len(bindings)))

		for _, binding := range bindings {
			if err := w.markBindingForDeletion(ctx, binding); err != nil {
				return fmt.Errorf("marking binding for deletion: %w", err)
			}
		}

		allBindingsDeleted, err := w.checkAllAddressGroupBindingsDeleted(ctx, bindings)
		if err != nil {
			return fmt.Errorf("checking binding deletion status: %w", err)
		}

		if !allBindingsDeleted {
			w.logger.Debug("AddressGroupBindings not yet physically deleted, waiting",
				zap.String("namespace", item.ResourceNamespace),
				zap.String("name", item.ResourceName),
				zap.Int("binding_count", len(bindings)))
			pending = true
		}
	}

	// Step 2: SvcSvcRule dependencies
	rules, err := w.findSvcSvcRulesForService(ctx, item.ResourceNamespace, item.ResourceName)
	if err != nil {
		return fmt.Errorf("finding SvcSvcRule dependencies: %w", err)
	}

	if len(rules) == 0 {
		w.logger.Debug("no SvcSvcRules referencing Service",
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))
	} else {
		w.logger.Info("found SvcSvcRules that need coordination",
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName),
			zap.Int("rule_count", len(rules)))

		for _, rule := range rules {
			if err := w.markSvcSvcRuleForDeletion(ctx, rule); err != nil {
				return fmt.Errorf("marking SvcSvcRule for deletion: %w", err)
			}
		}

		allRulesDeleted, err := w.checkAllSvcSvcRulesDeleted(ctx, rules)
		if err != nil {
			return fmt.Errorf("checking SvcSvcRule deletion status: %w", err)
		}

		if !allRulesDeleted {
			w.logger.Debug("SvcSvcRules not yet physically deleted, waiting",
				zap.String("namespace", item.ResourceNamespace),
				zap.String("name", item.ResourceName),
				zap.Int("rule_count", len(rules)))
			pending = true
		}
	}

	// Step 3: SvcFqdnRule dependencies
	fqdnRules, err := w.findSvcFqdnRulesForService(ctx, item.ResourceNamespace, item.ResourceName)
	if err != nil {
		return fmt.Errorf("finding SvcFqdnRule dependencies: %w", err)
	}

	if len(fqdnRules) == 0 {
		w.logger.Debug("no SvcFqdnRules referencing Service",
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName))
	} else {
		w.logger.Info("found SvcFqdnRules that need coordination",
			zap.String("namespace", item.ResourceNamespace),
			zap.String("name", item.ResourceName),
			zap.Int("rule_count", len(fqdnRules)))

		for _, rule := range fqdnRules {
			if err := w.markSvcFqdnRuleForDeletion(ctx, rule); err != nil {
				return fmt.Errorf("marking SvcFqdnRule for deletion: %w", err)
			}
		}

		allFqdnRulesDeleted, err := w.checkAllSvcFqdnRulesDeleted(ctx, fqdnRules)
		if err != nil {
			return fmt.Errorf("checking SvcFqdnRule deletion status: %w", err)
		}

		if !allFqdnRulesDeleted {
			w.logger.Debug("SvcFqdnRules not yet physically deleted, waiting",
				zap.String("namespace", item.ResourceNamespace),
				zap.String("name", item.ResourceName),
				zap.Int("rule_count", len(fqdnRules)))
			pending = true
		}
	}

	if pending {
		return ErrWaitingForSync
	}

	w.logger.Info("all dependent resources deleted, proceeding with Service DELETE",
		zap.String("namespace", item.ResourceNamespace),
		zap.String("name", item.ResourceName))

	return nil
}
