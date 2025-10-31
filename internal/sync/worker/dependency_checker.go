package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/registry"
)

// isResourceReady checks if a specific resource has Ready=True condition
// Queries k8s_metadata table to check if the resource has a Ready condition with status True
//
// Returns:
// - true if resource exists and has Ready=True
// - false if resource doesn't exist or Ready is not True
// - error if database query fails
func (w *OutboxWorker) isResourceReady(
	ctx context.Context,
	resourceType, namespace, name string,
) (bool, error) {
	// Map resource type to table name
	var tableName string
	switch resourceType {
	case string(registry.TypeHost):
		tableName = "hosts"
	case string(registry.TypeAddressGroup):
		tableName = "address_groups"
	case string(registry.TypeNetwork):
		tableName = "networks"
	case string(registry.TypeService):
		tableName = "services"
	case string(registry.TypeHostBinding):
		tableName = "host_bindings"
	case string(registry.TypeNetworkBinding):
		tableName = "network_bindings"
	default:
		return false, fmt.Errorf("unknown resource type: %s", resourceType)
	}

	// Query k8s_metadata via the resource table's resource_version foreign key
	// Check if conditions contains a Ready condition with status True
	query := fmt.Sprintf(`
		SELECT EXISTS(
			SELECT 1
			FROM %s r
			JOIN k8s_metadata km ON r.resource_version = km.resource_version
			WHERE r.namespace = $1
			  AND r.name = $2
			  AND km.conditions @> '[{"type":"Ready","status":"True"}]'::jsonb
		)
	`, tableName)

	var ready bool
	err := w.pool.QueryRow(ctx, query, namespace, name).Scan(&ready)
	if err != nil {
		return false, fmt.Errorf("failed to check resource readiness for %s/%s/%s: %w",
			resourceType, namespace, name, err)
	}

	w.logger.Debug("checked resource readiness",
		zap.String("resource_type", resourceType),
		zap.String("namespace", namespace),
		zap.String("name", name),
		zap.Bool("ready", ready))

	return ready, nil
}

// ensureOutboxEntryExists creates an Outbox entry for a pending dependency if it doesn't exist
// This ensures that the dependency will be synced/processed eventually
//
// Uses INSERT ... ON CONFLICT DO NOTHING for idempotency
// If entry already exists, this is a no-op (safe to call multiple times)
func (w *OutboxWorker) ensureOutboxEntryExists(
	ctx context.Context,
	resource AffectedResource,
) error {
	// First, check if resource exists and get its UID
	resourceUIDStr, err := w.getResourceUID(ctx, resource.Type, resource.Namespace, resource.Name)
	if err != nil {
		return fmt.Errorf("failed to get resource UID: %w", err)
	}

	// Parse UID string to UUID
	resourceUID, err := uuid.Parse(resourceUIDStr)
	if err != nil {
		return fmt.Errorf("failed to parse resource UID: %w", err)
	}

	// Determine target system based on resource type
	targetSystem := domain.TargetSystemSGROUP
	resourceDef, exists := registry.GetResourceDefinition(registry.ResourceType(resource.Type))
	if exists {
		switch resourceDef.TargetSystem {
		case registry.TargetSGROUP:
			targetSystem = domain.TargetSystemSGROUP
		case registry.TargetInternal:
			targetSystem = domain.TargetSystemInternal
		}
	}

	// Create outbox entry using the repository
	// The repository handles ON CONFLICT logic (UPSERT)
	entry := &domain.OutboxEntry{
		ResourceType: resource.Type,
		ResourceID:   resourceUID,
		Operation:    domain.SyncOperationUpdate,
		TargetSystem: targetSystem,
		Payload:      []byte(fmt.Sprintf(`{"metadata":{"namespace":"%s","name":"%s"}}`, resource.Namespace, resource.Name)),
		Status:       domain.OutboxStatusPending,
		MaxRetries:   20, // Standard retry count
	}

	// Validate before creating
	if err := entry.Validate(); err != nil {
		return fmt.Errorf("invalid outbox entry: %w", err)
	}

	// Create or update entry (repository handles UPSERT)
	if err := w.outboxRepo.Create(ctx, entry); err != nil {
		return fmt.Errorf("failed to create outbox entry: %w", err)
	}

	w.logger.Info("ensured outbox entry exists for dependency",
		zap.String("resource_type", resource.Type),
		zap.String("namespace", resource.Namespace),
		zap.String("name", resource.Name))

	return nil
}

