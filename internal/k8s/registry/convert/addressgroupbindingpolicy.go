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

// AddressGroupBindingPolicyConverter implements conversion between k8s AddressGroupBindingPolicy and domain AddressGroupBindingPolicy
type AddressGroupBindingPolicyConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.AddressGroupBindingPolicy, *models.AddressGroupBindingPolicy] = &AddressGroupBindingPolicyConverter{}

// ToDomain converts a Kubernetes AddressGroupBindingPolicy object to a domain AddressGroupBindingPolicy model
func (c *AddressGroupBindingPolicyConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.AddressGroupBindingPolicy) (*models.AddressGroupBindingPolicy, error) {
	if k8sObj == nil {
		return nil, fmt.Errorf("k8s AddressGroupBindingPolicy object is nil")
	}

	// Create domain address group binding policy
	domainPolicy := &models.AddressGroupBindingPolicy{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Spec.AddressGroupRef.Name,
				Namespace: k8sObj.Spec.AddressGroupRef.Namespace,
			},
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Spec.ServiceRef.Name,
				Namespace: k8sObj.Spec.ServiceRef.Namespace,
			},
		},
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
		domainPolicy.Meta.Labels = make(map[string]string)
		for k, v := range k8sObj.Labels {
			domainPolicy.Meta.Labels[k] = v
		}
	}

	if k8sObj.Annotations != nil {
		domainPolicy.Meta.Annotations = make(map[string]string)
		for k, v := range k8sObj.Annotations {
			domainPolicy.Meta.Annotations[k] = v
		}
	}

	return domainPolicy, nil
}

// FromDomain converts a domain AddressGroupBindingPolicy model to a Kubernetes AddressGroupBindingPolicy object
func (c *AddressGroupBindingPolicyConverter) FromDomain(ctx context.Context, domainObj *models.AddressGroupBindingPolicy) (*netguardv1beta1.AddressGroupBindingPolicy, error) {
	if domainObj == nil {
		return nil, fmt.Errorf("domain AddressGroupBindingPolicy object is nil")
	}

	// Create k8s address group binding policy
	k8sPolicy := &netguardv1beta1.AddressGroupBindingPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupBindingPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              domainObj.ResourceIdentifier.Name,
			Namespace:         domainObj.ResourceIdentifier.Namespace,
			UID:               types.UID(domainObj.Meta.UID),
			ResourceVersion:   domainObj.Meta.ResourceVersion,
			Generation:        domainObj.Meta.Generation,
			CreationTimestamp: domainObj.Meta.CreationTS,
		},
		Spec: netguardv1beta1.AddressGroupBindingPolicySpec{
			AddressGroupRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       domainObj.AddressGroupRef.Name,
				},
				Namespace: domainObj.AddressGroupRef.Namespace,
			},
			ServiceRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       domainObj.ServiceRef.Name,
				},
				Namespace: domainObj.ServiceRef.Namespace,
			},
		},
	}

	// Copy metadata
	if domainObj.Meta.Labels != nil {
		k8sPolicy.Labels = make(map[string]string)
		for k, v := range domainObj.Meta.Labels {
			k8sPolicy.Labels[k] = v
		}
	}

	if domainObj.Meta.Annotations != nil {
		k8sPolicy.Annotations = make(map[string]string)
		for k, v := range domainObj.Meta.Annotations {
			k8sPolicy.Annotations[k] = v
		}
	}

	// Convert status - переносим условия из Meta в Status
	k8sPolicy.Status = netguardv1beta1.AddressGroupBindingPolicyStatus{
		ObservedGeneration: domainObj.Meta.ObservedGeneration,
		Conditions:         domainObj.Meta.Conditions, // Backend формирует условия
	}

	return k8sPolicy, nil
}

// ToList converts a slice of domain AddressGroupBindingPolicy models to a Kubernetes AddressGroupBindingPolicyList object
func (c *AddressGroupBindingPolicyConverter) ToList(ctx context.Context, domainObjs []*models.AddressGroupBindingPolicy) (runtime.Object, error) {
	policyList := &netguardv1beta1.AddressGroupBindingPolicyList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupBindingPolicyList",
		},
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
