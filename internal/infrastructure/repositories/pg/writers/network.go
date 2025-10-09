package writers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// CIDRAlreadyExistsError represents a CIDR uniqueness violation error
type CIDRAlreadyExistsError struct {
	CIDR        string
	NetworkName string
	Err         error
}

func (e *CIDRAlreadyExistsError) Error() string {
	return fmt.Sprintf("CIDR '%s' already exists (attempted to create/update network %s)", e.CIDR, e.NetworkName)
}

func (e *CIDRAlreadyExistsError) Unwrap() error {
	return e.Err
}

// isUniqueViolation checks if the error is a PostgreSQL unique constraint violation
// for the specified constraint name
func isUniqueViolation(err error, constraintName string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// PostgreSQL unique_violation error code is "23505"
		if pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, constraintName) {
			return true
		}
	}
	return false
}

// SyncNetworks syncs networks to PostgreSQL with K8s metadata support
func (w *Writer) SyncNetworks(ctx context.Context, networks []models.Network, scope ports.Scope, options ...ports.Option) error {
	// Extract sync operation from options
	syncOp := models.SyncOpUpsert // Default operation
	for _, opt := range options {
		if syncOption, ok := opt.(ports.SyncOption); ok {
			syncOp = syncOption.Operation
			break
		}
	}

	// Handle scoped sync - delete existing resources in scope first (for non-DELETE operations)
	if !scope.IsEmpty() && syncOp != models.SyncOpDelete {
		if err := w.deleteNetworksInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete networks in scope")
		}
	}

	// Handle operations based on sync operation
	switch syncOp {
	case models.SyncOpDelete:
		// For DELETE operations, delete the specific networks
		var identifiers []models.ResourceIdentifier
		for _, network := range networks {
			identifiers = append(identifiers, models.ResourceIdentifier{
				Namespace: network.Namespace,
				Name:      network.Name,
			})
		}
		if err := w.DeleteNetworksByIDs(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to delete networks")
		}
	case models.SyncOpUpsert, models.SyncOpFullSync:
		// For UPSERT/FULLSYNC operations, upsert all provided networks
		for i := range networks {
			// Initialize metadata fields if not set
			if networks[i].Meta.UID == "" {
				networks[i].Meta.TouchOnCreate()
			}

			if err := w.upsertNetwork(ctx, &networks[i]); err != nil {
				return errors.Wrapf(err, "failed to upsert network %s/%s", networks[i].Namespace, networks[i].Name)
			}
		}
	}

	return nil
}

// upsertNetwork inserts or updates a network with full K8s metadata support
func (w *Writer) upsertNetwork(ctx context.Context, network *models.Network) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(network.Meta.Labels, network.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(network.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// ALWAYS INSERT new K8s metadata (to get new ResourceVersion)
	// PostgreSQL BIGSERIAL primary key only increments on INSERT, not UPDATE
	var resourceVersion int64
	metadataQuery := `
		INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
		VALUES ($1, $2, '{}', $3)
		RETURNING resource_version`
	err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to insert K8s metadata for network %s/%s", network.Namespace, network.Name)
	}

	// Update domain model with new ResourceVersion from DB
	network.Meta.TouchOnWrite(strconv.FormatInt(resourceVersion, 10))

	// Create network items with single CIDR entry
	networkItems := []map[string]interface{}{
		{"cidr": network.CIDR, "name": network.NetworkName},
	}
	networkItemsJSON, err := json.Marshal(networkItems)
	if err != nil {
		return errors.Wrap(err, "failed to marshal network_items")
	}

	// Extract reference fields from ObjectReference (references are in same namespace)
	var bindingRefNamespace, bindingRefName, agRefNamespace, agRefName interface{}
	if network.BindingRef != nil {
		bindingRefNamespace = network.Namespace // References are in same namespace
		bindingRefName = network.BindingRef.Name
	}
	if network.AddressGroupRef != nil {
		agRefNamespace = network.Namespace // References are in same namespace
		agRefName = network.AddressGroupRef.Name
	}

	// Then, upsert the network using the NEW resource version
	networkQuery := `
		INSERT INTO networks (namespace, name, cidr, network_items, is_bound,
			binding_ref_namespace, binding_ref_name,
			address_group_ref_namespace, address_group_ref_name,
			resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (namespace, name) DO UPDATE SET
			cidr = $3,
			network_items = $4,
			is_bound = $5,
			binding_ref_namespace = $6,
			binding_ref_name = $7,
			address_group_ref_namespace = $8,
			address_group_ref_name = $9,
			resource_version = $10`

	if err := w.exec(ctx, networkQuery,
		network.Namespace,
		network.Name,
		network.CIDR, // Add CIDR as separate column
		networkItemsJSON,
		network.IsBound,
		bindingRefNamespace,
		bindingRefName,
		agRefNamespace,
		agRefName,
		resourceVersion,
	); err != nil {
		// Check if this is a UNIQUE constraint violation on CIDR
		if isUniqueViolation(err, "idx_networks_cidr_unique") {
			return &CIDRAlreadyExistsError{
				CIDR:        network.CIDR,
				NetworkName: network.Key(),
				Err:         err,
			}
		}
		return errors.Wrapf(err, "failed to upsert network %s/%s", network.Namespace, network.Name)
	}

	return nil
}

// deleteNetworksInScope deletes networks that match the provided scope
func (w *Writer) deleteNetworksInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "n")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM networks n WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete networks in scope")
	}

	return nil
}

// DeleteNetworksByIDs deletes networks by their identifiers
func (w *Writer) DeleteNetworksByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
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
		FROM networks n
		WHERE n.resource_version = m.resource_version
		  AND (%s)
		  AND m.deletion_timestamp IS NULL`

	markQuery := fmt.Sprintf(markDeleteQuery, strings.Join(conditions, " OR "))
	_, err := w.tx.Exec(ctx, markQuery, args...)
	if err != nil {
		// Log but don't fail - deletion_timestamp is optional for now
		klog.V(4).InfoS("Failed to mark networks as deleting in k8s_metadata", "error", err.Error())
	}

	// Then delete from networks table
	query := fmt.Sprintf(`
		DELETE FROM networks WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete networks by identifiers")
	}

	return nil
}
