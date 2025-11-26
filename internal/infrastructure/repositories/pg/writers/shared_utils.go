package writers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// buildScopeFilter builds WHERE clause and arguments for scope filtering
func (w *Writer) buildScopeFilter(scope ports.Scope, tableAlias string) (string, []interface{}) {
	if scope.IsEmpty() {
		return "", nil
	}

	switch s := scope.(type) {
	case ports.ResourceIdentifierScope:
		if len(s.Identifiers) == 0 {
			return "", nil
		}

		// Build IN clauses for namespace and name pairs
		var conditions []string
		var args []interface{}
		argIndex := 1

		for _, id := range s.Identifiers {
			// Handle namespace-only filtering (when name is empty, filter only by namespace)
			if id.Name == "" {
				condition := fmt.Sprintf("(%s.namespace = $%d)",
					tableAlias, argIndex)
				conditions = append(conditions, condition)
				args = append(args, id.Namespace)
				argIndex += 1
			} else {
				// Handle specific resource filtering (namespace + name)
				condition := fmt.Sprintf("(%s.namespace = $%d AND %s.name = $%d)",
					tableAlias, argIndex, tableAlias, argIndex+1)
				conditions = append(conditions, condition)
				args = append(args, id.Namespace, id.Name)
				argIndex += 2
			}
		}

		return "(" + strings.Join(conditions, " OR ") + ")", args

	default:
		// For other scope types, return empty filter
		return "", nil
	}
}

// marshalIngressPorts converts domain IngressPort slice to JSONB
func (w *Writer) marshalIngressPorts(ports []models.IngressPort) ([]byte, error) {
	if len(ports) == 0 {
		return []byte("[]"), nil
	}

	jsonPorts := make([]map[string]interface{}, len(ports))
	for i, p := range ports {
		jsonPorts[i] = map[string]interface{}{
			"protocol":    string(p.Protocol),
			"port":        p.Port,
			"description": p.Description,
		}
	}

	return json.Marshal(jsonPorts)
}

// marshalLabelsAnnotations marshals labels and annotations to JSONB
func (w *Writer) marshalLabelsAnnotations(labels, annotations map[string]string) ([]byte, []byte, error) {
	var labelsJSON, annotationsJSON []byte
	var err error

	if labels != nil {
		labelsJSON, err = json.Marshal(labels)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to marshal labels")
		}
	} else {
		labelsJSON = []byte("{}")
	}

	if annotations != nil {
		annotationsJSON, err = json.Marshal(annotations)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to marshal annotations")
		}
	} else {
		annotationsJSON = []byte("{}")
	}

	return labelsJSON, annotationsJSON, nil
}

// marshalNetworkItems converts domain NetworkItem slice to JSONB
func (w *Writer) marshalNetworkItems(items []models.NetworkItem) ([]byte, error) {
	if len(items) == 0 {
		return []byte("[]"), nil
	}

	return json.Marshal(items)
}

// marshalAccessPorts handles the complex AccessPorts map marshaling
func (w *Writer) marshalAccessPorts(accessPorts map[models.ServiceRef]models.ServicePorts) ([]byte, error) {
	if len(accessPorts) == 0 {
		return []byte("{}"), nil
	}

	// Convert to map[string]interface{} for JSON marshaling
	jsonMap := make(map[string]interface{})
	for serviceRef, servicePorts := range accessPorts {
		// Use ServiceRef as string key
		key := fmt.Sprintf("%s/%s", serviceRef.Namespace, serviceRef.Name)
		jsonMap[key] = servicePorts
	}

	return json.Marshal(jsonMap)
}