// getResourceUID retrieves the UID for a specific resource
// The UID is needed to create a proper Outbox entry
func (w *OutboxWorker) getResourceUID(
	ctx context.Context,
	resourceType, namespace, name string,
) (uid string, err error) {
	// Map resource type to table name
	var tableName string
	switch resourceType {
	case string(registry.TypeHost):
		tableName = "hosts"
	case string(registry.TypeAddressGroup):
		tableName = "address_groups"
	case string(registry.TypeNetwork):
		tableName = "networks"
	case string(registry.TypeService):
		tableName = "services"
	case string(registry.TypeHostBinding):
		tableName = "host_bindings"
	case string(registry.TypeNetworkBinding):
		tableName = "network_bindings"
	default:
		return "", fmt.Errorf("unknown resource type: %s", resourceType)
	}

	// Query to get UID from k8s_metadata
	query := fmt.Sprintf(`
		SELECT km.uid
		FROM %s r
		JOIN k8s_metadata km ON r.resource_version = km.resource_version
		WHERE r.namespace = $1 AND r.name = $2
	`, tableName)

	err = w.pool.QueryRow(ctx, query, namespace, name).Scan(&uid)
	if err != nil {
		return "", fmt.Errorf("failed to get UID for %s/%s/%s: %w",
			resourceType, namespace, name, err)
	}

	return uid, nil
}

// getPendingResources filters a list of affected resources to return only those not yet Ready
// This is useful for logging and condition messages
func (w *OutboxWorker) getPendingResources(
	ctx context.Context,
	affectedResources []AffectedResource,
) ([]AffectedResource, error) {
	var pending []AffectedResource

	for _, resource := range affectedResources {
		ready, err := w.isResourceReady(ctx, resource.Type, resource.Namespace, resource.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to check readiness for %s/%s/%s: %w",
				resource.Type, resource.Namespace, resource.Name, err)
		}

		if !ready {
			pending = append(pending, resource)
		}
	}

	return pending, nil
}

// waitForDependencies is a helper that checks if all dependencies are ready
// Returns list of pending dependencies, or empty slice if all ready
func (w *OutboxWorker) waitForDependencies(
	ctx context.Context,
	item *domain.OutboxEntry,
) (pendingResources []string, allReady bool, err error) {
	// Parse affected resources
	var affectedResources []AffectedResource
	if len(item.AffectsResources) > 0 {
		if err := json.Unmarshal(item.AffectsResources, &affectedResources); err != nil {
			return nil, false, fmt.Errorf("failed to parse affected resources: %w", err)
		}
	}

	if len(affectedResources) == 0 {
		// No dependencies - consider as all ready
		return nil, true, nil
	}

	// Check each dependency
	pending := []string{}
	for _, affected := range affectedResources {
		ready, err := w.isResourceReady(ctx, affected.Type, affected.Namespace, affected.Name)
		if err != nil {
			return nil, false, fmt.Errorf("failed to check resource readiness: %w", err)
		}

		if !ready {
			pending = append(pending, fmt.Sprintf("%s/%s", affected.Type, affected.Name))
		}
	}

	return pending, len(pending) == 0, nil
}

