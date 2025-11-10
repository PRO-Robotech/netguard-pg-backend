package convert

import (
	"fmt"
	"reflect"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	APIVersionV1Beta1 = "netguard.sgroups.io/v1beta1"
)

func ConvertMetadataToDomain(objMeta metav1.ObjectMeta, statusConditions []metav1.Condition, observedGeneration int64) models.Meta {
	meta := models.Meta{
		UID:                string(objMeta.UID),
		ResourceVersion:    objMeta.ResourceVersion,
		Generation:         objMeta.Generation,
		CreationTS:         objMeta.CreationTimestamp,
		DeletionTS:         objMeta.DeletionTimestamp,
		GeneratedName:      objMeta.GenerateName,
		ObservedGeneration: observedGeneration,
		Conditions:         statusConditions,
	}
	if objMeta.Labels != nil {
		meta.Labels = make(map[string]string)
		for k, v := range objMeta.Labels {
			meta.Labels[k] = v
		}
	}
	if objMeta.Annotations != nil {
		meta.Annotations = make(map[string]string)
		for k, v := range objMeta.Annotations {
			meta.Annotations[k] = v
		}
	}
	if objMeta.ManagedFields != nil {
		meta.ManagedFields = make([]metav1.ManagedFieldsEntry, len(objMeta.ManagedFields))
		copy(meta.ManagedFields, objMeta.ManagedFields)
	}
	return meta
}
func ConvertMetadataFromDomain(meta models.Meta, name, namespace string) metav1.ObjectMeta {
	objMeta := metav1.ObjectMeta{
		Name:              name,
		Namespace:         namespace,
		UID:               types.UID(meta.UID),
		ResourceVersion:   meta.ResourceVersion,
		Generation:        meta.Generation,
		CreationTimestamp: meta.CreationTS,
		DeletionTimestamp: meta.DeletionTS,
		GenerateName:      meta.GeneratedName,
	}
	if meta.Labels != nil {
		objMeta.Labels = make(map[string]string)
		for k, v := range meta.Labels {
			objMeta.Labels[k] = v
		}
	}
	if meta.Annotations != nil {
		objMeta.Annotations = make(map[string]string)
		for k, v := range meta.Annotations {
			objMeta.Annotations[k] = v
		}
	}
	if meta.ManagedFields != nil {
		objMeta.ManagedFields = make([]metav1.ManagedFieldsEntry, len(meta.ManagedFields))
		copy(objMeta.ManagedFields, meta.ManagedFields)
	}
	return objMeta
}
func ConvertStatusFromDomain(meta models.Meta) ([]metav1.Condition, int64) {
	return meta.Conditions, meta.ObservedGeneration
}
func CreateStandardTypeMetaForResource(kind string) metav1.TypeMeta {
	return metav1.TypeMeta{
		APIVersion: APIVersionV1Beta1,
		Kind:       kind,
	}
}
func CreateStandardTypeMetaForList(listKind string) metav1.TypeMeta {
	return metav1.TypeMeta{
		APIVersion: APIVersionV1Beta1,
		Kind:       listKind,
	}
}
func ValidateNilObject(obj interface{}, objType string) error {
	if obj == nil {
		return fmt.Errorf("%s object is nil", objType)
	}
	if reflect.ValueOf(obj).IsNil() {
		return fmt.Errorf("%s object is nil", objType)
	}
	return nil
}

type NetworkItemConversionHelper struct{}

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
