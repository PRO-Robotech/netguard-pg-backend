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

// HostConverter implements conversion between k8s Host and domain Host
type HostConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.Host, *models.Host] = &HostConverter{}

// ToDomain converts a Kubernetes Host object to a domain Host model
func (c *HostConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.Host) (*models.Host, error) {
	if err := ValidateNilObject(k8sObj, "k8s Host"); err != nil {
		return nil, err
	}

	// Create domain host with standard metadata conversion
	domainHost := &models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		UUID:            k8sObj.Spec.UUID,
		HostName:        k8sObj.Status.HostName,
		IsBound:         k8sObj.Status.IsBound,
		BindingRef:      k8sObj.Status.BindingRef,
		AddressGroupRef: k8sObj.Status.AddressGroupRef,
		Meta:            ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, k8sObj.Status.ObservedGeneration),
	}

	return domainHost, nil
}

// FromDomain converts a domain Host model to a Kubernetes Host object
func (c *HostConverter) FromDomain(ctx context.Context, domainObj *models.Host) (*netguardv1beta1.Host, error) {
	if err := ValidateNilObject(domainObj, "domain Host"); err != nil {
		return nil, err
	}

	// Create k8s host object
	k8sHost := &netguardv1beta1.Host{
		TypeMeta:   CreateStandardTypeMetaForResource("Host"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.HostSpec{
			UUID: domainObj.UUID,
		},
		Status: netguardv1beta1.HostStatus{
			HostName:        domainObj.HostName,
			IsBound:         domainObj.IsBound,
			BindingRef:      domainObj.BindingRef,
			AddressGroupRef: domainObj.AddressGroupRef,
		},
	}

	// Convert status using standard helper
	conditions, observedGeneration := ConvertStatusFromDomain(domainObj.Meta)
	k8sHost.Status.ObservedGeneration = observedGeneration
	k8sHost.Status.Conditions = conditions

	return k8sHost, nil
}

// ToList converts a slice of domain Host models to a Kubernetes HostList object
func (c *HostConverter) ToList(ctx context.Context, domainObjs []*models.Host) (runtime.Object, error) {
	hostList := &netguardv1beta1.HostList{
		TypeMeta: CreateStandardTypeMetaForList("HostList"),
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.Host, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain host %d to k8s: %w", i, err)
		}
		hostList.Items[i] = *k8sObj
	}

	return hostList, nil
}
