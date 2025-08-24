// // DEPRECATED: This file has been replaced by modular writers in the writers/ package.
// // The monolithic writer.go (571 lines) has been split into focused, maintainable files:
// // - writers/writer.go - Base writer struct + shared utilities
// // - writers/service.go - Service + ServiceAlias write operations
// // - writers/address_group.go - AddressGroup family write operations
// // - writers/shared_utils.go - Common utility functions
// //
// // This file is kept only for reference and should not be used.
// // The new writer.go delegates to the modular writers for backward compatibility.
package pg

//
//import (
//	"context"
//	"encoding/json"
//	"fmt"
//	"strings"
//
//	"github.com/jackc/pgx/v5"
//	"github.com/pkg/errors"
//
//	"netguard-pg-backend/internal/domain/models"
//	"netguard-pg-backend/internal/domain/ports"
//)
//
//// writer implements the PostgreSQL writer with transaction support
//type writer struct {
//	registry *Registry
//	tx       pgx.Tx
//	ctx      context.Context
//}
//
//// Close closes the writer
//func (w *writer) Close() error {
//	return nil // Transaction lifecycle managed by Commit/Abort
//}
//
//// Commit commits the transaction
//func (w *writer) Commit() error {
//	if w.tx == nil {
//		return errors.New("no active transaction")
//	}
//	return w.tx.Commit(w.ctx)
//}
//
//// Abort rolls back the transaction
//func (w *writer) Abort() {
//	if w.tx == nil {
//		// No active transaction, nothing to abort
//		return
//	}
//	_ = w.tx.Rollback(w.ctx) // Ignore rollback errors in Abort
//}
//
//// SyncServices implements hybrid sync strategy for services
//func (w *writer) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, opts ...ports.Option) error {
//	// TODO: Extract sync operation from options if needed for PostgreSQL-specific behavior
//	// For now, PostgreSQL implementation just does basic upsert operations
//
//	// Handle scoped sync - delete existing resources in scope first
//	if !scope.IsEmpty() {
//		if err := w.deleteServicesInScope(ctx, scope); err != nil {
//			return errors.Wrap(err, "failed to delete services in scope")
//		}
//	}
//
//	// Bulk upsert services
//	for _, service := range services {
//		if err := w.upsertService(ctx, service); err != nil {
//			return errors.Wrapf(err, "failed to upsert service %s", service.Key())
//		}
//	}
//
//	return nil
//}
//
//// upsertService inserts or updates a single service
//func (w *writer) upsertService(ctx context.Context, service models.Service) error {
//	// Create or update K8s metadata
//	resourceVersion, err := w.createOrUpdateK8sMetadata(ctx, service.Meta)
//	if err != nil {
//		return errors.Wrap(err, "failed to handle k8s metadata")
//	}
//
//	// Serialize ingress ports to JSONB
//	ingressPortsJSON, err := json.Marshal(service.IngressPorts)
//	if err != nil {
//		return errors.Wrap(err, "failed to marshal ingress ports")
//	}
//
//	query := `
//		INSERT INTO services (namespace, name, description, ingress_ports, resource_version)
//		VALUES ($1, $2, $3, $4, $5)
//		ON CONFLICT (namespace, name) DO UPDATE SET
//			description = EXCLUDED.description,
//			ingress_ports = EXCLUDED.ingress_ports,
//			resource_version = EXCLUDED.resource_version`
//
//	_, err = w.tx.Exec(ctx, query,
//		service.Namespace,
//		service.Name,
//		service.Description,
//		ingressPortsJSON,
//		resourceVersion,
//	)
//
//	if err != nil {
//		return errors.Wrapf(err, "failed to upsert service %s", service.Key())
//	}
//
//	return nil
//}
//
//// deleteServicesInScope deletes services within the specified scope
//func (w *writer) deleteServicesInScope(ctx context.Context, scope ports.Scope) error {
//	if scope.IsEmpty() {
//		return nil
//	}
//
//	switch s := scope.(type) {
//	case ports.ResourceIdentifierScope:
//		return w.deleteServicesByIdentifiers(ctx, s.Identifiers)
//	default:
//		return errors.New("unsupported scope type for services deletion")
//	}
//}
//
//// deleteServicesByIdentifiers deletes services by resource identifiers
//func (w *writer) deleteServicesByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
//	if len(identifiers) == 0 {
//		return nil
//	}
//
//	// Build WHERE conditions for bulk delete
//	var conditions []string
//	var args []interface{}
//	argIndex := 1
//
//	for _, id := range identifiers {
//		if id.Name == "" && id.Namespace != "" {
//			// Delete all services in namespace
//			conditions = append(conditions, fmt.Sprintf("namespace = $%d", argIndex))
//			args = append(args, id.Namespace)
//			argIndex++
//		} else {
//			// Delete specific service
//			conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", argIndex, argIndex+1))
//			args = append(args, id.Namespace, id.Name)
//			argIndex += 2
//		}
//	}
//
//	if len(conditions) == 0 {
//		return nil
//	}
//
//	// First, get resource_versions to delete metadata
//	selectQuery := fmt.Sprintf("SELECT resource_version FROM services WHERE %s", strings.Join(conditions, " OR "))
//	rows, err := w.tx.Query(ctx, selectQuery, args...)
//	if err != nil {
//		return errors.Wrap(err, "failed to query services for deletion")
//	}
//	defer rows.Close()
//
//	var resourceVersions []int64
//	for rows.Next() {
//		var rv int64
//		if err := rows.Scan(&rv); err != nil {
//			return errors.Wrap(err, "failed to scan resource version")
//		}
//		resourceVersions = append(resourceVersions, rv)
//	}
//
//	// Delete services (CASCADE will handle metadata)
//	deleteQuery := fmt.Sprintf("DELETE FROM services WHERE %s", strings.Join(conditions, " OR "))
//	_, err = w.tx.Exec(ctx, deleteQuery, args...)
//	if err != nil {
//		return errors.Wrap(err, "failed to delete services")
//	}
//
//	return nil
//}
//
//// SyncAddressGroups implements sync for address groups
//func (w *writer) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope, opts ...ports.Option) error {
//	// TODO: Extract sync operation from options if needed for PostgreSQL-specific behavior
//	// For now, PostgreSQL implementation just does basic upsert operations
//
//	// Handle scoped sync
//	if !scope.IsEmpty() {
//		if err := w.deleteAddressGroupsInScope(ctx, scope); err != nil {
//			return errors.Wrap(err, "failed to delete address groups in scope")
//		}
//	}
//
//	// Bulk upsert address groups
//	for _, ag := range addressGroups {
//		if err := w.upsertAddressGroup(ctx, ag); err != nil {
//			return errors.Wrapf(err, "failed to upsert address group %s", ag.Key())
//		}
//	}
//
//	return nil
//}
//
//// upsertAddressGroup inserts or updates a single address group
//func (w *writer) upsertAddressGroup(ctx context.Context, ag models.AddressGroup) error {
//	// Create or update K8s metadata
//	resourceVersion, err := w.createOrUpdateK8sMetadata(ctx, ag.Meta)
//	if err != nil {
//		return errors.Wrap(err, "failed to handle k8s metadata")
//	}
//
//	query := `
//		INSERT INTO address_groups (namespace, name, default_action, logs, trace, description, resource_version)
//		VALUES ($1, $2, $3, $4, $5, $6, $7)
//		ON CONFLICT (namespace, name) DO UPDATE SET
//			default_action = EXCLUDED.default_action,
//			logs = EXCLUDED.logs,
//			trace = EXCLUDED.trace,
//			description = EXCLUDED.description,
//			resource_version = EXCLUDED.resource_version`
//
//	_, err = w.tx.Exec(ctx, query,
//		ag.Namespace,
//		ag.Name,
//		string(ag.DefaultAction),
//		ag.Logs,
//		ag.Trace,
//		"", // description placeholder (AddressGroup doesn't have Description field)
//		resourceVersion,
//	)
//
//	if err != nil {
//		return errors.Wrapf(err, "failed to upsert address group %s", ag.Key())
//	}
//
//	return nil
//}
//
//// deleteAddressGroupsInScope deletes address groups within the specified scope
//func (w *writer) deleteAddressGroupsInScope(ctx context.Context, scope ports.Scope) error {
//	if scope.IsEmpty() {
//		return nil
//	}
//
//	switch s := scope.(type) {
//	case ports.ResourceIdentifierScope:
//		return w.deleteAddressGroupsByIdentifiers(ctx, s.Identifiers)
//	default:
//		return errors.New("unsupported scope type for address groups deletion")
//	}
//}
//
//// deleteAddressGroupsByIdentifiers deletes address groups by resource identifiers
//func (w *writer) deleteAddressGroupsByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
//	if len(identifiers) == 0 {
//		return nil
//	}
//
//	// Build WHERE conditions for bulk delete
//	var conditions []string
//	var args []interface{}
//	argIndex := 1
//
//	for _, id := range identifiers {
//		if id.Name == "" && id.Namespace != "" {
//			conditions = append(conditions, fmt.Sprintf("namespace = $%d", argIndex))
//			args = append(args, id.Namespace)
//			argIndex++
//		} else {
//			conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", argIndex, argIndex+1))
//			args = append(args, id.Namespace, id.Name)
//			argIndex += 2
//		}
//	}
//
//	if len(conditions) == 0 {
//		return nil
//	}
//
//	deleteQuery := fmt.Sprintf("DELETE FROM address_groups WHERE %s", strings.Join(conditions, " OR "))
//	_, err := w.tx.Exec(ctx, deleteQuery, args...)
//	if err != nil {
//		return errors.Wrap(err, "failed to delete address groups")
//	}
//
//	return nil
//}
//
//// createOrUpdateK8sMetadata handles K8s metadata creation/update
//func (w *writer) createOrUpdateK8sMetadata(ctx context.Context, meta models.Meta) (int64, error) {
//	// For new resources or updates, always create new metadata
//	labels := make(map[string]string)
//	if meta.Labels != nil {
//		labels = meta.Labels
//	}
//
//	annotations := make(map[string]string)
//	if meta.Annotations != nil {
//		annotations = meta.Annotations
//	}
//
//	// Note: Meta doesn't have Finalizers field, using empty slice for now
//	// TODO: If finalizers are needed, they should be added to Meta struct
//	finalizers := []string{}
//
//	return w.registry.CreateK8sMetadata(ctx, w.tx, labels, annotations, finalizers)
//}
//
//// SyncAddressGroupBindings implements hybrid sync strategy for address group bindings
//func (w *writer) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope, opts ...ports.Option) error {
//	// TODO: Extract sync operation from options if needed for PostgreSQL-specific behavior
//	// For now, PostgreSQL implementation just does basic upsert operations
//
//	// Handle scoped sync - delete existing resources in scope first
//	if !scope.IsEmpty() {
//		if err := w.deleteAddressGroupBindingsInScope(ctx, scope); err != nil {
//			return errors.Wrap(err, "failed to delete address group bindings in scope")
//		}
//	}
//
//	// Bulk upsert address group bindings
//	for _, binding := range bindings {
//		if err := w.upsertAddressGroupBinding(ctx, binding); err != nil {
//			return errors.Wrapf(err, "failed to upsert address group binding %s", binding.Key())
//		}
//	}
//
//	return nil
//}
//
//// upsertAddressGroupBinding inserts or updates a single address group binding
//func (w *writer) upsertAddressGroupBinding(ctx context.Context, binding models.AddressGroupBinding) error {
//	// Create or update K8s metadata
//	resourceVersion, err := w.createOrUpdateK8sMetadata(ctx, binding.Meta)
//	if err != nil {
//		return errors.Wrap(err, "failed to handle k8s metadata")
//	}
//
//	query := `
//		INSERT INTO address_group_bindings (namespace, name, service_namespace, service_name, address_group_namespace, address_group_name, resource_version)
//		VALUES ($1, $2, $3, $4, $5, $6, $7)
//		ON CONFLICT (namespace, name) DO UPDATE SET
//			service_namespace = EXCLUDED.service_namespace,
//			service_name = EXCLUDED.service_name,
//			address_group_namespace = EXCLUDED.address_group_namespace,
//			address_group_name = EXCLUDED.address_group_name,
//			resource_version = EXCLUDED.resource_version`
//
//	_, err = w.tx.Exec(ctx, query,
//		binding.Namespace,
//		binding.Name,
//		binding.ServiceRef.Namespace,
//		binding.ServiceRef.Name,
//		binding.AddressGroupRef.Namespace,
//		binding.AddressGroupRef.Name,
//		resourceVersion,
//	)
//
//	if err != nil {
//		return errors.Wrapf(err, "failed to upsert address group binding %s", binding.Key())
//	}
//
//	return nil
//}
//
//// deleteAddressGroupBindingsInScope deletes address group bindings within the specified scope
//func (w *writer) deleteAddressGroupBindingsInScope(ctx context.Context, scope ports.Scope) error {
//	if scope.IsEmpty() {
//		return nil
//	}
//
//	switch s := scope.(type) {
//	case ports.ResourceIdentifierScope:
//		return w.deleteAddressGroupBindingsByIdentifiers(ctx, s.Identifiers)
//	default:
//		return errors.New("unsupported scope type for address group bindings deletion")
//	}
//}
//
//// deleteAddressGroupBindingsByIdentifiers deletes address group bindings by resource identifiers
//func (w *writer) deleteAddressGroupBindingsByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
//	if len(identifiers) == 0 {
//		return nil
//	}
//
//	// Build WHERE conditions for bulk delete
//	var conditions []string
//	var args []interface{}
//	argIndex := 1
//
//	for _, id := range identifiers {
//		if id.Name == "" && id.Namespace != "" {
//			conditions = append(conditions, fmt.Sprintf("namespace = $%d", argIndex))
//			args = append(args, id.Namespace)
//			argIndex++
//		} else {
//			conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", argIndex, argIndex+1))
//			args = append(args, id.Namespace, id.Name)
//			argIndex += 2
//		}
//	}
//
//	if len(conditions) == 0 {
//		return nil
//	}
//
//	deleteQuery := fmt.Sprintf("DELETE FROM address_group_bindings WHERE %s", strings.Join(conditions, " OR "))
//	_, err := w.tx.Exec(ctx, deleteQuery, args...)
//	if err != nil {
//		return errors.Wrap(err, "failed to delete address group bindings")
//	}
//
//	return nil
//}
//
//// upsertAddressGroupPortMapping inserts or updates an address group port mapping
//func (w *writer) upsertAddressGroupPortMapping(ctx context.Context, mapping models.AddressGroupPortMapping) error {
//	// Create or update K8s metadata
//	resourceVersion, err := w.createOrUpdateK8sMetadata(ctx, mapping.Meta)
//	if err != nil {
//		return errors.Wrap(err, "failed to handle k8s metadata")
//	}
//
//	// Convert AccessPorts map to JSONB using custom marshaling
//	accessPortsJSON, err := marshalAccessPorts(mapping.AccessPorts)
//	if err != nil {
//		return errors.Wrap(err, "failed to marshal access ports to JSON")
//	}
//
//	query := `
//		INSERT INTO address_group_port_mappings (namespace, name, access_ports, resource_version)
//		VALUES ($1, $2, $3, $4)
//		ON CONFLICT (namespace, name) DO UPDATE SET
//			access_ports = EXCLUDED.access_ports,
//			resource_version = EXCLUDED.resource_version`
//
//	_, err = w.tx.Exec(ctx, query,
//		mapping.Namespace,
//		mapping.Name,
//		accessPortsJSON,
//		resourceVersion,
//	)
//	if err != nil {
//		return errors.Wrap(err, "failed to upsert address group port mapping")
//	}
//
//	return nil
//}
//
//// deleteAddressGroupPortMappingsInScope deletes address group port mappings within a scope
//func (w *writer) deleteAddressGroupPortMappingsInScope(ctx context.Context, scope ports.Scope) error {
//	switch s := scope.(type) {
//	case ports.ResourceIdentifierScope:
//		return w.deleteAddressGroupPortMappingsByIdentifiers(ctx, s.Identifiers)
//	default:
//		return errors.New("unsupported scope type for address group port mappings deletion")
//	}
//}
//
//// deleteAddressGroupPortMappingsByIdentifiers deletes address group port mappings by resource identifiers
//func (w *writer) deleteAddressGroupPortMappingsByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
//	if len(identifiers) == 0 {
//		return nil
//	}
//
//	// Build WHERE conditions for bulk delete
//	var conditions []string
//	var args []interface{}
//	argIndex := 1
//
//	for _, id := range identifiers {
//		if id.Name == "" && id.Namespace != "" {
//			conditions = append(conditions, fmt.Sprintf("namespace = $%d", argIndex))
//			args = append(args, id.Namespace)
//			argIndex++
//		} else if id.Name != "" && id.Namespace != "" {
//			conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", argIndex, argIndex+1))
//			args = append(args, id.Namespace, id.Name)
//			argIndex += 2
//		}
//	}
//
//	if len(conditions) == 0 {
//		return nil
//	}
//
//	deleteQuery := fmt.Sprintf("DELETE FROM address_group_port_mappings WHERE %s", strings.Join(conditions, " OR "))
//	_, err := w.tx.Exec(ctx, deleteQuery, args...)
//	if err != nil {
//		return errors.Wrap(err, "failed to delete address group port mappings")
//	}
//
//	return nil
//}
//
//func (w *writer) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope, opts ...ports.Option) error {
//	// TODO: Extract sync operation from options if needed for PostgreSQL-specific behavior
//
//	// Delete existing mappings in scope if not empty
//	if !scope.IsEmpty() {
//		if err := w.deleteAddressGroupPortMappingsInScope(ctx, scope); err != nil {
//			return errors.Wrap(err, "failed to delete address group port mappings in scope")
//		}
//	}
//
//	// Upsert each mapping
//	for _, mapping := range mappings {
//		if err := w.upsertAddressGroupPortMapping(ctx, mapping); err != nil {
//			return errors.Wrapf(err, "failed to upsert address group port mapping %s", mapping.Key())
//		}
//	}
//
//	return nil
//}
//
//func (w *writer) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope, opts ...ports.Option) error {
//	return errors.New("RuleS2S sync not implemented yet - Phase 4")
//}
//
//func (w *writer) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope, opts ...ports.Option) error {
//	return errors.New("ServiceAliases sync not implemented yet - Phase 5")
//}
//
//func (w *writer) SyncAddressGroupBindingPolicies(ctx context.Context, policies []models.AddressGroupBindingPolicy, scope ports.Scope, opts ...ports.Option) error {
//	return errors.New("AddressGroupBindingPolicies sync not implemented yet - Phase 6")
//}
//
//func (w *writer) SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope, opts ...ports.Option) error {
//	return errors.New("IEAgAgRules sync not implemented yet - Phase 7")
//}
//
//func (w *writer) SyncNetworks(ctx context.Context, networks []models.Network, scope ports.Scope, opts ...ports.Option) error {
//	return errors.New("Networks sync not implemented yet - Phase 8")
//}
//
//func (w *writer) SyncNetworkBindings(ctx context.Context, bindings []models.NetworkBinding, scope ports.Scope, opts ...ports.Option) error {
//	return errors.New("NetworkBindings sync not implemented yet - Phase 8")
//}
//
//// Delete methods (placeholders)
//func (w *writer) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	return w.deleteServicesByIdentifiers(ctx, ids)
//}
//
//func (w *writer) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	return w.deleteAddressGroupsByIdentifiers(ctx, ids)
//}
//
//func (w *writer) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	return w.deleteAddressGroupBindingsByIdentifiers(ctx, ids)
//}
//
//func (w *writer) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	return w.deleteAddressGroupPortMappingsByIdentifiers(ctx, ids)
//}
//
//func (w *writer) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	return errors.New("DeleteRuleS2SByIDs not implemented yet - Phase 4")
//}
//
//func (w *writer) DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	return errors.New("DeleteServiceAliasesByIDs not implemented yet - Phase 5")
//}
//
//func (w *writer) DeleteAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	return errors.New("DeleteAddressGroupBindingPoliciesByIDs not implemented yet - Phase 6")
//}
//
//func (w *writer) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	return errors.New("DeleteIEAgAgRulesByIDs not implemented yet - Phase 7")
//}
//
//func (w *writer) DeleteNetworksByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	return errors.New("DeleteNetworksByIDs not implemented yet - Phase 8")
//}
//
//func (w *writer) DeleteNetworkBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	return errors.New("DeleteNetworkBindingsByIDs not implemented yet - Phase 8")
//}
