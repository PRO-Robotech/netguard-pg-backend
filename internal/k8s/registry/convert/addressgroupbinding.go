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

// AddressGroupBindingConverter implements conversion between k8s AddressGroupBinding and domain AddressGroupBinding
type AddressGroupBindingConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.AddressGroupBinding, *models.AddressGroupBinding] = &AddressGroupBindingConverter{}

// ToDomain converts a Kubernetes AddressGroupBinding object to a domain AddressGroupBinding model
func (c *AddressGroupBindingConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.AddressGroupBinding) (*models.AddressGroupBinding, error) {
	if k8sObj == nil {
		return nil, fmt.Errorf("k8s AddressGroupBinding object is nil")
	}

	// Create domain address group binding
	domainBinding := &models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Spec.ServiceRef.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Spec.AddressGroupRef.Name,
				Namespace: k8sObj.Spec.AddressGroupRef.Namespace,
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
		domainBinding.Meta.Labels = make(map[string]string)
		for k, v := range k8sObj.Labels {
			domainBinding.Meta.Labels[k] = v
		}
	}

	if k8sObj.Annotations != nil {
		domainBinding.Meta.Annotations = make(map[string]string)
		for k, v := range k8sObj.Annotations {
			domainBinding.Meta.Annotations[k] = v
		}
	}

	return domainBinding, nil
}

// FromDomain converts a domain AddressGroupBinding model to a Kubernetes AddressGroupBinding object
func (c *AddressGroupBindingConverter) FromDomain(ctx context.Context, domainObj *models.AddressGroupBinding) (*netguardv1beta1.AddressGroupBinding, error) {
	if domainObj == nil {
		return nil, fmt.Errorf("domain AddressGroupBinding object is nil")
	}

	// Create k8s address group binding
	k8sBinding := &netguardv1beta1.AddressGroupBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              domainObj.ResourceIdentifier.Name,
			Namespace:         domainObj.ResourceIdentifier.Namespace,
			UID:               types.UID(domainObj.Meta.UID),
			ResourceVersion:   domainObj.Meta.ResourceVersion,
			Generation:        domainObj.Meta.Generation,
			CreationTimestamp: domainObj.Meta.CreationTS,
		},
		Spec: netguardv1beta1.AddressGroupBindingSpec{
			ServiceRef: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       domainObj.ServiceRef.Name,
			},
			AddressGroupRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       domainObj.AddressGroupRef.Name,
				},
				Namespace: domainObj.AddressGroupRef.Namespace,
			},
		},
	}

	// Copy metadata
	if domainObj.Meta.Labels != nil {
		k8sBinding.Labels = make(map[string]string)
		for k, v := range domainObj.Meta.Labels {
			k8sBinding.Labels[k] = v
		}
	}

	if domainObj.Meta.Annotations != nil {
		k8sBinding.Annotations = make(map[string]string)
		for k, v := range domainObj.Meta.Annotations {
			k8sBinding.Annotations[k] = v
		}
	}

	// Convert status - переносим условия из Meta в Status
	k8sBinding.Status = netguardv1beta1.AddressGroupBindingStatus{
		ObservedGeneration: domainObj.Meta.ObservedGeneration,
		Conditions:         domainObj.Meta.Conditions, // Backend формирует условия
	}

	return k8sBinding, nil
}

// ToList converts a slice of domain AddressGroupBinding models to a Kubernetes AddressGroupBindingList object
func (c *AddressGroupBindingConverter) ToList(ctx context.Context, domainObjs []*models.AddressGroupBinding) (runtime.Object, error) {
	bindingList := &netguardv1beta1.AddressGroupBindingList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupBindingList",
		},
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.AddressGroupBinding, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain address group binding %d to k8s: %w", i, err)
		}
		bindingList.Items[i] = *k8sObj
	}

	return bindingList, nil
}

// NewAddressGroupBindingConverter creates a new AddressGroupBindingConverter instance
func NewAddressGroupBindingConverter() *AddressGroupBindingConverter {
	return &AddressGroupBindingConverter{}
}
