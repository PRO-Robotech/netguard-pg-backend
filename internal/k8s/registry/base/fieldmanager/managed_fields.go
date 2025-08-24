package fieldmanager

import (
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ManagedFieldsEntry represents a single entry in the managedFields list
// This structure follows the Kubernetes API specification for ManagedFields
type ManagedFieldsEntry struct {
	// Manager is an identifier of the workflow managing these fields
	Manager string `json:"manager,omitempty"`

	// Operation is the type of operation which lead to this ManagedFieldsEntry being created
	// The only valid values for this field are 'Apply' and 'Update'
	Operation metav1.ManagedFieldsOperationType `json:"operation,omitempty"`

	// APIVersion defines the version of this resource that this field set applies to
	// The format is "group/version" just like the top-level APIVersion field
	APIVersion string `json:"apiVersion,omitempty"`

	// Time is the timestamp of when the ManagedFields entry was added
	// The timestamp will also be updated if a field is added, the manager changes any of the owned fields value or removes a field
	Time *metav1.Time `json:"time,omitempty"`

	// FieldsType is the discriminator for the different fields format and version
	// There is currently only one possible value: "FieldsV1"
	FieldsType string `json:"fieldsType,omitempty"`

	// FieldsV1 stores a set of fields in a data structure like a Trie, in JSON format
	// Each key is either a '.' representing the field itself, and will always map to an empty set,
	// or a string representing a sub-field or item
	FieldsV1 *metav1.FieldsV1 `json:"fieldsV1,omitempty"`

	// Subresource is the name of the subresource used to update that object, or empty string if the object was updated through the main resource
	Subresource string `json:"subresource,omitempty"`
}

// ManagedFieldsManager handles the management of ManagedFields for Kubernetes objects
type ManagedFieldsManager struct {
	// defaultManager is the default field manager name when none is specified
	defaultManager string
}

// NewManagedFieldsManager creates a new ManagedFieldsManager instance
func NewManagedFieldsManager(defaultManager string) *ManagedFieldsManager {
	if defaultManager == "" {
		defaultManager = "netguard-apiserver"
	}
	return &ManagedFieldsManager{
		defaultManager: defaultManager,
	}
}

// AddManagedFields adds or updates a managed fields entry for the given object
func (m *ManagedFieldsManager) AddManagedFields(
	obj runtime.Object,
	manager string,
	operation metav1.ManagedFieldsOperationType,
	fieldsV1 *metav1.FieldsV1,
	subresource string,
) error {
	if obj == nil {
		return fmt.Errorf("cannot add managed fields to nil object")
	}

	// Get object accessor to access metadata
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("failed to get object metadata: %w", err)
	}

	// Use default manager if none specified
	if manager == "" {
		manager = m.defaultManager
	}

	// Get current managed fields
	managedFields := accessor.GetManagedFields()
	if managedFields == nil {
		managedFields = []metav1.ManagedFieldsEntry{}
	}

	// Get API version from the object
	apiVersion := obj.GetObjectKind().GroupVersionKind().GroupVersion().String()
	if apiVersion == "" {
		// Fallback to getting from accessor
		if gvk := obj.GetObjectKind().GroupVersionKind(); !gvk.Empty() {
			apiVersion = gvk.GroupVersion().String()
		}
		if apiVersion == "" {
			apiVersion = "v1" // Default fallback
		}
	}

	// Create new managed fields entry
	now := metav1.NewTime(time.Now())
	newEntry := metav1.ManagedFieldsEntry{
		Manager:     manager,
		Operation:   operation,
		APIVersion:  apiVersion,
		Time:        &now,
		FieldsType:  "FieldsV1",
		FieldsV1:    fieldsV1,
		Subresource: subresource,
	}

	// Find existing entry for this manager and operation
	existingIndex := -1
	for i, entry := range managedFields {
		if entry.Manager == manager && entry.Operation == operation && entry.Subresource == subresource {
			existingIndex = i
			break
		}
	}

	// Update existing entry or add new one
	if existingIndex >= 0 {
		managedFields[existingIndex] = newEntry
	} else {
		managedFields = append(managedFields, newEntry)
	}

	// Set updated managed fields back to the object
	accessor.SetManagedFields(managedFields)

	return nil
}

