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

// SyncAddressGroups implements hybrid sync strategy for address groups
func (w *Writer) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope, opts ...ports.Option) error {
	// 🔧 PRODUCTION FIX: Check if this is a condition-only operation
	// If so, we need to handle transaction context properly to avoid UID conflicts
	isConditionOnly := false
	for _, opt := range opts {
		if _, ok := opt.(ports.ConditionOnlyOperation); ok {
			isConditionOnly = true
			fmt.Printf("🔧 DEBUG: Detected ConditionOnlyOperation for AddressGroup sync\n")
			break
		}
	}

	// 🚨 CRITICAL: For condition-only operations, we need fresh transaction context
	// because ConditionManager runs after main transaction commit and can't see the addressgroup
	// that was just committed due to transaction isolation
	if isConditionOnly {
		fmt.Printf("🚧 DEBUG: ConditionOnly operation detected for AddressGroup - using fresh ReadCommitted transaction...\n")

		// Use type assertion to check if registry supports condition operations
		if conditionRegistry, ok := w.registry.(interface {
			WriterForConditions(ctx context.Context) (ports.Writer, error)
		}); ok {
			// Create a fresh writer with ReadCommitted isolation that can see committed data
			freshWriter, err := conditionRegistry.WriterForConditions(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to create fresh writer for condition operations")
			}
			defer freshWriter.Abort() // Ensure cleanup

			// Use the fresh writer for the sync operation
			fmt.Printf("✅ DEBUG: Using fresh ReadCommitted writer for AddressGroup condition sync\n")

			// Use the fresh writer's SyncAddressGroups method directly
			// 🚨 CRITICAL: Don't pass ConditionOnlyOperation to prevent infinite recursion!
			var filteredOpts []ports.Option
			for _, opt := range opts {
				if _, ok := opt.(ports.ConditionOnlyOperation); !ok {
					filteredOpts = append(filteredOpts, opt)
				}
			}
			if err := freshWriter.SyncAddressGroups(ctx, addressGroups, scope, filteredOpts...); err != nil {
				return errors.Wrap(err, "failed to sync address groups with fresh writer")
			}
			// Commit the fresh transaction
			if err := freshWriter.Commit(); err != nil {
				return errors.Wrap(err, "failed to commit fresh writer transaction")
			}
			fmt.Printf("✅ DEBUG: AddressGroup condition sync completed successfully with fresh transaction\n")
			return nil
		}
		return errors.New("registry does not support condition operations")
	}
	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteAddressGroupsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete address groups in scope")
		}
	}

	// Upsert all provided address groups
	for i := range addressGroups {
		// 🔧 CRITICAL FIX: Initialize metadata fields (UID, Generation, ObservedGeneration)
		// This is what Memory backend does via ensureMetaFill() -> TouchOnCreate()
		// Without this, PATCH operations fail because objInfo.UpdatedObject() needs UID
		// IMPORTANT: Use index-based loop to modify original, not copy!
		if addressGroups[i].Meta.UID == "" {
			addressGroups[i].Meta.TouchOnCreate()
		}

		if err := w.upsertAddressGroup(ctx, addressGroups[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert address group %s/%s", addressGroups[i].Namespace, addressGroups[i].Name)
		}
	}

	return nil
}