// ============================================================================
// Entity Dependency Checking (for embedded references)
// ============================================================================
// These functions handle dependency checking for Entity resources that have
// embedded references (e.g., AddressGroup.AggregatedHosts, Network.AddressGroupRef,
// Service.AggregatedAddressGroups).
//
// Unlike Process resources (which use AffectsResources), Entity resources
// sync themselves but require their embedded dependencies to be Ready first.
// ============================================================================

// EntityDependency represents a dependency from an Entity resource
type EntityDependency struct {
	Type      string
	Name      string
	Namespace string
	Reason    string // Why this dependency exists (for logging)
}

// checkEntityDependencies checks if all embedded dependencies for an Entity resource are Ready
// Returns (allReady, missingDependencies, error)
func (w *OutboxWorker) checkEntityDependencies(
	ctx context.Context,
	resourceType string,
	resource interface{},
) (bool, []EntityDependency, error) {
	// Extract embedded dependencies from resource
	dependencies, err := w.extractEntityDependencies(resourceType, resource)
	if err != nil {
		return false, nil, fmt.Errorf("failed to extract entity dependencies: %w", err)
	}

	// If no dependencies, return immediately
	if len(dependencies) == 0 {
		w.logger.Debug("no embedded dependencies for entity resource",
			zap.String("resource_type", resourceType))
		return true, nil, nil
	}

	w.logger.Debug("checking entity dependencies",
		zap.String("resource_type", resourceType),
		zap.Int("dependency_count", len(dependencies)))

	// Check each dependency
	var notReady []EntityDependency
	for _, dep := range dependencies {
		ready, err := w.isResourceReady(ctx, dep.Type, dep.Namespace, dep.Name)
		if err != nil {
			return false, nil, fmt.Errorf("failed to check dependency %s/%s/%s: %w",
				dep.Type, dep.Namespace, dep.Name, err)
		}

		if !ready {
			w.logger.Debug("entity dependency not ready",
				zap.String("dep_type", dep.Type),
				zap.String("dep_namespace", dep.Namespace),
				zap.String("dep_name", dep.Name),
				zap.String("reason", dep.Reason))
			notReady = append(notReady, dep)
		}
	}

	if len(notReady) > 0 {
		return false, notReady, nil
	}

	w.logger.Debug("all entity dependencies ready",
		zap.String("resource_type", resourceType),
		zap.Int("dependency_count", len(dependencies)))

	return true, nil, nil
}

// extractEntityDependencies extracts embedded dependencies from a resource based on its type
func (w *OutboxWorker) extractEntityDependencies(
	resourceType string,
	resource interface{},
) ([]EntityDependency, error) {
	switch resourceType {
	case string(registry.TypeAddressGroup):
		return w.extractAddressGroupDependencies(resource)
	case string(registry.TypeNetwork):
		return w.extractNetworkDependencies(resource)
	case string(registry.TypeService):
		return w.extractServiceDependencies(resource)
	case string(registry.TypeHost):
		// FIXED: Hosts HAVE dependency when bound to AddressGroup!
		// When IsBound=true, Host includes SgName in ToSGroupsProto()
		return w.extractHostDependencies(resource)
	case string(registry.TypeSvcSvcRule):
		// SvcSvcRule depends on ServiceFromRef and ServiceToRef (both Services must be Ready)
		return w.extractSvcSvcRuleDependencies(resource)
	default:
		return nil, fmt.Errorf("unknown entity resource type: %s", resourceType)
	}
}

