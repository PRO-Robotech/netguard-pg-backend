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

// AddressGroupBindingPolicyConverter implements conversion between k8s AddressGroupBindingPolicy and domain AddressGroupBindingPolicy
type AddressGroupBindingPolicyConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.AddressGroupBindingPolicy, *models.AddressGroupBindingPolicy] = &AddressGroupBindingPolicyConverter{}

// ToDomain converts a Kubernetes AddressGroupBindingPolicy object to a domain AddressGroupBindingPolicy model
func (c *AddressGroupBindingPolicyConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.AddressGroupBindingPolicy) (*models.AddressGroupBindingPolicy, error) {
	if err := ValidateNilObject(k8sObj, "k8s AddressGroupBindingPolicy"); err != nil {
		return nil, err
	}

	// Create domain address group binding policy
	domainPolicy := &models.AddressGroupBindingPolicy{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		AddressGroupRef: k8sObj.Spec.AddressGroupRef,
		ServiceRef:      k8sObj.Spec.ServiceRef,
		Meta:            ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, k8sObj.Status.ObservedGeneration),
	}

	// Metadata already converted by ConvertMetadataToDomain helper

	return domainPolicy, nil
}

// FromDomain converts a domain AddressGroupBindingPolicy model to a Kubernetes AddressGroupBindingPolicy object
func (c *AddressGroupBindingPolicyConverter) FromDomain(ctx context.Context, domainObj *models.AddressGroupBindingPolicy) (*netguardv1beta1.AddressGroupBindingPolicy, error) {
	if err := ValidateNilObject(domainObj, "domain AddressGroupBindingPolicy"); err != nil {
		return nil, err
	}

	// Create k8s address group binding policy
	k8sPolicy := &netguardv1beta1.AddressGroupBindingPolicy{
		TypeMeta:   CreateStandardTypeMetaForResource("AddressGroupBindingPolicy"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.AddressGroupBindingPolicySpec{
			AddressGroupRef: EnsureNamespacedObjectReferenceFields(domainObj.AddressGroupRef, "AddressGroup"),
			ServiceRef:      EnsureNamespacedObjectReferenceFields(domainObj.ServiceRef, "Service"),
		},
	}

	// Metadata already converted by ConvertMetadataFromDomain helper

	// Convert status using standard helper
	conditions, observedGeneration := ConvertStatusFromDomain(domainObj.Meta)
	k8sPolicy.Status = netguardv1beta1.AddressGroupBindingPolicyStatus{
		ObservedGeneration: observedGeneration,
		Conditions:         conditions,
	}

	return k8sPolicy, nil
}

// ToList converts a slice of domain AddressGroupBindingPolicy models to a Kubernetes AddressGroupBindingPolicyList object
func (c *AddressGroupBindingPolicyConverter) ToList(ctx context.Context, domainObjs []*models.AddressGroupBindingPolicy) (runtime.Object, error) {
	policyList := &netguardv1beta1.AddressGroupBindingPolicyList{
		TypeMeta: CreateStandardTypeMetaForList("AddressGroupBindingPolicyList"),
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.AddressGroupBindingPolicy, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain address group binding policy %d to k8s: %w", i, err)
		}
		policyList.Items[i] = *k8sObj
	}

	return policyList, nil
}

// NewAddressGroupBindingPolicyConverter creates a new AddressGroupBindingPolicyConverter instance
func NewAddressGroupBindingPolicyConverter() *AddressGroupBindingPolicyConverter {
	return &AddressGroupBindingPolicyConverter{}
}
