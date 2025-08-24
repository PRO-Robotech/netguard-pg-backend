package convert

import (
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// Constants for NetGuard API
const (
	// APIVersionV1Beta1 is the standard API version for NetGuard resources
	APIVersionV1Beta1 = "netguard.sgroups.io/v1beta1"
)

// Standard conversion helpers to eliminate code duplication across all converters.
// This infrastructure centralizes metadata conversion logic and ensures proper
// ManagedFields handling for Server-Side Apply support.

// ConvertMetadataToDomain converts Kubernetes ObjectMeta to domain Meta model
// This helper eliminates ~30 lines of duplicated code in each converter's ToDomain method
func ConvertMetadataToDomain(objMeta metav1.ObjectMeta, statusConditions []metav1.Condition, observedGeneration int64) models.Meta {
	meta := models.Meta{
		UID:                string(objMeta.UID),
		ResourceVersion:    objMeta.ResourceVersion,
		Generation:         objMeta.Generation,
		CreationTS:         objMeta.CreationTimestamp,
		GeneratedName:      objMeta.GenerateName,
		ObservedGeneration: observedGeneration,
		Conditions:         statusConditions,
	}

	// Copy labels
	if objMeta.Labels != nil {
		meta.Labels = make(map[string]string)
		for k, v := range objMeta.Labels {
			meta.Labels[k] = v
		}
	}

	// Copy annotations
	if objMeta.Annotations != nil {
		meta.Annotations = make(map[string]string)
		for k, v := range objMeta.Annotations {
			meta.Annotations[k] = v
		}
	}

	// 🚨 CRITICAL: ManagedFields preservation (fixes visibility issue)
	// This ensures ManagedFields are properly preserved across conversions
	if objMeta.ManagedFields != nil {
		meta.ManagedFields = make([]metav1.ManagedFieldsEntry, len(objMeta.ManagedFields))
		copy(meta.ManagedFields, objMeta.ManagedFields)
	}

	return meta
}

// ConvertMetadataFromDomain converts domain Meta model to Kubernetes ObjectMeta
// This helper eliminates ~30 lines of duplicated code in each converter's FromDomain method
func ConvertMetadataFromDomain(meta models.Meta, name, namespace string) metav1.ObjectMeta {
	objMeta := metav1.ObjectMeta{
		Name:              name,
		Namespace:         namespace,
		UID:               types.UID(meta.UID),
		ResourceVersion:   meta.ResourceVersion,
		Generation:        meta.Generation,
		CreationTimestamp: meta.CreationTS,
		GenerateName:      meta.GeneratedName,
	}

	// Copy labels
	if meta.Labels != nil {
		objMeta.Labels = make(map[string]string)
		for k, v := range meta.Labels {
			objMeta.Labels[k] = v
		}
	}

	// Copy annotations
	if meta.Annotations != nil {
		objMeta.Annotations = make(map[string]string)
		for k, v := range meta.Annotations {
			objMeta.Annotations[k] = v
		}
	}

	// 🚨 CRITICAL: ManagedFields restoration (fixes visibility issue)
	// This ensures ManagedFields are properly restored in K8s objects
	if meta.ManagedFields != nil {
		objMeta.ManagedFields = make([]metav1.ManagedFieldsEntry, len(meta.ManagedFields))
		copy(objMeta.ManagedFields, meta.ManagedFields)
	}

	return objMeta
}

// ConvertStatusFromDomain converts domain Meta conditions to Kubernetes status with ObservedGeneration
// This helper standardizes status conversion across all resource types
func ConvertStatusFromDomain(meta models.Meta) ([]metav1.Condition, int64) {
	return meta.Conditions, meta.ObservedGeneration
}

// CreateStandardTypeMetaForResource creates TypeMeta for a given resource type
// This helper ensures consistent APIVersion and Kind across all converters
func CreateStandardTypeMetaForResource(kind string) metav1.TypeMeta {
	return metav1.TypeMeta{
		APIVersion: APIVersionV1Beta1,
		Kind:       kind,
	}
}

// CreateStandardTypeMetaForList creates TypeMeta for list objects
// This helper ensures consistent List APIVersion and Kind across all converters
func CreateStandardTypeMetaForList(listKind string) metav1.TypeMeta {
	return metav1.TypeMeta{
		APIVersion: APIVersionV1Beta1,
		Kind:       listKind,
	}
}

// ValidateNilObject performs standard nil validation for converter inputs
// This helper eliminates repetitive nil checks across all converters
func ValidateNilObject(obj interface{}, objType string) error {
	if obj == nil {
		return fmt.Errorf("%s object is nil", objType)
	}
	// Check for typed nil values (interface containing nil)
	if reflect.ValueOf(obj).IsNil() {
		return fmt.Errorf("%s object is nil", objType)
	}
	return nil
}

// NetworkItemConversionHelper handles conversion between domain and K8s NetworkItem types
// This helper eliminates NetworkItem conversion duplication in AddressGroup-related converters
type NetworkItemConversionHelper struct{}

// ConvertNetworkItemsToDomain converts K8s NetworkItems to domain NetworkItems
func (h *NetworkItemConversionHelper) ConvertNetworkItemsToDomain(k8sItems []netguardv1beta1.NetworkItem) []models.NetworkItem {
	if len(k8sItems) == 0 {
		return nil
	}

	networks := make([]models.NetworkItem, len(k8sItems))
	for i, item := range k8sItems {
		networks[i] = models.NetworkItem{
			Name:       item.Name,
			CIDR:       item.CIDR,
			ApiVersion: item.ApiVersion,
			Kind:       item.Kind,
			Namespace:  item.Namespace,
		}
	}
	return networks
}

// ConvertNetworkItemsFromDomain converts domain NetworkItems to K8s NetworkItems
func (h *NetworkItemConversionHelper) ConvertNetworkItemsFromDomain(domainItems []models.NetworkItem) []netguardv1beta1.NetworkItem {
	if len(domainItems) == 0 {
		return nil
	}

	networks := make([]netguardv1beta1.NetworkItem, len(domainItems))
	for i, item := range domainItems {
		networks[i] = netguardv1beta1.NetworkItem{
			Name:       item.Name,
			CIDR:       item.CIDR,
			ApiVersion: item.ApiVersion,
			Kind:       item.Kind,
			Namespace:  item.Namespace,
		}
	}
	return networks
}

// ObjectReference Helper Functions

// EnsureObjectReferenceFields ensures that ObjectReference has proper apiVersion and kind
// This function fixes the core issue where ObjectReference fields are lost during conversions
func EnsureObjectReferenceFields(objRef netguardv1beta1.ObjectReference, kind string) netguardv1beta1.ObjectReference {
	result := objRef
	if result.APIVersion == "" {
		result.APIVersion = APIVersionV1Beta1
	}
	if result.Kind == "" {
		result.Kind = kind
	}
	return result
}

// EnsureNamespacedObjectReferenceFields ensures that NamespacedObjectReference has proper apiVersion and kind
// This function fixes the core issue where NamespacedObjectReference fields are lost during conversions
func EnsureNamespacedObjectReferenceFields(objRef netguardv1beta1.NamespacedObjectReference, kind string) netguardv1beta1.NamespacedObjectReference {
	result := objRef
	if result.APIVersion == "" {
		result.APIVersion = APIVersionV1Beta1
	}
	if result.Kind == "" {
		result.Kind = kind
	}
	return result
}
