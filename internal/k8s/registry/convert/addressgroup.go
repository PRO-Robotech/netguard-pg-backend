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

// AddressGroupConverter implements conversion between k8s AddressGroup and domain AddressGroup
type AddressGroupConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.AddressGroup, *models.AddressGroup] = &AddressGroupConverter{}

// ToDomain converts a Kubernetes AddressGroup object to a domain AddressGroup model
func (c *AddressGroupConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.AddressGroup) (*models.AddressGroup, error) {
	if err := ValidateNilObject(k8sObj, "k8s AddressGroup"); err != nil {
		return nil, err
	}

	// Convert Networks using standard helper
	networkHelper := &NetworkItemConversionHelper{}
	networks := networkHelper.ConvertNetworkItemsToDomain(k8sObj.Networks)

	// Create domain address group with standard metadata conversion
	domainAddressGroup := &models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		DefaultAction:    models.RuleAction(k8sObj.Spec.DefaultAction),
		Logs:             k8sObj.Spec.Logs,
		Trace:            k8sObj.Spec.Trace,
		Networks:         networks,
		AddressGroupName: k8sObj.Status.AddressGroupName,
		Meta:             ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, k8sObj.Status.ObservedGeneration),
	}

	return domainAddressGroup, nil
}

// FromDomain converts a domain AddressGroup model to a Kubernetes AddressGroup object
func (c *AddressGroupConverter) FromDomain(ctx context.Context, domainObj *models.AddressGroup) (*netguardv1beta1.AddressGroup, error) {
	if err := ValidateNilObject(domainObj, "domain AddressGroup"); err != nil {
		return nil, err
	}

	// Convert Networks using standard helper
	networkHelper := &NetworkItemConversionHelper{}
	networks := networkHelper.ConvertNetworkItemsFromDomain(domainObj.Networks)

	// Create k8s address group with standard metadata conversion
	k8sAddressGroup := &netguardv1beta1.AddressGroup{
		TypeMeta:   CreateStandardTypeMetaForResource("AddressGroup"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.AddressGroupSpec{
			DefaultAction: netguardv1beta1.RuleAction(domainObj.DefaultAction),
			Logs:          domainObj.Logs,
			Trace:         domainObj.Trace,
		},
		Networks: networks,
	}

	// Convert status using standard helper
	conditions, observedGeneration := ConvertStatusFromDomain(domainObj.Meta)
	k8sAddressGroup.Status = netguardv1beta1.AddressGroupStatus{
		AddressGroupName:   domainObj.AddressGroupName,
		ObservedGeneration: observedGeneration,
		Conditions:         conditions,
	}

	return k8sAddressGroup, nil
}

// ToList converts a slice of domain AddressGroup models to a Kubernetes AddressGroupList object
func (c *AddressGroupConverter) ToList(ctx context.Context, domainObjs []*models.AddressGroup) (runtime.Object, error) {
	addressGroupList := &netguardv1beta1.AddressGroupList{
		TypeMeta: CreateStandardTypeMetaForList("AddressGroupList"),
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.AddressGroup, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain address group %d to k8s: %w", i, err)
		}
		addressGroupList.Items[i] = *k8sObj
	}

	return addressGroupList, nil
}

// NewAddressGroupConverter creates a new AddressGroupConverter instance
func NewAddressGroupConverter() *AddressGroupConverter {
	return &AddressGroupConverter{}
}