// upsertAddressGroup inserts or updates an address group with full K8s metadata support
func (w *Writer) upsertAddressGroup(ctx context.Context, ag models.AddressGroup) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(ag.Meta.Labels, ag.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(ag.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// Marshal Networks field (critical fix for Networks field persistence)
	networksJSON, err := json.Marshal(ag.Networks)
	if err != nil {
		return errors.Wrap(err, "failed to marshal networks")
	}

	// First, check if address group exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM address_groups WHERE namespace = $1 AND name = $2`
	_ = w.tx.QueryRow(ctx, existingQuery, ag.Namespace, ag.Name).Scan(&existingResourceVersion)

	var resourceVersion int64
	if existingResourceVersion.Valid {
		// UPDATE existing K8s metadata
		metadataQuery := `
			UPDATE k8s_metadata 
			SET labels = $1, annotations = $2, conditions = $3, updated_at = NOW()
			WHERE resource_version = $4
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, existingResourceVersion.Int64).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to update K8s metadata for address group %s/%s", ag.Namespace, ag.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
			VALUES ($1, $2, '{}', $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for address group %s/%s", ag.Namespace, ag.Name)
		}
	}

	// Then, upsert the address group using the resource version (including Networks field)
	addressGroupQuery := `
		INSERT INTO address_groups (namespace, name, default_action, logs, trace, description, networks, resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (namespace, name) DO UPDATE SET
			default_action = $3,
			logs = $4,
			trace = $5,
			description = $6,
			networks = $7,
			resource_version = $8`

	if err := w.exec(ctx, addressGroupQuery,
		ag.Namespace,
		ag.Name,
		string(ag.DefaultAction),
		ag.Logs,
		ag.Trace,
		"",           // description field
		networksJSON, // Networks field - CRITICAL FIX
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert address group %s/%s", ag.Namespace, ag.Name)
	}

	return nil
}

// deleteAddressGroupsInScope deletes address groups that match the provided scope
func (w *Writer) deleteAddressGroupsInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "ag")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM address_groups ag WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address groups in scope")
	}

	return nil
}

// deleteAddressGroupsByIdentifiers deletes specific address groups by their identifiers
func (w *Writer) deleteAddressGroupsByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
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
		DELETE FROM address_groups WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address groups by identifiers")
	}

	return nil
}

// SyncAddressGroupBindings implements hybrid sync strategy for address group bindings
func (w *Writer) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope, opts ...ports.Option) error {
	// Extract sync operation from options
	syncOp := models.SyncOpUpsert // Default operation
	for _, opt := range opts {
		if syncOption, ok := opt.(ports.SyncOption); ok {
			syncOp = syncOption.Operation
			break
		}
	}

	// Handle scoped sync - delete existing resources in scope first (for non-DELETE operations)
	if !scope.IsEmpty() && syncOp != models.SyncOpDelete {
		if err := w.deleteAddressGroupBindingsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete address group bindings in scope")
		}
	}

	// Handle operations based on sync operation
	switch syncOp {
	case models.SyncOpDelete:
		// For DELETE operations, delete the specific bindings
		var identifiers []models.ResourceIdentifier
		for _, binding := range bindings {
			identifiers = append(identifiers, binding.SelfRef.ResourceIdentifier)
		}
		if err := w.deleteAddressGroupBindingsByIdentifiers(ctx, identifiers); err != nil {
			return errors.Wrap(err, "failed to delete address group bindings")
		}
	case models.SyncOpUpsert, models.SyncOpFullSync:
		// For UPSERT/FULLSYNC operations, upsert all provided bindings
		for i := range bindings {
			// 🔧 CRITICAL FIX: Initialize metadata fields (UID, Generation, ObservedGeneration)
			// This is what Memory backend does via ensureMetaFill() -> TouchOnCreate()
			// Without this, PATCH operations fail because objInfo.UpdatedObject() needs UID
			// IMPORTANT: Use index-based loop to modify original, not copy!
			if bindings[i].Meta.UID == "" {
				bindings[i].Meta.TouchOnCreate()
			}

			if err := w.upsertAddressGroupBinding(ctx, bindings[i]); err != nil {
				return errors.Wrapf(err, "failed to upsert address group binding %s/%s", bindings[i].Namespace, bindings[i].Name)
			}
		}
	default:
		return errors.New(fmt.Sprintf("unsupported sync operation: %v", syncOp))
	}

	return nil
}

// upsertAddressGroupBinding inserts or updates an address group binding
func (w *Writer) upsertAddressGroupBinding(ctx context.Context, binding models.AddressGroupBinding) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(binding.Meta.Labels, binding.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(binding.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, check if address group binding exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM address_group_bindings WHERE namespace = $1 AND name = $2`
	_ = w.tx.QueryRow(ctx, existingQuery, binding.Namespace, binding.Name).Scan(&existingResourceVersion)

	var resourceVersion int64
	if existingResourceVersion.Valid {
		// UPDATE existing K8s metadata
		metadataQuery := `
			UPDATE k8s_metadata 
			SET labels = $1, annotations = $2, conditions = $3, updated_at = NOW()
			WHERE resource_version = $4
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, existingResourceVersion.Int64).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to update K8s metadata for address group binding %s/%s", binding.Namespace, binding.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
			VALUES ($1, $2, '{}', $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for address group binding %s/%s", binding.Namespace, binding.Name)
		}
	}

	// Then, upsert the address group binding using the resource version
	bindingQuery := `
		INSERT INTO address_group_bindings (namespace, name, service_namespace, service_name, address_group_namespace, address_group_name, resource_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (namespace, name) DO UPDATE SET
			service_namespace = $3,
			service_name = $4,
			address_group_namespace = $5,
			address_group_name = $6,
			resource_version = $7`

	if err := w.exec(ctx, bindingQuery,
		binding.Namespace,
		binding.Name,
		binding.ServiceRef.Namespace,
		binding.ServiceRef.Name,
		binding.AddressGroupRef.Namespace,
		binding.AddressGroupRef.Name,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert address group binding %s/%s", binding.Namespace, binding.Name)
	}

	return nil
}

// deleteAddressGroupBindingsInScope deletes address group bindings that match the provided scope
func (w *Writer) deleteAddressGroupBindingsInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "agb")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM address_group_bindings agb WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address group bindings in scope")
	}

	return nil
}

// deleteAddressGroupBindingsByIdentifiers deletes specific address group bindings by their identifiers
func (w *Writer) deleteAddressGroupBindingsByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
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
		DELETE FROM address_group_bindings WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address group bindings by identifiers")
	}

	return nil
}

// SyncAddressGroupPortMappings implements hybrid sync strategy for address group port mappings
func (w *Writer) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope, opts ...ports.Option) error {
	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteAddressGroupPortMappingsInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete address group port mappings in scope")
		}
	}

	// Upsert all provided mappings
	for i := range mappings {
		// 🔧 CRITICAL FIX: Initialize metadata fields (UID, Generation, ObservedGeneration)
		// This is what Memory backend does via ensureMetaFill() -> TouchOnCreate()
		// Without this, PATCH operations fail because objInfo.UpdatedObject() needs UID
		// IMPORTANT: Use index-based loop to modify original, not copy!
		if mappings[i].Meta.UID == "" {
			mappings[i].Meta.TouchOnCreate()
		}

		if err := w.upsertAddressGroupPortMapping(ctx, mappings[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert address group port mapping %s/%s", mappings[i].Namespace, mappings[i].Name)
		}
	}

	return nil
}

// upsertAddressGroupPortMapping inserts or updates an address group port mapping with complex map handling
func (w *Writer) upsertAddressGroupPortMapping(ctx context.Context, mapping models.AddressGroupPortMapping) error {
	// Marshal the complex AccessPorts map
	accessPortsJSON, err := w.marshalAccessPorts(mapping.AccessPorts)
	if err != nil {
		return errors.Wrap(err, "failed to marshal access ports")
	}

	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(mapping.Meta.Labels, mapping.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(mapping.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, check if address group port mapping exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM address_group_port_mappings WHERE namespace = $1 AND name = $2`
	_ = w.tx.QueryRow(ctx, existingQuery, mapping.Namespace, mapping.Name).Scan(&existingResourceVersion)

	var resourceVersion int64
	if existingResourceVersion.Valid {
		// UPDATE existing K8s metadata
		metadataQuery := `
			UPDATE k8s_metadata 
			SET labels = $1, annotations = $2, conditions = $3, updated_at = NOW()
			WHERE resource_version = $4
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, existingResourceVersion.Int64).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to update K8s metadata for address group port mapping %s/%s", mapping.Namespace, mapping.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
			VALUES ($1, $2, '{}', $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for address group port mapping %s/%s", mapping.Namespace, mapping.Name)
		}
	}

	// Then, upsert the address group port mapping using the resource version
	portMappingQuery := `
		INSERT INTO address_group_port_mappings (namespace, name, access_ports, resource_version)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (namespace, name) DO UPDATE SET
			access_ports = $3,
			resource_version = $4`

	if err := w.exec(ctx, portMappingQuery,
		mapping.Namespace,
		mapping.Name,
		accessPortsJSON,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert address group port mapping %s/%s", mapping.Namespace, mapping.Name)
	}

	return nil
}

// deleteAddressGroupPortMappingsInScope deletes address group port mappings that match the provided scope
func (w *Writer) deleteAddressGroupPortMappingsInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "agpm")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM address_group_port_mappings agpm WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address group port mappings in scope")
	}

	return nil
}

// deleteAddressGroupPortMappingsByIdentifiers deletes specific address group port mappings by their identifiers
func (w *Writer) deleteAddressGroupPortMappingsByIdentifiers(ctx context.Context, identifiers []models.ResourceIdentifier) error {
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
		DELETE FROM address_group_port_mappings WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address group port mappings by identifiers")
	}

	return nil
}

// SyncAddressGroupBindingPolicies implements hybrid sync strategy for address group binding policies
func (w *Writer) SyncAddressGroupBindingPolicies(ctx context.Context, policies []models.AddressGroupBindingPolicy, scope ports.Scope, opts ...ports.Option) error {
	// Handle scoped sync - delete existing resources in scope first
	if !scope.IsEmpty() {
		if err := w.deleteAddressGroupBindingPoliciesInScope(ctx, scope); err != nil {
			return errors.Wrap(err, "failed to delete address group binding policies in scope")
		}
	}

	// Upsert all provided policies
	for i := range policies {
		// 🔧 CRITICAL FIX: Initialize metadata fields (UID, Generation, ObservedGeneration)
		// This is what Memory backend does via ensureMetaFill() -> TouchOnCreate()
		// Without this, PATCH operations fail because objInfo.UpdatedObject() needs UID
		// IMPORTANT: Use index-based loop to modify original, not copy!
		if policies[i].Meta.UID == "" {
			policies[i].Meta.TouchOnCreate()
		}

		if err := w.upsertAddressGroupBindingPolicy(ctx, policies[i]); err != nil {
			return errors.Wrapf(err, "failed to upsert address group binding policy %s/%s", policies[i].Namespace, policies[i].Name)
		}
	}

	return nil
}

// upsertAddressGroupBindingPolicy inserts or updates an address group binding policy
func (w *Writer) upsertAddressGroupBindingPolicy(ctx context.Context, policy models.AddressGroupBindingPolicy) error {
	// Marshal K8s metadata
	labelsJSON, annotationsJSON, err := w.marshalLabelsAnnotations(policy.Meta.Labels, policy.Meta.Annotations)
	if err != nil {
		return errors.Wrap(err, "failed to marshal K8s metadata")
	}

	conditionsJSON, err := json.Marshal(policy.Meta.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	// First, check if address group binding policy exists and get existing resource version
	var existingResourceVersion sql.NullInt64
	existingQuery := `SELECT resource_version FROM address_group_binding_policies WHERE namespace = $1 AND name = $2`
	_ = w.tx.QueryRow(ctx, existingQuery, policy.Namespace, policy.Name).Scan(&existingResourceVersion)

	var resourceVersion int64
	if existingResourceVersion.Valid {
		// UPDATE existing K8s metadata
		metadataQuery := `
			UPDATE k8s_metadata 
			SET labels = $1, annotations = $2, conditions = $3, updated_at = NOW()
			WHERE resource_version = $4
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON, existingResourceVersion.Int64).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to update K8s metadata for address group binding policy %s/%s", policy.Namespace, policy.Name)
		}
	} else {
		// INSERT new K8s metadata
		metadataQuery := `
			INSERT INTO k8s_metadata (labels, annotations, finalizers, conditions)
			VALUES ($1, $2, '{}', $3)
			RETURNING resource_version`
		err = w.tx.QueryRow(ctx, metadataQuery, labelsJSON, annotationsJSON, conditionsJSON).Scan(&resourceVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to create K8s metadata for address group binding policy %s/%s", policy.Namespace, policy.Name)
		}
	}

	// Marshal reference fields to JSONB
	addressGroupRefJSON, err := json.Marshal(policy.AddressGroupRef)
	if err != nil {
		return errors.Wrap(err, "failed to marshal address group reference")
	}

	serviceRefJSON, err := json.Marshal(policy.ServiceRef)
	if err != nil {
		return errors.Wrap(err, "failed to marshal service reference")
	}

	// Then, upsert the address group binding policy using the resource version
	policyQuery := `
		INSERT INTO address_group_binding_policies (namespace, name, address_group_ref, service_ref, resource_version)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (namespace, name) DO UPDATE SET
			address_group_ref = $3,
			service_ref = $4,
			resource_version = $5`

	if err := w.exec(ctx, policyQuery,
		policy.Namespace,
		policy.Name,
		addressGroupRefJSON,
		serviceRefJSON,
		resourceVersion,
	); err != nil {
		return errors.Wrapf(err, "failed to upsert address group binding policy %s/%s", policy.Namespace, policy.Name)
	}

	return nil
}

// deleteAddressGroupBindingPoliciesInScope deletes address group binding policies that match the provided scope
func (w *Writer) deleteAddressGroupBindingPoliciesInScope(ctx context.Context, scope ports.Scope) error {
	if scope.IsEmpty() {
		return nil
	}

	whereClause, args := w.buildScopeFilter(scope, "agbp")
	if whereClause == "" {
		return nil
	}

	query := fmt.Sprintf(`
		DELETE FROM address_group_binding_policies agbp WHERE %s`, whereClause)

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address group binding policies in scope")
	}

	return nil
}

// Delete methods by IDs
func (w *Writer) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.deleteAddressGroupsByIdentifiers(ctx, ids)
}

func (w *Writer) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.deleteAddressGroupBindingsByIdentifiers(ctx, ids)
}

func (w *Writer) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	return w.deleteAddressGroupPortMappingsByIdentifiers(ctx, ids)
}

func (w *Writer) DeleteAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
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
		DELETE FROM address_group_binding_policies WHERE %s`,
		strings.Join(conditions, " OR "))

	if err := w.exec(ctx, query, args...); err != nil {
		return errors.Wrap(err, "failed to delete address group binding policies by identifiers")
	}

	return nil
}
