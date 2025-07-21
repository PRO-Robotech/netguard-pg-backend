package convert

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

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
	if k8sObj == nil {
		return nil, fmt.Errorf("k8s AddressGroup object is nil")
	}

	// Create domain address group
	domainAddressGroup := &models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		DefaultAction: models.RuleAction(k8sObj.Spec.DefaultAction),
		Logs:          k8sObj.Spec.Logs,
		Trace:         k8sObj.Spec.Trace,
		Meta: models.Meta{
			UID:                string(k8sObj.UID),
			ResourceVersion:    k8sObj.ResourceVersion,
			Generation:         k8sObj.Generation,
			CreationTS:         k8sObj.CreationTimestamp,
			ObservedGeneration: k8sObj.Status.ObservedGeneration,
			Conditions:         k8sObj.Status.Conditions,
		},
	}

	// Copy metadata
	if k8sObj.Labels != nil {
		domainAddressGroup.Meta.Labels = make(map[string]string)
		for k, v := range k8sObj.Labels {
			domainAddressGroup.Meta.Labels[k] = v
		}
	}

	if k8sObj.Annotations != nil {
		domainAddressGroup.Meta.Annotations = make(map[string]string)
		for k, v := range k8sObj.Annotations {
			domainAddressGroup.Meta.Annotations[k] = v
		}
	}

	return domainAddressGroup, nil
}

// FromDomain converts a domain AddressGroup model to a Kubernetes AddressGroup object
func (c *AddressGroupConverter) FromDomain(ctx context.Context, domainObj *models.AddressGroup) (*netguardv1beta1.AddressGroup, error) {
	if domainObj == nil {
		return nil, fmt.Errorf("domain AddressGroup object is nil")
	}

	// Create k8s address group
	k8sAddressGroup := &netguardv1beta1.AddressGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              domainObj.ResourceIdentifier.Name,
			Namespace:         domainObj.ResourceIdentifier.Namespace,
			UID:               types.UID(domainObj.Meta.UID),
			ResourceVersion:   domainObj.Meta.ResourceVersion,
			Generation:        domainObj.Meta.Generation,
			CreationTimestamp: domainObj.Meta.CreationTS,
		},
		Spec: netguardv1beta1.AddressGroupSpec{
			DefaultAction: netguardv1beta1.RuleAction(domainObj.DefaultAction),
			Logs:          domainObj.Logs,
			Trace:         domainObj.Trace,
		},
	}

	// Copy metadata
	if domainObj.Meta.Labels != nil {
		k8sAddressGroup.Labels = make(map[string]string)
		for k, v := range domainObj.Meta.Labels {
			k8sAddressGroup.Labels[k] = v
		}
	}

	if domainObj.Meta.Annotations != nil {
		k8sAddressGroup.Annotations = make(map[string]string)
		for k, v := range domainObj.Meta.Annotations {
			k8sAddressGroup.Annotations[k] = v
		}
	}

	// Convert status - переносим условия из Meta в Status
	k8sAddressGroup.Status = netguardv1beta1.AddressGroupStatus{
		ObservedGeneration: domainObj.Meta.ObservedGeneration,
		Conditions:         domainObj.Meta.Conditions, // Backend формирует условия
	}

	return k8sAddressGroup, nil
}

// ToList converts a slice of domain AddressGroup models to a Kubernetes AddressGroupList object
func (c *AddressGroupConverter) ToList(ctx context.Context, domainObjs []*models.AddressGroup) (runtime.Object, error) {
	addressGroupList := &netguardv1beta1.AddressGroupList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupList",
		},
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
