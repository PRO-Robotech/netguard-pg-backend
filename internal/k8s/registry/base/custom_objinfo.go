package base

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"
)

// netguardUpdatedObjectInfo implements rest.UpdatedObjectInfo to bypass the internal lookup
// that fails in rest.defaultUpdatedObjectInfo for our aggregated API resources
type netguardUpdatedObjectInfo struct {
	patchType types.PatchType
	patchData []byte
	gvk       schema.GroupVersionKind
}

// NewNetguardUpdatedObjectInfo creates a custom UpdatedObjectInfo that works with our aggregated API
func NewNetguardUpdatedObjectInfo(patchType types.PatchType, patchData []byte, gvk schema.GroupVersionKind) rest.UpdatedObjectInfo {
	return &netguardUpdatedObjectInfo{
		patchType: patchType,
		patchData: patchData,
		gvk:       gvk,
	}
}

// UpdatedObject applies the patch to the current object without doing any internal lookups
func (n *netguardUpdatedObjectInfo) UpdatedObject(ctx context.Context, oldObj runtime.Object) (runtime.Object, error) {
	klog.InfoS("🔧 NetguardUpdatedObjectInfo.UpdatedObject CALLED",
		"patchType", string(n.patchType),
		"oldObjType", fmt.Sprintf("%T", oldObj),
		"patchDataLength", len(n.patchData))

	if oldObj == nil {
		klog.InfoS("❌ NetguardUpdatedObjectInfo: oldObj is nil")
		return nil, errors.NewBadRequest("cannot patch: current object is nil")
	}

	// Apply the patch based on type
	switch n.patchType {
	case types.JSONPatchType:
		return n.applyJSONPatch(oldObj)
	case types.MergePatchType:
		return n.applyMergePatch(oldObj)
	case types.StrategicMergePatchType:
		return n.applyStrategicMergePatch(oldObj)
	default:
		klog.InfoS("❌ NetguardUpdatedObjectInfo: unsupported patch type", "patchType", string(n.patchType))
		return nil, errors.NewBadRequest(fmt.Sprintf("unsupported patch type: %s", n.patchType))
	}
}

// Preconditions returns no preconditions - we trust the backend validation
func (n *netguardUpdatedObjectInfo) Preconditions() *metav1.Preconditions {
	return nil
}

// applyMergePatch applies a JSON merge patch (RFC 7396)
func (n *netguardUpdatedObjectInfo) applyMergePatch(oldObj runtime.Object) (runtime.Object, error) {
	klog.InfoS("🔧 Applying merge patch", "patchData", string(n.patchData))

	// Convert current object to JSON
	oldJSON, err := json.Marshal(oldObj)
	if err != nil {
		klog.InfoS("❌ Failed to marshal old object", "error", err.Error())
		return nil, fmt.Errorf("failed to marshal current object: %w", err)
	}

	// Apply merge patch
	newJSON, err := n.applyMergePatchToJSON(oldJSON, n.patchData)
	if err != nil {
		klog.InfoS("❌ Failed to apply merge patch", "error", err.Error())
		return nil, fmt.Errorf("failed to apply merge patch: %w", err)
	}

	// Unmarshal back to object
	newObj := oldObj.DeepCopyObject()
	if err := json.Unmarshal(newJSON, newObj); err != nil {
		klog.InfoS("❌ Failed to unmarshal patched object", "error", err.Error())
		return nil, fmt.Errorf("failed to unmarshal patched object: %w", err)
	}

	klog.InfoS("✅ Merge patch applied successfully")
	return newObj, nil
}

// applyMergePatchToJSON implements RFC 7396 JSON Merge Patch
func (n *netguardUpdatedObjectInfo) applyMergePatchToJSON(oldJSON, patchJSON []byte) ([]byte, error) {
	var oldObj interface{}
	var patchObj interface{}

	if err := json.Unmarshal(oldJSON, &oldObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal old JSON: %w", err)
	}

	if err := json.Unmarshal(patchJSON, &patchObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patch JSON: %w", err)
	}

	// Apply merge patch logic
	merged := n.mergePatchObjects(oldObj, patchObj)

	return json.Marshal(merged)
}

// mergePatchObjects implements the merge patch algorithm
func (n *netguardUpdatedObjectInfo) mergePatchObjects(old, patch interface{}) interface{} {
	if patch == nil {
		return nil
	}

	patchMap, isPatchMap := patch.(map[string]interface{})
	if !isPatchMap {
		// If patch is not an object, it replaces the old value
		return patch
	}

	oldMap, isOldMap := old.(map[string]interface{})
	if !isOldMap {
		// If old is not an object, patch replaces it completely
		return patch
	}

	// Merge objects
	result := make(map[string]interface{})

	// Copy all fields from old object
	for key, value := range oldMap {
		result[key] = value
	}

	// Apply patch fields
	for key, patchValue := range patchMap {
		if patchValue == nil {
			// null in patch means delete the field
			delete(result, key)
		} else if _, isPatchValueMap := patchValue.(map[string]interface{}); isPatchValueMap {
			// Recursively merge nested objects
			if oldValue, exists := result[key]; exists {
				result[key] = n.mergePatchObjects(oldValue, patchValue)
			} else {
				result[key] = patchValue
			}
		} else {
			// Replace scalar values
			result[key] = patchValue
		}
	}

	return result
}

// applyJSONPatch applies a JSON patch (RFC 6902)
func (n *netguardUpdatedObjectInfo) applyJSONPatch(oldObj runtime.Object) (runtime.Object, error) {
	klog.InfoS("❌ JSON Patch not implemented yet", "patchData", string(n.patchData))
	return nil, errors.NewBadRequest("JSON patch not implemented in custom objInfo")
}

// applyStrategicMergePatch applies a strategic merge patch
func (n *netguardUpdatedObjectInfo) applyStrategicMergePatch(oldObj runtime.Object) (runtime.Object, error) {
	klog.InfoS("❌ Strategic Merge Patch not implemented yet", "patchData", string(n.patchData))
	// For now, fall back to merge patch
	return n.applyMergePatch(oldObj)
}