// UpdateManagedFields updates the managed fields for a specific manager
func (m *ManagedFieldsManager) UpdateManagedFields(
	obj runtime.Object,
	manager string,
	operation metav1.ManagedFieldsOperationType,
	fieldsV1 *metav1.FieldsV1,
) error {
	return m.AddManagedFields(obj, manager, operation, fieldsV1, "")
}

// RemoveManagedFields removes managed fields entries for a specific manager
func (m *ManagedFieldsManager) RemoveManagedFields(obj runtime.Object, manager string) error {
	if obj == nil {
		return fmt.Errorf("cannot remove managed fields from nil object")
	}

	// Get object accessor to access metadata
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("failed to get object metadata: %w", err)
	}

	// Get current managed fields
	managedFields := accessor.GetManagedFields()
	if managedFields == nil {
		return nil // Nothing to remove
	}

	// Filter out entries for the specified manager
	filteredFields := make([]metav1.ManagedFieldsEntry, 0, len(managedFields))
	for _, entry := range managedFields {
		if entry.Manager != manager {
			filteredFields = append(filteredFields, entry)
		}
	}

	// Set filtered managed fields back to the object
	accessor.SetManagedFields(filteredFields)

	return nil
}

// GetManagedFields returns the managed fields for an object
func (m *ManagedFieldsManager) GetManagedFields(obj runtime.Object) ([]metav1.ManagedFieldsEntry, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot get managed fields from nil object")
	}

	// Get object accessor to access metadata
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get object metadata: %w", err)
	}

	return accessor.GetManagedFields(), nil
}

// GetFieldsForManager returns the fields managed by a specific manager
func (m *ManagedFieldsManager) GetFieldsForManager(obj runtime.Object, manager string) (*metav1.FieldsV1, error) {
	managedFields, err := m.GetManagedFields(obj)
	if err != nil {
		return nil, err
	}

	for _, entry := range managedFields {
		if entry.Manager == manager {
			return entry.FieldsV1, nil
		}
	}

	return nil, nil // Manager not found
}

// HasManager checks if a specific manager has managed fields for the object
func (m *ManagedFieldsManager) HasManager(obj runtime.Object, manager string) (bool, error) {
	managedFields, err := m.GetManagedFields(obj)
	if err != nil {
		return false, err
	}

	for _, entry := range managedFields {
		if entry.Manager == manager {
			return true, nil
		}
	}

	return false, nil
}

// GetManagers returns a list of all managers that have managed fields for the object
func (m *ManagedFieldsManager) GetManagers(obj runtime.Object) ([]string, error) {
	managedFields, err := m.GetManagedFields(obj)
	if err != nil {
		return nil, err
	}

	managers := make([]string, 0, len(managedFields))
	seen := make(map[string]bool)

	for _, entry := range managedFields {
		if !seen[entry.Manager] {
			managers = append(managers, entry.Manager)
			seen[entry.Manager] = true
		}
	}

	return managers, nil
}

// SerializeManagedFields serializes managed fields to JSON
func (m *ManagedFieldsManager) SerializeManagedFields(managedFields []metav1.ManagedFieldsEntry) ([]byte, error) {
	return json.Marshal(managedFields)
}

// DeserializeManagedFields deserializes managed fields from JSON
func (m *ManagedFieldsManager) DeserializeManagedFields(data []byte) ([]metav1.ManagedFieldsEntry, error) {
	var managedFields []metav1.ManagedFieldsEntry
	if err := json.Unmarshal(data, &managedFields); err != nil {
		return nil, fmt.Errorf("failed to deserialize managed fields: %w", err)
	}
	return managedFields, nil
}

// ValidateManagedFields validates the structure and content of managed fields
func (m *ManagedFieldsManager) ValidateManagedFields(managedFields []metav1.ManagedFieldsEntry) error {
	for i, entry := range managedFields {
		if entry.Manager == "" {
			return fmt.Errorf("managed field entry %d: manager cannot be empty", i)
		}

		if entry.Operation != metav1.ManagedFieldsOperationApply && entry.Operation != metav1.ManagedFieldsOperationUpdate {
			return fmt.Errorf("managed field entry %d: invalid operation '%s', must be 'Apply' or 'Update'", i, entry.Operation)
		}

		if entry.APIVersion == "" {
			return fmt.Errorf("managed field entry %d: apiVersion cannot be empty", i)
		}

		if entry.FieldsType != "" && entry.FieldsType != "FieldsV1" {
			return fmt.Errorf("managed field entry %d: invalid fieldsType '%s', must be 'FieldsV1'", i, entry.FieldsType)
		}

		if entry.Time == nil {
			return fmt.Errorf("managed field entry %d: time cannot be nil", i)
		}
	}

	return nil
}

