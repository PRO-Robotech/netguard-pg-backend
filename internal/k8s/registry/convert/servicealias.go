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

// ServiceAliasConverter implements conversion between k8s ServiceAlias and domain ServiceAlias
type ServiceAliasConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.ServiceAlias, *models.ServiceAlias] = &ServiceAliasConverter{}

// ToDomain converts a Kubernetes ServiceAlias object to a domain ServiceAlias model
func (c *ServiceAliasConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.ServiceAlias) (*models.ServiceAlias, error) {
	if k8sObj == nil {
		return nil, fmt.Errorf("k8s ServiceAlias object is nil")
	}

	// Create domain service alias
	domainAlias := &models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Spec.ServiceRef.Name,
				Namespace: k8sObj.Namespace, // ServiceRef in k8s API is not namespaced, use alias namespace
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
		domainAlias.Meta.Labels = make(map[string]string)
		for k, v := range k8sObj.Labels {
			domainAlias.Meta.Labels[k] = v
		}
	}

	if k8sObj.Annotations != nil {
		domainAlias.Meta.Annotations = make(map[string]string)
		for k, v := range k8sObj.Annotations {
			domainAlias.Meta.Annotations[k] = v
		}
	}

	return domainAlias, nil
}

// FromDomain converts a domain ServiceAlias model to a Kubernetes ServiceAlias object
func (c *ServiceAliasConverter) FromDomain(ctx context.Context, domainObj *models.ServiceAlias) (*netguardv1beta1.ServiceAlias, error) {
	if domainObj == nil {
		return nil, fmt.Errorf("domain ServiceAlias object is nil")
	}

	// Create k8s service alias
	k8sAlias := &netguardv1beta1.ServiceAlias{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "ServiceAlias",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              domainObj.ResourceIdentifier.Name,
			Namespace:         domainObj.ResourceIdentifier.Namespace,
			UID:               types.UID(domainObj.Meta.UID),
			ResourceVersion:   domainObj.Meta.ResourceVersion,
			Generation:        domainObj.Meta.Generation,
			CreationTimestamp: domainObj.Meta.CreationTS,
		},
		Spec: netguardv1beta1.ServiceAliasSpec{
			ServiceRef: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       domainObj.ServiceRef.Name,
			},
		},
	}

	// Copy metadata
	if domainObj.Meta.Labels != nil {
		k8sAlias.Labels = make(map[string]string)
		for k, v := range domainObj.Meta.Labels {
			k8sAlias.Labels[k] = v
		}
	}

	if domainObj.Meta.Annotations != nil {
		k8sAlias.Annotations = make(map[string]string)
		for k, v := range domainObj.Meta.Annotations {
			k8sAlias.Annotations[k] = v
		}
	}

	// Convert status - переносим условия из Meta в Status
	k8sAlias.Status = netguardv1beta1.ServiceAliasStatus{
		ObservedGeneration: domainObj.Meta.ObservedGeneration,
		Conditions:         domainObj.Meta.Conditions, // Backend формирует условия
	}

	return k8sAlias, nil
}

// ToList converts a slice of domain ServiceAlias models to a Kubernetes ServiceAliasList object
func (c *ServiceAliasConverter) ToList(ctx context.Context, domainObjs []*models.ServiceAlias) (runtime.Object, error) {
	aliasList := &netguardv1beta1.ServiceAliasList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "ServiceAliasList",
		},
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.ServiceAlias, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain service alias %d to k8s: %w", i, err)
		}
		aliasList.Items[i] = *k8sObj
	}

	return aliasList, nil
}

// NewServiceAliasConverter creates a new ServiceAliasConverter instance
func NewServiceAliasConverter() *ServiceAliasConverter {
	return &ServiceAliasConverter{}
}
