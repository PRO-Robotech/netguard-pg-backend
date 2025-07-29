package converters

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NetworkBindingConverter converts between NetworkBinding K8s objects and domain models
type NetworkBindingConverter struct{}

// NewNetworkBindingConverter creates a new NetworkBindingConverter
func NewNetworkBindingConverter() *NetworkBindingConverter {
	return &NetworkBindingConverter{}
}

// ToDomain converts a K8s NetworkBinding to a domain NetworkBinding
func (c *NetworkBindingConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.NetworkBinding) (*models.NetworkBinding, error) {
	if k8sObj == nil {
		return nil, nil
	}

	binding := &models.NetworkBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		NetworkRef:      k8sObj.Spec.NetworkRef,
		AddressGroupRef: k8sObj.Spec.AddressGroupRef,
		Meta: models.Meta{
			UID:             string(k8sObj.UID),
			ResourceVersion: k8sObj.ResourceVersion,
			Generation:      k8sObj.Generation,
			CreationTS:      k8sObj.CreationTimestamp,
			Labels:          k8sObj.Labels,
			Annotations:     k8sObj.Annotations,
			Conditions:      k8sObj.Status.Conditions,
		},
	}

	// Convert NetworkItem from the main struct
	binding.NetworkItem = models.NetworkItem{
		Name:       k8sObj.NetworkItem.Name,
		CIDR:       k8sObj.NetworkItem.CIDR,
		ApiVersion: k8sObj.NetworkItem.ApiVersion,
		Kind:       k8sObj.NetworkItem.Kind,
		Namespace:  k8sObj.NetworkItem.Namespace,
	}

	return binding, nil
}

// FromDomain converts a domain NetworkBinding to a K8s NetworkBinding
func (c *NetworkBindingConverter) FromDomain(ctx context.Context, domainObj *models.NetworkBinding) (*netguardv1beta1.NetworkBinding, error) {
	if domainObj == nil {
		return nil, nil
	}

	k8sObj := &netguardv1beta1.NetworkBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NetworkBinding",
			APIVersion: "netguard.sgroups.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              domainObj.Name,
			Namespace:         domainObj.Namespace,
			UID:               types.UID(domainObj.Meta.UID),
			ResourceVersion:   domainObj.Meta.ResourceVersion,
			Generation:        domainObj.Meta.Generation,
			CreationTimestamp: domainObj.Meta.CreationTS,
			Labels:            domainObj.Meta.Labels,
			Annotations:       domainObj.Meta.Annotations,
		},
		Spec: netguardv1beta1.NetworkBindingSpec{
			NetworkRef:      domainObj.NetworkRef,
			AddressGroupRef: domainObj.AddressGroupRef,
		},
		Status: netguardv1beta1.NetworkBindingStatus{
			Conditions: domainObj.Meta.Conditions,
		},
		NetworkItem: netguardv1beta1.NetworkItem{
			Name:       domainObj.NetworkItem.Name,
			CIDR:       domainObj.NetworkItem.CIDR,
			ApiVersion: domainObj.NetworkItem.ApiVersion,
			Kind:       domainObj.NetworkItem.Kind,
			Namespace:  domainObj.NetworkItem.Namespace,
		},
	}

	return k8sObj, nil
}

// ToList converts a slice of domain NetworkBindings to a K8s NetworkBindingList
func (c *NetworkBindingConverter) ToList(ctx context.Context, domainObjs []*models.NetworkBinding) (*netguardv1beta1.NetworkBindingList, error) {
	items := make([]netguardv1beta1.NetworkBinding, 0, len(domainObjs))

	for _, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, err
		}
		if k8sObj != nil {
			items = append(items, *k8sObj)
		}
	}

	return &netguardv1beta1.NetworkBindingList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NetworkBindingList",
			APIVersion: "netguard.sgroups.io/v1beta1",
		},
		Items: items,
	}, nil
}

// convertNetworkItem converts K8s NetworkItem to domain NetworkItem
func convertNetworkItem(k8sItem netguardv1beta1.NetworkItem) models.NetworkItem {
	return models.NetworkItem{
		Name:       k8sItem.Name,
		CIDR:       k8sItem.CIDR,
		ApiVersion: k8sItem.ApiVersion,
		Kind:       k8sItem.Kind,
		Namespace:  k8sItem.Namespace,
	}
}

// convertDomainNetworkItem converts domain NetworkItem to K8s NetworkItem
func convertDomainNetworkItem(domainItem models.NetworkItem) netguardv1beta1.NetworkItem {
	return netguardv1beta1.NetworkItem{
		Name:       domainItem.Name,
		CIDR:       domainItem.CIDR,
		ApiVersion: domainItem.ApiVersion,
		Kind:       domainItem.Kind,
		Namespace:  domainItem.Namespace,
	}
}
