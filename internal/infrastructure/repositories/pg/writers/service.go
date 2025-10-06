package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// addressGroupRefJSON represents the JSON structure for address group references in the database
// This avoids importing K8s types in the repository layer
type addressGroupRefJSON struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

// SyncServices implements hybrid sync strategy for services
func (w *Writer) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, opts ...ports.Option) error {
	// Extract sync operation from options (like address_group.go)
	// This was MISSING and caused PATCH operations to not update resource content!
	syncOp := models.SyncOpUpsert // Default operation
	isConditionOnly := false

	for _, opt := range opts {
		if syncOption, ok := opt.(ports.SyncOption); ok {
			syncOp = syncOption.Operation
		}
		if _, ok := opt.(ports.ConditionOnlyOperation); ok {
			isConditionOnly = true
		}
	}

	// For condition-only operations, only update k8s_metadata conditions
	if isConditionOnly {
		for _, service := range services {
			if err := w.updateServiceConditionsOnly(ctx, service); err != nil {
				return errors.Wrapf(err, "failed to update conditions for service %s/%s", service.Namespace, service.Name)
			}
		}
		return nil
	}

	// Handle scoped sync - delete existing resources in scope first (for non-DELETE operations)
	// This matches the logic from address_group.go that works correctly
	if !scope.IsEmpty() && syncOp != models.SyncOpDelete {
		if err := w.deleteServicesInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete services in scope")
		}
	}

	// Handle operations based on sync operation type
	// This was COMPLETELY MISSING and is why PATCH operations didn't work!
	switch syncOp {
	case models.SyncOpDelete:
		// For DELETE operations, delete the specific services
		var identifiers []models.ResourceIdentifier
		for _, service := range services {
			identifiers = append(identifiers, service.SelfRef.ResourceIdentifier)
		}
		if err := w.deleteServicesByIdentifiers(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to delete services")
		}
	case models.SyncOpUpsert, models.SyncOpFullSync:
		// For UPSERT/FULLSYNC operations, upsert all provided services
		for i := range services {
			// For UPDATE operations, preserve existing UID from database
			// Only call TouchOnCreate() for truly new resources
			// Check if this service already exists and get its UID
			if services[i].Meta.UID == "" {
				existingUID, err := w.getExistingServiceUID(ctx, services[i].Namespace, services[i].Name)
				if err == nil && existingUID != "" {
					// Resource exists, preserve existing UID
					services[i].Meta.UID = existingUID
				} else {
					// New resource, generate UID
					services[i].Meta.TouchOnCreate()
				}
			}

			if err := w.upsertService(ctx, services[i]); err != nil {
				return errors.Wrapf(err, "failed to upsert service %s/%s", services[i].Namespace, services[i].Name)
			}
		}
	default:
		return errors.Errorf("unsupported sync operation: %v", syncOp)
	}

	return nil
}

