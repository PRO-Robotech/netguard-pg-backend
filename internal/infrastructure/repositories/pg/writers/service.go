package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// SyncServices implements hybrid sync strategy for services
func (w *Writer) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, opts ...ports.Option) error {
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

	if isConditionOnly {
		for _, service := range services {
			if err := w.updateServiceConditionsOnly(ctx, service); err != nil {
				return errors.Wrapf(err, "failed to update conditions for service %s/%s", service.Namespace, service.Name)
			}
		}
		return nil
	}

	if !scope.IsEmpty() && syncOp != models.SyncOpDelete {
		if err := w.deleteServicesInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete services in scope")
		}
	}

	switch syncOp {
	case models.SyncOpDelete:
		var identifiers []models.ResourceIdentifier
		for _, service := range services {
			identifiers = append(identifiers, service.SelfRef.ResourceIdentifier)
		}
		if err := w.deleteServicesByIdentifiers(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to delete services")
		}
	case models.SyncOpUpsert, models.SyncOpFullSync:
		for i := range services {
			if services[i].Meta.UID == "" {
				existingUID, err := w.getExistingServiceUID(ctx, services[i].Namespace, services[i].Name)
				if err == nil && existingUID != "" {
					services[i].Meta.UID = existingUID
				} else {
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

func (w *Writer) upsertService(ctx context.Context, service models.Service) error {
	ingressPortsJSON, err := w.marshalIngressPorts(service.IngressPorts)
	if err != nil {
		return errors.Wrap(err, "failed to marshal ingress ports")
	}

	addressGroupsJSON, err := w.marshalSpecAddressGroups(service)
	if err != nil {
		return errors.Wrap(err, "failed to marshal spec address groups")
	}

	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(service.Meta.Labels, service.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(service.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM services WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, existingQuery, service.Namespace, service.Name).Scan(&existingResourceVersion)

	if err != nil {
		if err != sql.ErrNoRows && err.Error() != "no rows in result set" {
			return errors.Wrapf(err, "failed to check existing service %s/%s", service.Namespace, service.Name)
		}
		err = nil
	}

	var resourceVersion int64
	if existingResourceVersion.Valid {
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
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions, uid, generation)
			VALUES ($1, $2, '{}', $3, $4, $5)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, service.Meta.UID, service.Meta.Generation).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for service %s/%s", service.Namespace, service.Name)
		}
	}

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

func (w *Writer) marshalSpecAddressGroups(service models.Service) ([]byte, error) {
	specAddressGroups := service.GetSpecAddressGroups()

	var dbAddressGroups []models.AddressGroupRef
	for _, agRef := range specAddressGroups {
		dbAddressGroups = append(dbAddressGroups, agRef.Ref)
	}

	jsonData, err := json.Marshal(dbAddressGroups)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal spec address groups")
	}

	return jsonData, nil
}

func (w *Writer) getExistingServiceUID(ctx context.Context, namespace, name string) (string, error) {
	var uid string
	query := `
		SELECT km.uid
		FROM services s
		JOIN k8s_metadata km ON s.resource_version = km.resource_version
		WHERE s.namespace = $1 AND s.name = $2`

	err := w.tx.QueryRow(ctx, query, namespace, name).Scan(&uid)
	if err != nil {
		return "", err
	}
	return uid, nil
}

func (w *Writer) updateServiceConditionsOnly(ctx context.Context, service models.Service) error {
	conditionsJSON, err := json.Marshal(service.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	var resourceVersion int64
	findQuery := `SELECT resource_version FROM services WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, findQuery, service.Namespace, service.Name).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to find service %s/%s for condition update", service.Namespace, service.Name)
	}

	conditionUpdateQuery := `
		UPDATE k8s_metadata
		SET conditions = $1, updated_at = NOW()
		WHERE resource_version = $2`

	if err := w.exec(ctx, conditionUpdateQuery, conditionsJSON, resourceVersion); err != nil {
		return errors.Wrapf(err, "failed to update conditions for service %s/%s", service.Namespace, service.Name)
	}

	return nil
}

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

	query := fmt.Sprintf(`
		DELETE FROM services WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete services by identifiers")
	}

	return nil
}

func (w *Writer) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope, opts ...ports.Option) error {
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

	if isConditionOnly {
		for _, alias := range aliases {
			if err := w.updateServiceAliasConditionsOnly(ctx, alias); err != nil {
				return errors.Wrapf(err, "failed to update conditions for service alias %s/%s", alias.Namespace, alias.Name)
			}
		}
		return nil
	}

	if !scope.IsEmpty() && syncOp != models.SyncOpDelete {
		if err := w.deleteServiceAliasesInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete service aliases in scope")
		}
	}

	switch syncOp {
	case models.SyncOpDelete:
		var identifiers []models.ResourceIdentifier
		for _, alias := range aliases {
			identifiers = append(identifiers, alias.SelfRef.ResourceIdentifier)
		}
		if err := w.deleteServiceAliasesByIdentifiers(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to delete service aliases")
		}
	case models.SyncOpUpsert, models.SyncOpFullSync:
		for i := range aliases {
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

	query := fmt.Sprintf(`
		DELETE FROM service_aliases WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete service aliases by identifiers")
	}

	return nil
}

func (w *Writer) updateServiceAliasConditionsOnly(ctx context.Context, alias models.ServiceAlias) error {
	conditionsJSON, err := json.Marshal(alias.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	var resourceVersion int64
	findQuery := `SELECT resource_version FROM service_aliases WHERE namespace = $1 AND name = $2`
	err = w.tx.QueryRow(ctx, findQuery, alias.Namespace, alias.Name).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to find service alias %s/%s for condition update", alias.Namespace, alias.Name)
	}

	conditionUpdateQuery := `
		UPDATE k8s_metadata
		SET conditions = $1, updated_at = NOW()
		WHERE resource_version = $2`

	if err := w.exec(ctx, conditionUpdateQuery, conditionsJSON, resourceVersion); err != nil {
		return errors.Wrapf(err, "failed to update conditions for service alias %s/%s", alias.Namespace, alias.Name)
	}

	return nil
}