// extractHostDependencies extracts AddressGroup dependency from Host
// Host depends on AddressGroup when IsBound=true (1:1 relationship via AddressGroupRef)
// This is critical because Host.ToSGroupsProto() includes SgName when bound
func (w *OutboxWorker) extractHostDependencies(
	resource interface{},
) ([]EntityDependency, error) {
	// Type assert to *models.Host
	host, ok := resource.(*models.Host)
	if !ok {
		return nil, fmt.Errorf("invalid resource type for Host extraction: %T (expected *models.Host)", resource)
	}

	// Host depends on AddressGroup ONLY when bound (IsBound=true)
	// When not bound, Host syncs without SgName field (no dependency)
	if !host.IsBound || host.AddressGroupRef == nil {
		// Host is not bound - no dependency
		w.logger.Debug("host is not bound to AddressGroup, no dependency",
			zap.String("host_namespace", host.Namespace),
			zap.String("host_name", host.Name),
			zap.Bool("is_bound", host.IsBound))
		return nil, nil
	}

	// Host is bound - depends on the referenced AddressGroup
	// This AddressGroup must be Ready before Host can sync (because SgName is included)
	// Note: AddressGroup is in the same namespace as Host (cross-namespace references not supported)
	deps := []EntityDependency{
		{
			Type:      string(registry.TypeAddressGroup),
			Name:      host.AddressGroupRef.Name,
			Namespace: host.Namespace, // AddressGroup is in same namespace as Host
			Reason:    fmt.Sprintf("AddressGroup referenced in Host.AddressGroupRef (IsBound=true, sent as SgName='%s/%s' to SGROUP)", host.Namespace, host.AddressGroupRef.Name),
		},
	}

	w.logger.Debug("extracted Host dependencies",
		zap.String("host_namespace", host.Namespace),
		zap.String("host_name", host.Name),
		zap.String("ag_ref", fmt.Sprintf("%s/%s", host.Namespace, host.AddressGroupRef.Name)))

	return deps, nil
}

// extractSvcSvcRuleDependencies extracts Service dependencies from SvcSvcRule
// SvcSvcRule depends on both ServiceFromRef and ServiceToRef (both Services must be Ready)
func (w *OutboxWorker) extractSvcSvcRuleDependencies(
	resource interface{},
) ([]EntityDependency, error) {
	// Type assert to *models.SvcSvcRule
	rule, ok := resource.(*models.SvcSvcRule)
	if !ok {
		return nil, fmt.Errorf("invalid resource type for SvcSvcRule extraction: %T (expected *models.SvcSvcRule)", resource)
	}

	// SvcSvcRule depends on two Services: ServiceFromRef and ServiceToRef
	deps := []EntityDependency{
		{
			Type:      string(registry.TypeService),
			Name:      rule.ServiceFromRef.Name,
			Namespace: rule.ServiceFromRef.Namespace,
			Reason:    fmt.Sprintf("Service referenced in SvcSvcRule.ServiceFromRef (sent as SvcFrom='%s/%s' to SGROUP)", rule.ServiceFromRef.Namespace, rule.ServiceFromRef.Name),
		},
		{
			Type:      string(registry.TypeService),
			Name:      rule.ServiceToRef.Name,
			Namespace: rule.ServiceToRef.Namespace,
			Reason:    fmt.Sprintf("Service referenced in SvcSvcRule.ServiceToRef (sent as SvcTo='%s/%s' to SGROUP)", rule.ServiceToRef.Namespace, rule.ServiceToRef.Name),
		},
	}

	w.logger.Debug("extracted SvcSvcRule dependencies",
		zap.String("rule_namespace", rule.Namespace),
		zap.String("rule_name", rule.Name),
		zap.String("service_from", fmt.Sprintf("%s/%s", rule.ServiceFromRef.Namespace, rule.ServiceFromRef.Name)),
		zap.String("service_to", fmt.Sprintf("%s/%s", rule.ServiceToRef.Namespace, rule.ServiceToRef.Name)))

	return deps, nil
}