// upsertService inserts or updates a service with full K8s metadata support
func (w *Writer) upsertService(ctx context.Context, service models.Service) error {
	// Marshal ingress ports to JSON
	ingressPortsJSON, err := w.marshalIngressPorts(service.IngressPorts)
	if err != nil {
		return errors.Wrap(err, "failed to marshal ingress ports")
	}

	// Marshal address_groups to JSON using intermediate structure
	var addressGroupsJSON []byte
	if len(service.AddressGroups) > 0 {
		// Convert domain AddressGroups to intermediate JSON structure for database
		agRefs := make([]addressGroupRefJSON, len(service.AddressGroups))
		for i, ag := range service.AddressGroups {
			agRefs[i] = addressGroupRefJSON{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       ag.Name,
				Namespace:  ag.Namespace,
			}
		}
		var err error
		addressGroupsJSON, err = json.Marshal(agRefs)
		if err != nil {
			return errors.Wrap(err, "failed to marshal address_groups")
		}
	} else {
		addressGroupsJSON = []byte("[]")
	}

	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(service.Meta.Labels, service.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(service.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, check if service exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM services WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, existingQuery, service.Namespace, service.Name).Scan(&existingResourceVersion)

	// Note: sql.ErrNoRows is expected for new services, not an actual error
	if err != nil {
		if err != sql.ErrNoRows && err.Error() != "no rows in result set" {
			return errors.Wrapf(err, "failed to check existing service %s/%s", service.Namespace, service.Name)
		}
		// Reset err to nil for sql.ErrNoRows or "no rows in result set"
		err = nil
	}

	var resourceVersion int64
	if existingResourceVersion.Valid {
		// UPDATE existing K8s metadata with UID and Generation
		metadataQuery := `
			UPDATE k8s_metadata
			SET labels = $1, annotations = $2, conditions = $3, uid = $4, generation = $5, updated_at = NOW()
			WHERE resource_version = $6
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, service.Meta.UID, service.Meta.Generation, existingResourceVersion.Int64).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to update K8s metadata for service %s/%s", service.Namespace, service.Name)
		}
	} else {
		// INSERT new K8s metadata with UID and Generation from TouchOnCreate()
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions, uid, generation)
			VALUES ($1, $2, '{}', $3, $4, $5)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, service.Meta.UID, service.Meta.Generation).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for service %s/%s", service.Namespace, service.Name)
		}
	}

	// Then, upsert the service using the resource version
	serviceQuery := `
		INSERT INTO services (namespace, name, description, ingress_ports, address_groups, resource_version)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (namespace, name) DO UPDATE SET
			description = $3,
			ingress_ports = $4,
			address_groups = $5,
			resource_version = $6`

	if err := w.exec(ctx, serviceQuery,
		service.Namespace,
		service.Name,
		service.Description,
		ingressPortsJSON,
		addressGroupsJSON,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert service %s/%s", service.Namespace, service.Name)
	}

	return nil
}

// getExistingServiceUID retrieves the UID of an existing service from the database
func (w *Writer) getExistingServiceUID(ctx context.Context, namespace, name string) (string, error) {
	var uid string
	query := `
		SELECT km.uid
		FROM services s
		JOIN k8s_metadata km ON s.resource_version = km.resource_version
		WHERE s.namespace = $1 AND s.name = $2`

	err := w.tx.QueryRow(ctx, query, namespace, name).Scan(&uid)
	if err != nil {
		return "", err // sql.ErrNoRows expected for new services
	}
	return uid, nil
}

// updateServiceConditionsOnly updates only the conditions in k8s_metadata for condition-only operations
// This avoids the UID conflict issues when ConditionManager runs after main transaction commit
func (w *Writer) updateServiceConditionsOnly(ctx context.Context, service models.Service) error {
	// Marshal only the conditions we need to update
	conditionsJSON, err := json.Marshal(service.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// Find the existing service's resource_version by namespace/name
	var resourceVersion int64
	findQuery := `SELECT resource_version FROM services WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, findQuery, service.Namespace, service.Name).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to find service %s/%s for condition update", service.Namespace, service.Name)
	}

	// Update only the conditions in k8s_metadata using the resource_version
	conditionUpdateQuery := `
		UPDATE k8s_metadata
		SET conditions = $1, updated_at = NOW()
		WHERE resource_version = $2`

	if err := w.exec(ctx, conditionUpdateQuery, conditionsJSON, resourceVersion); err != nil {
		return errors.Wrapf(err, "failed to update conditions for service %s/%s", service.Namespace, service.Name)
	}

	return nil
}

// deleteServicesInScope deletes services that match the provided scope
func (w *Writer) deleteServicesInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "s")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM services s WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete services in scope")
	}

	return nil
}

