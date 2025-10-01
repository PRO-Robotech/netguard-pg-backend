package converters

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

// NetworkConverter converts between Network K8s objects and domain models
type NetworkConverter struct{}

// NewNetworkConverter creates a new NetworkConverter
func NewNetworkConverter() *NetworkConverter {
	return &NetworkConverter{}
}

// ToDomain converts a K8s Network to a domain Network
func (c *NetworkConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.Network) (*models.Network, error) {
	if k8sObj == nil {
		return nil, nil
	}

	network := &models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		CIDR: k8sObj.Spec.CIDR,
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

	// Convert status fields
	network.NetworkName = k8sObj.Status.NetworkName
	network.IsBound = k8sObj.Status.IsBound
	network.BindingRef = k8sObj.Status.BindingRef
	network.AddressGroupRef = k8sObj.Status.AddressGroupRef

	return network, nil
}

// FromDomain converts a domain Network to a K8s Network
func (c *NetworkConverter) FromDomain(ctx context.Context, domainObj *models.Network) (*netguardv1beta1.Network, error) {
	if domainObj == nil {
		return nil, nil
	}

	// Debug logging
	klog.Infof("🔍 CONVERTER FromDomain: Network[%s] has %d conditions, IsBound=%t", domainObj.Key(), len(domainObj.Meta.Conditions), domainObj.IsBound)
	if domainObj.BindingRef != nil {
	} else {
	}
	if domainObj.AddressGroupRef != nil {
	} else {
	}

	k8sObj := &netguardv1beta1.Network{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Network",
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
		Spec: netguardv1beta1.NetworkSpec{
			CIDR: domainObj.CIDR,
		},
		Status: netguardv1beta1.NetworkStatus{
			NetworkName:     domainObj.NetworkName,
			IsBound:         domainObj.IsBound,
			BindingRef:      domainObj.BindingRef,
			AddressGroupRef: domainObj.AddressGroupRef,
			Conditions:      domainObj.Meta.Conditions,
		},
	}

	return k8sObj, nil
}

// ToList converts a slice of domain Networks to a K8s NetworkList
func (c *NetworkConverter) ToList(ctx context.Context, domainObjs []*models.Network) (*netguardv1beta1.NetworkList, error) {
	items := make([]netguardv1beta1.Network, 0, len(domainObjs))

	for _, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, err
		}
		if k8sObj != nil {
			items = append(items, *k8sObj)
		}
	}

	return &netguardv1beta1.NetworkList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NetworkList",
			APIVersion: "netguard.sgroups.io/v1beta1",
		},
		Items: items,
	}, nil
}
