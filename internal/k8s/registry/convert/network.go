package convert

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// NetworkConverter implements conversion between k8s Network and domain Network
type NetworkConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.Network, *models.Network] = &NetworkConverter{}

// ToDomain converts a Kubernetes Network object to a domain Network model
func (c *NetworkConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.Network) (*models.Network, error) {
	if err := ValidateNilObject(k8sObj, "k8s Network"); err != nil {
		return nil, err
	}

	// Create domain network with standard metadata conversion
	domainNetwork := &models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		CIDR:            k8sObj.Spec.CIDR,
		NetworkName:     k8sObj.Status.NetworkName,
		IsBound:         k8sObj.Status.IsBound,
		BindingRef:      k8sObj.Status.BindingRef,
		AddressGroupRef: k8sObj.Status.AddressGroupRef,
		Meta:            ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, 0), // Network doesn't have ObservedGeneration
	}

	return domainNetwork, nil
}

// FromDomain converts a domain Network model to a Kubernetes Network object
func (c *NetworkConverter) FromDomain(ctx context.Context, domainObj *models.Network) (*netguardv1beta1.Network, error) {
	if err := ValidateNilObject(domainObj, "domain Network"); err != nil {
		return nil, err
	}

	// Create k8s network with standard metadata conversion
	k8sNetwork := &netguardv1beta1.Network{
		TypeMeta:   CreateStandardTypeMetaForResource("Network"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.NetworkSpec{
			CIDR: domainObj.CIDR,
		},
	}

	// Convert status - Network doesn't have ObservedGeneration
	k8sNetwork.Status = netguardv1beta1.NetworkStatus{
		NetworkName: domainObj.NetworkName,
		IsBound:     domainObj.IsBound,
		Conditions:  domainObj.Meta.Conditions,
	}

	// Ensure ObjectReference fields in status are properly set
	if domainObj.BindingRef != nil {
		bindingRef := EnsureObjectReferenceFields(*domainObj.BindingRef, "NetworkBinding")
		k8sNetwork.Status.BindingRef = &bindingRef
	}
	if domainObj.AddressGroupRef != nil {
		addressGroupRef := EnsureObjectReferenceFields(*domainObj.AddressGroupRef, "AddressGroup")
		k8sNetwork.Status.AddressGroupRef = &addressGroupRef
	}

	return k8sNetwork, nil
}

// ToList converts a slice of domain Network models to a Kubernetes NetworkList object
func (c *NetworkConverter) ToList(ctx context.Context, domainObjs []*models.Network) (runtime.Object, error) {
	networkList := &netguardv1beta1.NetworkList{
		TypeMeta: CreateStandardTypeMetaForList("NetworkList"),
		Items:    make([]netguardv1beta1.Network, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain network %d to k8s: %w", i, err)
		}
		networkList.Items[i] = *k8sObj
	}

	return networkList, nil
}

// NewNetworkConverter creates a new NetworkConverter instance
func NewNetworkConverter() *NetworkConverter {
	return &NetworkConverter{}
}
