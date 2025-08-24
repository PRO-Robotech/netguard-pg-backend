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

// ServiceAliasConverter implements conversion between k8s ServiceAlias and domain ServiceAlias
type ServiceAliasConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.ServiceAlias, *models.ServiceAlias] = &ServiceAliasConverter{}

// ToDomain converts a Kubernetes ServiceAlias object to a domain ServiceAlias model
func (c *ServiceAliasConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.ServiceAlias) (*models.ServiceAlias, error) {
	if err := ValidateNilObject(k8sObj, "k8s ServiceAlias"); err != nil {
		return nil, err
	}

	// Create domain service alias
	domainAlias := &models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		ServiceRef: k8sObj.Spec.ServiceRef,
		Meta:       ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, k8sObj.Status.ObservedGeneration),
	}

	// Metadata already converted by ConvertMetadataToDomain helper

	return domainAlias, nil
}

// FromDomain converts a domain ServiceAlias model to a Kubernetes ServiceAlias object
func (c *ServiceAliasConverter) FromDomain(ctx context.Context, domainObj *models.ServiceAlias) (*netguardv1beta1.ServiceAlias, error) {
	if err := ValidateNilObject(domainObj, "domain ServiceAlias"); err != nil {
		return nil, err
	}

	// Create k8s service alias
	k8sAlias := &netguardv1beta1.ServiceAlias{
		TypeMeta:   CreateStandardTypeMetaForResource("ServiceAlias"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.ServiceAliasSpec{
			ServiceRef: EnsureNamespacedObjectReferenceFields(domainObj.ServiceRef, "Service"),
		},
	}

	// Metadata already converted by ConvertMetadataFromDomain helper

	// Convert status using standard helper
	conditions, observedGeneration := ConvertStatusFromDomain(domainObj.Meta)
	k8sAlias.Status = netguardv1beta1.ServiceAliasStatus{
		ObservedGeneration: observedGeneration,
		Conditions:         conditions,
	}

	return k8sAlias, nil
}

// ToList converts a slice of domain ServiceAlias models to a Kubernetes ServiceAliasList object
func (c *ServiceAliasConverter) ToList(ctx context.Context, domainObjs []*models.ServiceAlias) (runtime.Object, error) {
	aliasList := &netguardv1beta1.ServiceAliasList{
		TypeMeta: CreateStandardTypeMetaForList("ServiceAliasList"),
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