// extractAddressGroupDependencies extracts dependencies for AddressGroup
// IMPORTANT: AddressGroup does NOT depend on Hosts!
//
// Architecture rationale:
// - AggregatedHosts is a payload field for K8s API status display, NOT a dependency
// - AddressGroup.ToSGroupsProto() does NOT include Hosts in the SGROUP sync payload
// - SGROUP architecture uses ONE-WAY dependency: Host → AddressGroup (via SgName)
// - Hosts synchronize independently and reference their AddressGroup
// - Making AG depend on Hosts creates CIRCULAR DEPENDENCY deadlock:
//   - Host waits for AG.Ready=True
//   - AG waits for Host.Ready=True
//   - → Deadlock! All operations (CREATE/UPDATE/DELETE) hang forever
//
// Therefore: AddressGroup has NO dependencies (or optionally depends only on Networks)
func (w *OutboxWorker) extractAddressGroupDependencies(
	resource interface{},
) ([]EntityDependency, error) {
	// Type assert to *models.AddressGroup
	ag, ok := resource.(*models.AddressGroup)
	if !ok {
		return nil, fmt.Errorf("invalid resource type for AddressGroup extraction: %T (expected *models.AddressGroup)", resource)
	}

	// AddressGroup has NO dependencies on Hosts (see comment above)
	// AggregatedHosts is for status display only, not for SGROUP sync

	// Future: Could add dependency on Networks if AG.Networks is used in sync
	// For now, AddressGroup is independent
	var deps []EntityDependency

	w.logger.Debug("extracted AddressGroup dependencies",
		zap.String("ag_namespace", ag.Namespace),
		zap.String("ag_name", ag.Name),
		zap.Int("dependency_count", len(deps))) // Should be 0

	return deps, nil
}

// extractNetworkDependencies extracts AddressGroup dependency from Network
// Network depends on AddressGroup (1:1 relationship)
func (w *OutboxWorker) extractNetworkDependencies(
	resource interface{},
) ([]EntityDependency, error) {
	// Type assert to *models.Network
	network, ok := resource.(*models.Network)
	if !ok {
		return nil, fmt.Errorf("invalid resource type for Network extraction: %T (expected *models.Network)", resource)
	}

	// Network depends on AddressGroup (1:1 relationship via AddressGroupRef)
	if network.AddressGroupRef == nil {
		// No AddressGroup reference - no dependency
		w.logger.Debug("network has no AddressGroup reference",
			zap.String("network_namespace", network.Namespace),
			zap.String("network_name", network.Name))
		return nil, nil
	}

	deps := []EntityDependency{
		{
			Type:      string(registry.TypeAddressGroup),
			Name:      network.AddressGroupRef.Name,
			Namespace: network.Namespace, // AddressGroup is in same namespace as Network
			Reason:    "AddressGroup referenced in Network.AddressGroupRef (1:1 link)",
		},
	}

	w.logger.Debug("extracted Network dependencies",
		zap.String("network_namespace", network.Namespace),
		zap.String("network_name", network.Name),
		zap.String("ag_ref", fmt.Sprintf("%s/%s", network.Namespace, network.AddressGroupRef.Name)))

	return deps, nil
}

// extractServiceDependencies extracts AddressGroup dependencies from Service
// Service depends on all AddressGroups in AggregatedAddressGroups
func (w *OutboxWorker) extractServiceDependencies(
	resource interface{},
) ([]EntityDependency, error) {
	// Type assert to *models.Service
	service, ok := resource.(*models.Service)
	if !ok {
		return nil, fmt.Errorf("invalid resource type for Service extraction: %T (expected *models.Service)", resource)
	}

	// Service depends on all AddressGroups in AggregatedAddressGroups
	// These are used in ToSGroupsProto() for sg_names field
	var deps []EntityDependency
	for _, agRef := range service.AggregatedAddressGroups {
		deps = append(deps, EntityDependency{
			Type:      string(registry.TypeAddressGroup),
			Name:      agRef.Ref.Name,
			Namespace: agRef.Ref.Namespace,
			Reason:    fmt.Sprintf("AddressGroup referenced in Service.AggregatedAddressGroups (source: %s)", agRef.Source),
		})
	}

	w.logger.Debug("extracted Service dependencies",
		zap.String("service_namespace", service.Namespace),
		zap.String("service_name", service.Name),
		zap.Int("ag_count", len(deps)))

	return deps, nil
}