// forcePendingSyncCondition ensures new resources start with Ready=False
// until they are successfully synced to SGROUP.
//
// Business Rule: Resources should NOT have Ready=True until they are confirmed
// to exist in SGROUP. This prevents bindings from being created before resources
// are actually available in the target system.
//
// Workflow:
//  1. Resource created → Ready=False (PendingSGROUPSync)
//  2. Worker syncs to SGROUP → Ready=True (Synced)
//  3. Only then can bindings be created (Migration 033 checks Ready status)
func forcePendingSyncCondition(conditions []metav1.Condition) []metav1.Condition {
	// Remove any existing Ready condition
	filtered := []metav1.Condition{}
	for _, cond := range conditions {
		if cond.Type != "Ready" {
			filtered = append(filtered, cond)
		}
	}

	// Add Ready=False with PendingSGROUPSync reason
	pending := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "PendingSGROUPSync",
		Message:            "Awaiting synchronization with SGROUP before marking as ready",
		LastTransitionTime: metav1.Now(),
	}

	result := append(filtered, pending)

	return result
}

type conditionMergeOptions struct {
	ForcePendingReady bool
}

func (w *Writer) prepareConditionsJSON(
	ctx context.Context,
	existingResourceVersion sql.NullInt64,
	incoming []metav1.Condition,
	opts conditionMergeOptions,
) ([]byte, error) {
	var (
		err      error
		existing []metav1.Condition
	)

	if existingResourceVersion.Valid {
		existing, err = w.fetchConditions(ctx, existingResourceVersion.Int64)
		if err != nil {
			return nil, err
		}
	}

	if opts.ForcePendingReady && !existingResourceVersion.Valid {
		incoming = forcePendingSyncCondition(incoming)
	}

	merged := mergeConditionsByType(existing, incoming)

	conditionsJSON, err := json.Marshal(merged)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal merged conditions")
	}
	return conditionsJSON, nil
}

func (w *Writer) fetchConditions(ctx context.Context, resourceVersion int64) ([]metav1.Condition, error) {
	query := `SELECT conditions FROM k8s_metadata WHERE resource_version = $1`
	var raw []byte
	err := w.tx.QueryRow(ctx, query, resourceVersion).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "failed to fetch existing conditions")
	}

	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var conditions []metav1.Condition
	if err := json.Unmarshal(raw, &conditions); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal existing conditions")
	}
	return conditions, nil
}

func mergeConditionsByType(existing, incoming []metav1.Condition) []metav1.Condition {
	if len(existing) == 0 && len(incoming) == 0 {
		return []metav1.Condition{}
	}

	merged := make(map[string]metav1.Condition, len(existing)+len(incoming))

	for _, cond := range existing {
		if cond.Type == "" {
			continue
		}
		merged[cond.Type] = cond
	}

	for _, cond := range incoming {
		if cond.Type == "" {
			continue
		}
		merged[cond.Type] = normalizeIncomingCondition(cond, merged[cond.Type])
	}

	if len(merged) == 0 {
		return []metav1.Condition{}
	}

	types := make([]string, 0, len(merged))
	for condType := range merged {
		types = append(types, condType)
	}
	priority := map[string]int{
		"Validated":   0,
		"Synced":      1,
		"PendingSync": 2,
		"Ready":       3,
	}
	sort.Slice(types, func(i, j int) bool {
		li, lj := priorityValue(priority, types[i]), priorityValue(priority, types[j])
		if li == lj {
			return types[i] < types[j]
		}
		return li < lj
	})

	result := make([]metav1.Condition, 0, len(types))
	for _, condType := range types {
		result = append(result, merged[condType])
	}

	return result
}

func normalizeIncomingCondition(incoming metav1.Condition, existing metav1.Condition) metav1.Condition {
	normalized := incoming.DeepCopy()
	if normalized == nil {
		return existing
	}

	if normalized.LastTransitionTime.IsZero() {
		if conditionsEquivalent(*normalized, existing) {
			normalized.LastTransitionTime = existing.LastTransitionTime
		} else {
			now := metav1.Now()
			normalized.LastTransitionTime = now
		}
	}

	return *normalized
}

func conditionsEquivalent(a, b metav1.Condition) bool {
	if a.Type != b.Type {
		return false
	}
	if a.Status != b.Status || a.Reason != b.Reason || a.Message != b.Message {
		return false
	}
	return a.ObservedGeneration == b.ObservedGeneration
}

func priorityValue(priority map[string]int, typ string) int {
	if v, ok := priority[typ]; ok {
		return v
	}
	if typ == "" {
		return 1 << 30
	}
	return 100 + int(typ[0])
}
