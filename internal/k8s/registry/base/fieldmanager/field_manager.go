package fieldmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/klog/v2"
)

// FieldManager defines the interface for server-side apply field management
type FieldManager interface {
	// Apply performs server-side apply operation
	Apply(ctx context.Context, obj runtime.Object, patch []byte, fieldManager string, force bool) (runtime.Object, error)

	// Update updates managed fields for regular update operations
	Update(ctx context.Context, obj runtime.Object, fieldManager string) (runtime.Object, error)

	// DetectConflicts identifies conflicts between field managers
	DetectConflicts(current, desired runtime.Object, fieldManager string) ([]Conflict, error)
}

// Conflict represents a field ownership conflict
type Conflict struct {
	Manager string `json:"manager"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ServerSideFieldManager implements FieldManager interface
type ServerSideFieldManager struct {
	managedFieldsManager *ManagedFieldsManager
}

// NewServerSideFieldManager creates a new ServerSideFieldManager
func NewServerSideFieldManager(defaultManager string) *ServerSideFieldManager {
	return &ServerSideFieldManager{
		managedFieldsManager: NewManagedFieldsManager(defaultManager),
	}
}

// Apply implements server-side apply functionality according to Kubernetes server-side apply semantics
func (fm *ServerSideFieldManager) Apply(ctx context.Context, obj runtime.Object, patch []byte, fieldManager string, force bool) (runtime.Object, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot apply to nil object")
	}
	if fieldManager == "" {
		return nil, fmt.Errorf("fieldManager cannot be empty")
	}
	if len(patch) == 0 {
		return nil, fmt.Errorf("patch cannot be empty")
	}

	klog.V(2).InfoS("Starting server-side apply",
		"fieldManager", fieldManager,
		"force", force,
		"patchSize", len(patch))

	// Parse the patch to understand what fields are being applied
	var patchObj map[string]interface{}
	if err := json.Unmarshal(patch, &patchObj); err != nil {
		return nil, fmt.Errorf("failed to parse patch: %w", err)
	}

	// Build FieldsV1 for the patch once, use for conflicts and ownership update
	fieldsV1, err := fm.createFieldsV1FromPatch(patchObj)
	if err != nil {
		klog.ErrorS(err, "Failed to create FieldsV1 from patch", "fieldManager", fieldManager)
		fieldsV1 = &metav1.FieldsV1{Raw: []byte("{}")}
	}

	// Conflicts handling
	if !force {
		conflicts, err := fm.detectConflictsForFields(obj, fieldsV1, fieldManager)
		if err != nil {
			return nil, fmt.Errorf("failed to detect conflicts: %w", err)
		}
		if len(conflicts) > 0 {
			return nil, fmt.Errorf("apply conflicts detected: %v", conflicts)
		}
	}

	// Apply the patch using strategic merge patch
	patchedObj, err := fm.applyPatch(obj, patch)
	if err != nil {
		return nil, fmt.Errorf("failed to apply patch: %w", err)
	}

	// Reassign ownership on the patched object if force is set
	if force {
		if err := fm.reassignOwnershipForFields(patchedObj, fieldsV1, fieldManager); err != nil {
			klog.ErrorS(err, "Failed to reassign ownership during force apply", "fieldManager", fieldManager)
		}
	}

	// Update managed fields on the patched object with the patch field set
	if err := fm.managedFieldsManager.AddManagedFields(
		patchedObj,
		fieldManager,
		metav1.ManagedFieldsOperationApply,
		fieldsV1,
		"",
	); err != nil {
		klog.ErrorS(err, "Failed to update managed fields", "fieldManager", fieldManager)
	}

	klog.V(2).InfoS("Successfully completed server-side apply",
		"fieldManager", fieldManager,
		"force", force)

	return patchedObj, nil
}

// Update implements field tracking for update operations
func (fm *ServerSideFieldManager) Update(ctx context.Context, obj runtime.Object, fieldManager string) (runtime.Object, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot update nil object")
	}

	if fieldManager == "" {
		fieldManager = "netguard-apiserver"
	}

	klog.V(3).InfoS("Updating managed fields for update operation", "fieldManager", fieldManager)

	// Create FieldsV1 from the entire object for update operations
	fieldsV1, err := fm.managedFieldsManager.CreateFieldsV1FromObject(obj)
	if err != nil {
		klog.ErrorS(err, "Failed to create FieldsV1 from object", "fieldManager", fieldManager)
		// Continue without managed fields update rather than failing
		fieldsV1 = &metav1.FieldsV1{Raw: []byte("{}")}
	}

	// Update managed fields for update operation
	err = fm.managedFieldsManager.AddManagedFields(
		obj,
		fieldManager,
		metav1.ManagedFieldsOperationUpdate,
		fieldsV1,
		"",
	)
	if err != nil {
		klog.ErrorS(err, "Failed to update managed fields", "fieldManager", fieldManager)
		// Continue without managed fields update rather than failing
	}

	return obj, nil
}

// DetectConflicts identifies field ownership conflicts between field managers
func (fm *ServerSideFieldManager) DetectConflicts(current, desired runtime.Object, fieldManager string) ([]Conflict, error) {
	if current == nil {
		return nil, fmt.Errorf("cannot detect conflicts with nil current object")
	}

	if fieldManager == "" {
		return nil, fmt.Errorf("fieldManager cannot be empty")
	}

	// Get current managed fields
	managedFields, err := fm.managedFieldsManager.GetManagedFields(current)
	if err != nil {
		return nil, fmt.Errorf("failed to get managed fields: %w", err)
	}

	var conflicts []Conflict

	// Check for conflicts with other managers
	for _, entry := range managedFields {
		if entry.Manager != fieldManager && entry.Operation == metav1.ManagedFieldsOperationApply {
			// This is a simplified conflict detection
			// In production, this would use structured merge diff to detect actual field conflicts
			conflict := Conflict{
				Manager: entry.Manager,
				Field:   "unknown", // Would be determined by field analysis
				Message: fmt.Sprintf("Field managed by %s conflicts with %s", entry.Manager, fieldManager),
			}
			conflicts = append(conflicts, conflict)
		}
	}

	if len(conflicts) > 0 {
		klog.V(2).InfoS("Detected field ownership conflicts",
			"fieldManager", fieldManager,
			"conflictCount", len(conflicts))
	}

	return conflicts, nil
}

// applyPatch applies the patch to the object using strategic merge patch
func (fm *ServerSideFieldManager) applyPatch(obj runtime.Object, patch []byte) (runtime.Object, error) {
	// Convert object to JSON
	originalJSON, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object: %w", err)
	}

	// Get object type for strategic merge patch
	objValue := reflect.ValueOf(obj)
	if objValue.Kind() == reflect.Ptr {
		objValue = objValue.Elem()
	}
	objType := objValue.Type()

	// Apply strategic merge patch
	patchedJSON, err := strategicpatch.StrategicMergePatch(originalJSON, patch, objType)
	if err != nil {
		// Fallback to simple merge if strategic merge fails
		if fm.isStrategicMergeUnsupportedError(err) {
			patchedJSON, err = fm.applySimpleMergePatch(originalJSON, patch)
			if err != nil {
				return nil, fmt.Errorf("failed to apply fallback merge patch: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to apply strategic merge patch: %w", err)
		}
	}

	// Create new object instance
	freshObj := reflect.New(objType).Interface().(runtime.Object)

	// Unmarshal patched JSON into fresh object
	if err := json.Unmarshal(patchedJSON, freshObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patched object: %w", err)
	}

	return freshObj, nil
}

// applySimpleMergePatch applies a simple merge patch as fallback
func (fm *ServerSideFieldManager) applySimpleMergePatch(originalJSON, patch []byte) ([]byte, error) {
	var original, patchObj map[string]interface{}

	if err := json.Unmarshal(originalJSON, &original); err != nil {
		return nil, fmt.Errorf("failed to unmarshal original: %w", err)
	}

	if err := json.Unmarshal(patch, &patchObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patch: %w", err)
	}

	// Simple merge - patch overwrites original
	merged := fm.mergeMaps(original, patchObj)

	return json.Marshal(merged)
}

// mergeMaps performs simple map merging
func (fm *ServerSideFieldManager) mergeMaps(original, patch map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy original
	for k, v := range original {
		result[k] = v
	}

	// Apply patch
	for k, v := range patch {
		if v == nil {
			delete(result, k)
		} else if patchMap, ok := v.(map[string]interface{}); ok {
			if originalMap, exists := result[k]; exists {
				if origMap, ok := originalMap.(map[string]interface{}); ok {
					result[k] = fm.mergeMaps(origMap, patchMap)
				} else {
					result[k] = v
				}
			} else {
				result[k] = v
			}
		} else {
			result[k] = v
		}
	}

	return result
}

// createFieldsV1FromPatch creates FieldsV1 from patch data
func (fm *ServerSideFieldManager) createFieldsV1FromPatch(patchObj map[string]interface{}) (*metav1.FieldsV1, error) {
	// Create simplified FieldsV1 structure from patch
	fieldsMap := fm.createFieldsMapFromPatch(patchObj)

	fieldsJSON, err := json.Marshal(fieldsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal fields map: %w", err)
	}

	return &metav1.FieldsV1{Raw: fieldsJSON}, nil
}

// createFieldsMapFromPatch creates fields map from patch object
func (fm *ServerSideFieldManager) createFieldsMapFromPatch(patchObj map[string]interface{}) map[string]interface{} {
	fieldsMap := make(map[string]interface{})

	for key, value := range patchObj {
		if value == nil {
			// Null values indicate field deletion, still track them
			fieldsMap[key] = map[string]interface{}{}
		} else if valueMap, ok := value.(map[string]interface{}); ok {
			// Nested object
			fieldsMap[key] = fm.createFieldsMapFromPatch(valueMap)
		} else {
			// Primitive value or array
			fieldsMap[key] = map[string]interface{}{}
		}
	}

	return fieldsMap
}

// isStrategicMergeUnsupportedError checks if error indicates strategic merge is unsupported
func (fm *ServerSideFieldManager) isStrategicMergeUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "unable to find api field") ||
		strings.Contains(errStr, "no kind is registered") ||
		strings.Contains(errStr, "strategic merge patch format is not supported")
}

// detectConflictsForFields compares the requested field set against existing managed fields of other managers.
func (fm *ServerSideFieldManager) detectConflictsForFields(current runtime.Object, requested *metav1.FieldsV1, requester string) ([]Conflict, error) {
	if current == nil {
		return nil, fmt.Errorf("cannot detect conflicts with nil current object")
	}
	if requester == "" {
		return nil, fmt.Errorf("fieldManager cannot be empty")
	}

	requestedSet, err := fm.flattenFieldsV1(requested)
	if err != nil {
		return nil, fmt.Errorf("failed to parse requested fields: %w", err)
	}
	if len(requestedSet) == 0 {
		return nil, nil
	}

	managed, err := fm.managedFieldsManager.GetManagedFields(current)
	if err != nil {
		return nil, fmt.Errorf("failed to get managed fields: %w", err)
	}

	var conflicts []Conflict
	for _, entry := range managed {
		if entry.Manager == requester {
			continue
		}
		if entry.Operation != metav1.ManagedFieldsOperationApply {
			// Only apply-managed fields are considered for conflicts
			continue
		}
		otherSet, err := fm.flattenFieldsV1(entry.FieldsV1)
		if err != nil {
			klog.V(3).InfoS("Skipping invalid managed fields entry during conflict detection", "manager", entry.Manager, "error", err)
			continue
		}
		for path := range requestedSet {
			if _, ok := otherSet[path]; ok {
				conflicts = append(conflicts, Conflict{
					Manager: entry.Manager,
					Field:   path,
					Message: fmt.Sprintf("field '%s' managed by %s conflicts with %s", path, entry.Manager, requester),
				})
			}
		}
	}
	if len(conflicts) > 0 {
		klog.V(2).InfoS("Detected conflicts for server-side apply", "fieldManager", requester, "count", len(conflicts))
	}
	return conflicts, nil
}

// reassignOwnershipForFields removes requested field paths from other managers and keeps their remaining ownership.
func (fm *ServerSideFieldManager) reassignOwnershipForFields(obj runtime.Object, requested *metav1.FieldsV1, requester string) error {
	if obj == nil {
		return fmt.Errorf("cannot reassign ownership on nil object")
	}
	acc, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("failed to access object metadata: %w", err)
	}
	requestedSet, err := fm.flattenFieldsV1(requested)
	if err != nil {
		return fmt.Errorf("failed to parse requested fields: %w", err)
	}
	if len(requestedSet) == 0 {
		return nil
	}

	managed := acc.GetManagedFields()
	updated := make([]metav1.ManagedFieldsEntry, 0, len(managed))
	for _, entry := range managed {
		if entry.Manager == requester || entry.Operation != metav1.ManagedFieldsOperationApply {
			updated = append(updated, entry)
			continue
		}
		// Remove requested paths from this manager's FieldsV1
		m, err := fm.decodeFieldsV1(entry.FieldsV1)
		if err != nil {
			klog.V(2).InfoS("Skipping manager during ownership reassignment due to invalid fields", "manager", entry.Manager, "error", err)
			updated = append(updated, entry)
			continue
		}
		removed := 0
		for path := range requestedSet {
			if fm.removePathFromFields(m, path) {
				removed++
			}
		}
		newFields := fm.encodeFieldsMap(m)
		entry.FieldsV1 = newFields
		updated = append(updated, entry)
		if removed > 0 {
			klog.V(2).InfoS("Reassigned field ownership from manager", "from", entry.Manager, "to", requester, "removedPaths", removed)
		}
	}
	acc.SetManagedFields(updated)
	return nil
}

// flattenFieldsV1 turns a FieldsV1 JSON into a set of dotted field paths.
func (fm *ServerSideFieldManager) flattenFieldsV1(fields *metav1.FieldsV1) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	if fields == nil || len(fields.Raw) == 0 {
		return result, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(fields.Raw, &m); err != nil {
		return nil, err
	}
	fm.flattenInto(m, "", result)
	return result, nil
}

func (fm *ServerSideFieldManager) flattenInto(node interface{}, prefix string, out map[string]struct{}) {
	m, ok := node.(map[string]interface{})
	if !ok {
		if prefix != "" {
			out[prefix] = struct{}{}
		}
		return
	}
	if len(m) == 0 {
		if prefix != "" {
			out[prefix] = struct{}{}
		}
		return
	}
	for k, v := range m {
		p := k
		if prefix != "" {
			p = prefix + "." + k
		}
		fm.flattenInto(v, p, out)
	}
}

func (fm *ServerSideFieldManager) decodeFieldsV1(fields *metav1.FieldsV1) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	if fields == nil || len(fields.Raw) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(fields.Raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (fm *ServerSideFieldManager) encodeFieldsMap(m map[string]interface{}) *metav1.FieldsV1 {
	b, _ := json.Marshal(m)
	return &metav1.FieldsV1{Raw: b}
}

// removePathFromFields removes a dotted path from a nested map. Returns true if something was removed.
func (fm *ServerSideFieldManager) removePathFromFields(m map[string]interface{}, path string) bool {
	if path == "" {
		return false
	}
	parts := strings.Split(path, ".")
	return fm.removePathParts(m, parts)
}

func (fm *ServerSideFieldManager) removePathParts(m map[string]interface{}, parts []string) bool {
	if len(parts) == 0 {
		return false
	}
	head := parts[0]
	rest := parts[1:]
	child, ok := m[head]
	if !ok {
		return false
	}
	if len(rest) == 0 {
		delete(m, head)
		return true
	}
	childMap, ok := child.(map[string]interface{})
	if !ok {
		// Not a map, cannot descend; remove child entirely
		delete(m, head)
		return true
	}
	removed := fm.removePathParts(childMap, rest)
	if removed && len(childMap) == 0 {
		delete(m, head)
	}
	return removed
}
