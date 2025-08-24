package convert

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// AddressGroupBindingConverter implements conversion between k8s AddressGroupBinding and domain AddressGroupBinding
type AddressGroupBindingConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.AddressGroupBinding, *models.AddressGroupBinding] = &AddressGroupBindingConverter{}

// ToDomain converts a Kubernetes AddressGroupBinding object to a domain AddressGroupBinding model
func (c *AddressGroupBindingConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.AddressGroupBinding) (*models.AddressGroupBinding, error) {
	if err := ValidateNilObject(k8sObj, "k8s AddressGroupBinding"); err != nil {
		return nil, err
	}

	// Validate and process ServiceRef
	if k8sObj.Spec.ServiceRef.Name == "" {
		return nil, fmt.Errorf("AddressGroupBinding %s/%s has empty ServiceRef.Name", k8sObj.Namespace, k8sObj.Name)
	}

	// Validate and process AddressGroupRef
	if k8sObj.Spec.AddressGroupRef.Name == "" {
		return nil, fmt.Errorf("AddressGroupBinding %s/%s has empty AddressGroupRef.Name", k8sObj.Namespace, k8sObj.Name)
	}

	// Create domain address group binding
	domainBinding := &models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		ServiceRef:      k8sObj.Spec.ServiceRef,
		AddressGroupRef: k8sObj.Spec.AddressGroupRef,
		Meta:            ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, k8sObj.Status.ObservedGeneration),
	}

	// Metadata already converted by ConvertMetadataToDomain helper

	return domainBinding, nil
}

// FromDomain converts a domain AddressGroupBinding model to a Kubernetes AddressGroupBinding object
func (c *AddressGroupBindingConverter) FromDomain(ctx context.Context, domainObj *models.AddressGroupBinding) (*netguardv1beta1.AddressGroupBinding, error) {
	if err := ValidateNilObject(domainObj, "domain AddressGroupBinding"); err != nil {
		return nil, err
	}

	// Create k8s address group binding with standard metadata conversion
	k8sBinding := &netguardv1beta1.AddressGroupBinding{
		TypeMeta:   CreateStandardTypeMetaForResource("AddressGroupBinding"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.AddressGroupBindingSpec{
			ServiceRef:      EnsureNamespacedObjectReferenceFields(domainObj.ServiceRef, "Service"),
			AddressGroupRef: EnsureNamespacedObjectReferenceFields(domainObj.AddressGroupRef, "AddressGroup"),
		},
	}

	// Metadata already converted by ConvertMetadataFromDomain helper

	// Convert status using standard helper
	conditions, observedGeneration := ConvertStatusFromDomain(domainObj.Meta)
	k8sBinding.Status = netguardv1beta1.AddressGroupBindingStatus{
		ObservedGeneration: observedGeneration,
		Conditions:         conditions,
	}

	return k8sBinding, nil
}

// ToList converts a slice of domain AddressGroupBinding models to a Kubernetes AddressGroupBindingList object
func (c *AddressGroupBindingConverter) ToList(ctx context.Context, domainObjs []*models.AddressGroupBinding) (runtime.Object, error) {
	bindingList := &netguardv1beta1.AddressGroupBindingList{
		TypeMeta: CreateStandardTypeMetaForList("AddressGroupBindingList"),
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.AddressGroupBinding, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain address group binding %d to k8s: %w", i, err)
		}
		bindingList.Items[i] = *k8sObj
	}

	return bindingList, nil
}

// NewAddressGroupBindingConverter creates a new AddressGroupBindingConverter instance
func NewAddressGroupBindingConverter() *AddressGroupBindingConverter {
	return &AddressGroupBindingConverter{}
}
