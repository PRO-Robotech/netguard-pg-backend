package convert

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// NetworkBindingConverter implements conversion between k8s NetworkBinding and domain NetworkBinding
type NetworkBindingConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.NetworkBinding, *models.NetworkBinding] = &NetworkBindingConverter{}

// ToDomain converts a Kubernetes NetworkBinding object to a domain NetworkBinding model
func (c *NetworkBindingConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.NetworkBinding) (*models.NetworkBinding, error) {
	if err := ValidateNilObject(k8sObj, "k8s NetworkBinding"); err != nil {
		return nil, err
	}

	// Create domain network binding with standard metadata conversion
	domainBinding := &models.NetworkBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		NetworkRef:      k8sObj.Spec.NetworkRef,
		AddressGroupRef: k8sObj.Spec.AddressGroupRef,
		// NetworkItem is derived from domain model, not stored in K8s status
		Meta: ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, 0), // NetworkBinding doesn't have ObservedGeneration
	}

	return domainBinding, nil
}

// FromDomain converts a domain NetworkBinding model to a Kubernetes NetworkBinding object
func (c *NetworkBindingConverter) FromDomain(ctx context.Context, domainObj *models.NetworkBinding) (*netguardv1beta1.NetworkBinding, error) {
	if err := ValidateNilObject(domainObj, "domain NetworkBinding"); err != nil {
		return nil, err
	}

	// Create k8s network binding with standard metadata conversion
	k8sBinding := &netguardv1beta1.NetworkBinding{
		TypeMeta:   CreateStandardTypeMetaForResource("NetworkBinding"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.NetworkBindingSpec{
			NetworkRef:      EnsureObjectReferenceFields(domainObj.NetworkRef, "Network"),
			AddressGroupRef: EnsureObjectReferenceFields(domainObj.AddressGroupRef, "AddressGroup"),
		},
	}

	// Convert status - NetworkBinding doesn't have ObservedGeneration or NetworkItem in status
	k8sBinding.Status = netguardv1beta1.NetworkBindingStatus{
		Conditions: domainObj.Meta.Conditions,
	}

	return k8sBinding, nil
}

// ToList converts a slice of domain NetworkBinding models to a Kubernetes NetworkBindingList object
func (c *NetworkBindingConverter) ToList(ctx context.Context, domainObjs []*models.NetworkBinding) (runtime.Object, error) {
	bindingList := &netguardv1beta1.NetworkBindingList{
		TypeMeta: CreateStandardTypeMetaForList("NetworkBindingList"),
		Items:    make([]netguardv1beta1.NetworkBinding, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain network binding %d to k8s: %w", i, err)
		}
		bindingList.Items[i] = *k8sObj
	}

	return bindingList, nil
}

// NewNetworkBindingConverter creates a new NetworkBindingConverter instance
func NewNetworkBindingConverter() *NetworkBindingConverter {
	return &NetworkBindingConverter{}
}