// deleteServicesByIdentifiers deletes specific services by their identifiers
func (w *Writer) deleteServicesByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
	if len(identifiers) == 0 {
		return nil
	}

	// Build parameter placeholders and collect args
	var conditions []string
	var args []interface{}
	argIndex := 1

	for _, id := range identifiers {
		conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", argIndex, argIndex+1))
		args = append(args, id.Namespace, id.Name)
		argIndex += 2
	}

	// First, mark objects as being deleted in k8s_metadata to prevent re-creation by ListWatch
	markDeleteQuery := `
		UPDATE k8s_metadata m
		SET deletion_timestamp = NOW()
		FROM services s
		WHERE s.resource_version = m.resource_version
		  AND (%s)
		  AND m.deletion_timestamp IS NULL`

	markQuery := fmt.Sprintf(markDeleteQuery, strings.Join(conditions, " OR "))
	_, err := w.tx.Exec(ctx, markQuery, args...)
	if err != nil {
		// Log but don't fail - deletion_timestamp is optional for now
		klog.V(4).InfoS("Failed to mark services as deleting in k8s_metadata", "error", err.Error())
	}

	// Then delete from services table
	query := fmt.Sprintf(`
		DELETE FROM services WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete services by identifiers")
	}

	return nil
}

// SyncServiceAliases implements hybrid sync strategy for service aliases
func (w *Writer) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope, opts ...ports.Option) error {
	// Extract sync operation from options (like services and address_group)
	// This was MISSING and caused DELETE operations to be treated as UPSERT!
	syncOp := models.SyncOpUpsert // Default operation
	isConditionOnly := false

	for _, opt := range opts {
		if syncOption, ok := opt.(ports.SyncOption); ok {
			syncOp = syncOption.Operation
		}
		if _, ok := opt.(ports.ConditionOnlyOperation); ok {
			isConditionOnly = true
		}
	}

	// For condition-only operations, only update k8s_metadata conditions
	if isConditionOnly {
		for _, alias := range aliases {
			if err := w.updateServiceAliasConditionsOnly(ctx, alias); err != nil {
				return errors.Wrapf(err, "failed to update conditions for service alias %s/%s", alias.Namespace, alias.Name)
			}
		}
		return nil
	}

	// Handle scoped sync - delete existing resources in scope first (for non-DELETE operations)
	// This matches the logic from services.go and address_group.go that work correctly
	if !scope.IsEmpty() && syncOp != models.SyncOpDelete {
		if err := w.deleteServiceAliasesInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete service aliases in scope")
		}
	}

	// Handle operations based on sync operation type
	// This was COMPLETELY MISSING and is why DELETE operations were treated as UPSERT!
	switch syncOp {
	case models.SyncOpDelete:
		// For DELETE operations, delete the specific service aliases
		var identifiers []models.ResourceIdentifier
		for _, alias := range aliases {
			identifiers = append(identifiers, alias.SelfRef.ResourceIdentifier)
		}
		if err := w.deleteServiceAliasesByIdentifiers(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to delete service aliases")
		}
	case models.SyncOpUpsert, models.SyncOpFullSync:
		// For UPSERT/FULLSYNC operations, upsert all provided service aliases
		for i := range aliases {
			// Initialize metadata fields if not set
			if aliases[i].Meta.UID == "" {
				aliases[i].Meta.TouchOnCreate()
			}

			if err := w.upsertServiceAlias(ctx, aliases[i]); err != nil {
				return errors.Wrapf(err, "failed to upsert service alias %s/%s", aliases[i].Namespace, aliases[i].Name)
			}
		}
	}

	return nil
}

// upsertServiceAlias inserts or updates a service alias
func (w *Writer) upsertServiceAlias(ctx context.Context, alias models.ServiceAlias) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(alias.Meta.Labels, alias.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(alias.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, upsert K8s metadata and get resource version with UID and Generation
	metadataQuery := `
		INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions, uid, generation)
		VALUES ($1, $2, '{}', $3, $4, $5)
		RETURNING resource_version`

	var resourceVersion int64
	err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, alias.Meta.UID, alias.Meta.Generation).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to create K8s metadata for service alias %s/%s", alias.Namespace, alias.Name)
	}

	// Then, upsert the service alias using the resource version
	serviceAliasQuery := `
		INSERT INTO service_aliases (namespace, name, service_namespace, service_name, resource_version)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (namespace, name) DO UPDATE SET
			service_namespace = $3,
			service_name = $4,
			resource_version = $5`

	if err := w.exec(ctx, serviceAliasQuery,
		alias.Namespace,
		alias.Name,
		alias.ServiceRef.Namespace,
		alias.ServiceRef.Name,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert service alias %s/%s", alias.Namespace, alias.Name)
	}

	return nil
}

// deleteServiceAliasesInScope deletes service aliases that match the provided scope
func (w *Writer) deleteServiceAliasesInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "sa")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM service_aliases sa WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete service aliases in scope")
	}

	return nil
}

// DeleteServicesByIDs deletes services by their resource identifiers
func (w *Writer) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.deleteServicesByIdentifiers(ctx, ids)
}

// deleteServiceAliasesByIdentifiers deletes specific service aliases by their identifiers (internal helper for SyncServiceAliases)
func (w *Writer) deleteServiceAliasesByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
	if len(identifiers) == 0 {
		return nil
	}

	// Build parameter placeholders and collect args
	var conditions []string
	var args []interface{}
	argIndex := 1

	for _, id := range identifiers {
		conditions = append(conditions, fmt.Sprintf("(namespace = $%d AND name = $%d)", argIndex, argIndex+1))
		args = append(args, id.Namespace, id.Name)
		argIndex += 2
	}

	// First, mark objects as being deleted in k8s_metadata to prevent re-creation by ListWatch
	markDeleteQuery := `
		UPDATE k8s_metadata m
		SET deletion_timestamp = NOW()
		FROM service_aliases sa
		WHERE sa.resource_version = m.resource_version
		  AND (%s)
		  AND m.deletion_timestamp IS NULL`

	markQuery := fmt.Sprintf(markDeleteQuery, strings.Join(conditions, " OR "))
	_, err := w.tx.Exec(ctx, markQuery, args...)
	if err != nil {
		// Log but don't fail - deletion_timestamp is optional for now
		klog.V(4).InfoS("Failed to mark service aliases as deleting in k8s_metadata", "error", err.Error())
	}

	// Then delete from service_aliases table
	query := fmt.Sprintf(`
		DELETE FROM service_aliases WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete service aliases by identifiers")
	}

	return nil
}