// CreateFieldsV1FromObject creates a FieldsV1 representation from a runtime.Object
// This is a simplified implementation - in production, this would use the structured merge diff library
func (m *ManagedFieldsManager) CreateFieldsV1FromObject(obj runtime.Object) (*metav1.FieldsV1, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot create FieldsV1 from nil object")
	}

	// Convert object to JSON
	objJSON, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	// Parse JSON to extract field structure
	var objMap map[string]interface{}
	if err := json.Unmarshal(objJSON, &objMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal object JSON: %w", err)
	}

	// Create simplified FieldsV1 structure
	// In production, this would use k8s.io/apimachinery/pkg/util/managedfields
	fieldsMap := m.createFieldsMap(objMap)

	fieldsJSON, err := json.Marshal(fieldsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal fields map: %w", err)
	}

	return &metav1.FieldsV1{Raw: fieldsJSON}, nil
}

// createFieldsMap creates a simplified fields map from an object map
// This is a basic implementation - production would use the structured merge diff library
func (m *ManagedFieldsManager) createFieldsMap(objMap map[string]interface{}) map[string]interface{} {
	fieldsMap := make(map[string]interface{})

	for key, value := range objMap {
		// Skip metadata fields that shouldn't be tracked
		if key == "metadata" {
			if metaMap, ok := value.(map[string]interface{}); ok {
				// Only track specific metadata fields
				trackedMeta := make(map[string]interface{})
				for metaKey := range metaMap {
					if metaKey == "labels" || metaKey == "annotations" {
						trackedMeta[metaKey] = map[string]interface{}{}
					}
				}
				if len(trackedMeta) > 0 {
					fieldsMap[key] = trackedMeta
				}
			}
		} else {
			// For other fields, create empty map structure
			fieldsMap[key] = m.createFieldStructure(value)
		}
	}

	return fieldsMap
}

// createFieldStructure creates a field structure representation
func (m *ManagedFieldsManager) createFieldStructure(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key := range v {
			result[key] = map[string]interface{}{}
		}
		return result
	case []interface{}:
		// For arrays, we track the presence of the array itself
		return map[string]interface{}{}
	default:
		// For primitive values, we track the field itself
		return map[string]interface{}{}
	}
}

// MergeFieldsV1 merges two FieldsV1 structures
// This is a simplified implementation - production would use structured merge diff
func (m *ManagedFieldsManager) MergeFieldsV1(base, overlay *metav1.FieldsV1) (*metav1.FieldsV1, error) {
	if base == nil && overlay == nil {
		return nil, nil
	}
	if base == nil {
		return overlay, nil
	}
	if overlay == nil {
		return base, nil
	}

	// Parse both FieldsV1 structures
	var baseMap, overlayMap map[string]interface{}

	if err := json.Unmarshal(base.Raw, &baseMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal base FieldsV1: %w", err)
	}

	if err := json.Unmarshal(overlay.Raw, &overlayMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal overlay FieldsV1: %w", err)
	}

	// Merge the maps
	merged := m.mergeFieldsMaps(baseMap, overlayMap)

	// Convert back to FieldsV1
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged fields: %w", err)
	}

	return &metav1.FieldsV1{Raw: mergedJSON}, nil
}

// mergeFieldsMaps merges two fields maps
func (m *ManagedFieldsManager) mergeFieldsMaps(base, overlay map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy base fields
	for key, value := range base {
		result[key] = value
	}

	// Merge overlay fields
	for key, overlayValue := range overlay {
		if baseValue, exists := result[key]; exists {
			// If both are maps, merge recursively
			if baseMap, ok := baseValue.(map[string]interface{}); ok {
				if overlayMap, ok := overlayValue.(map[string]interface{}); ok {
					result[key] = m.mergeFieldsMaps(baseMap, overlayMap)
					continue
				}
			}
		}
		// Otherwise, overlay wins
		result[key] = overlayValue
	}

	return result
}
