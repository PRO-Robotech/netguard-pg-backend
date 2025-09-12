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

// HostBindingConverter implements conversion between k8s HostBinding and domain HostBinding
type HostBindingConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.HostBinding, *models.HostBinding] = &HostBindingConverter{}

// ToDomain converts a Kubernetes HostBinding object to a domain HostBinding model
func (c *HostBindingConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.HostBinding) (*models.HostBinding, error) {
	if err := ValidateNilObject(k8sObj, "k8s HostBinding"); err != nil {
		return nil, err
	}

	// Create domain host binding with standard metadata conversion
	domainHostBinding := &models.HostBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		HostRef:         k8sObj.Spec.HostRef,
		AddressGroupRef: k8sObj.Spec.AddressGroupRef,
		Meta:            ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, k8sObj.Status.ObservedGeneration),
	}

	return domainHostBinding, nil
}

// FromDomain converts a domain HostBinding model to a Kubernetes HostBinding object
func (c *HostBindingConverter) FromDomain(ctx context.Context, domainObj *models.HostBinding) (*netguardv1beta1.HostBinding, error) {
	if err := ValidateNilObject(domainObj, "domain HostBinding"); err != nil {
		return nil, err
	}

	// Create k8s host binding object
	k8sHostBinding := &netguardv1beta1.HostBinding{
		TypeMeta:   CreateStandardTypeMetaForResource("HostBinding"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.HostBindingSpec{
			HostRef:         domainObj.HostRef,
			AddressGroupRef: domainObj.AddressGroupRef,
		},
	}

	// Convert status using standard helper
	conditions, observedGeneration := ConvertStatusFromDomain(domainObj.Meta)
	k8sHostBinding.Status = netguardv1beta1.HostBindingStatus{
		ObservedGeneration: observedGeneration,
		Conditions:         conditions,
	}

	return k8sHostBinding, nil
}

// ToList converts a slice of domain HostBinding models to a Kubernetes HostBindingList object
func (c *HostBindingConverter) ToList(ctx context.Context, domainObjs []*models.HostBinding) (runtime.Object, error) {
	hostBindingList := &netguardv1beta1.HostBindingList{
		TypeMeta: CreateStandardTypeMetaForList("HostBindingList"),
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.HostBinding, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain host binding %d to k8s: %w", i, err)
		}
		hostBindingList.Items[i] = *k8sObj
	}

	return hostBindingList, nil
}