// DeleteServiceAliasesByIDs deletes service aliases by their resource identifiers
func (w *Writer) DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
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

	// First, mark objects as being deleted in k8s_metadata to prevent re-creation by ListWatch
	markDeleteQuery := `
		UPDATE k8s_metadata m
		SET deletion_timestamp = NOW()
		FROM service_aliases sa
		WHERE sa.resource_version = m.resource_version
		  AND (%s)
		  AND m.deletion_timestamp IS NULL`

	markQuery := fmt.Sprintf(markDeleteQuery, strings.Join(conditions, " OR "))
	_, err := w.tx.Exec(ctx, markQuery, args...)
	if err != nil {
		// Log but don't fail - deletion_timestamp is optional for now
		klog.V(4).InfoS("Failed to mark service aliases as deleting in k8s_metadata", "error", err.Error())
	}

	// Then delete from service_aliases table
	query := fmt.Sprintf(`
		DELETE FROM service_aliases WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete service aliases by identifiers")
	}

	return nil
}

// updateServiceAliasConditionsOnly updates only the conditions in k8s_metadata for condition-only operations
// This avoids the UID conflict issues when ConditionManager runs after main transaction commit
func (w *Writer) updateServiceAliasConditionsOnly(ctx context.Context, alias models.ServiceAlias) error {
	// Marshal only the conditions we need to update
	conditionsJSON, err := json.Marshal(alias.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// Find the existing service alias's resource_version by namespace/name
	var resourceVersion int64
	findQuery := `SELECT resource_version FROM service_aliases WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, findQuery, alias.Namespace, alias.Name).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to find service alias %s/%s for condition update", alias.Namespace, alias.Name)
	}

	// Update only the conditions in k8s_metadata using the resource_version
	conditionUpdateQuery := `
		UPDATE k8s_metadata
		SET conditions = $1, updated_at = NOW()
		WHERE resource_version = $2`

	if err := w.exec(ctx, conditionUpdateQuery, conditionsJSON, resourceVersion); err != nil {
		return errors.Wrapf(err, "failed to update conditions for service alias %s/%s", alias.Namespace, alias.Name)
	}

	return nil
}
