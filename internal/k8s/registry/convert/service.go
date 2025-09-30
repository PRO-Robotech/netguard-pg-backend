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

// ServiceConverter implements conversion between k8s Service and domain Service
type ServiceConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.Service, *models.Service] = &ServiceConverter{}

// ToDomain converts a Kubernetes Service object to a domain Service model
func (c *ServiceConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.Service) (*models.Service, error) {
	if err := ValidateNilObject(k8sObj, "k8s Service"); err != nil {
		return nil, err
	}

	// Create domain service with standard metadata conversion
	domainService := &models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		Description: k8sObj.Spec.Description,
		Meta:        ConvertMetadataToDomain(k8sObj.ObjectMeta, k8sObj.Status.Conditions, k8sObj.Status.ObservedGeneration),
	}

	// Convert ingress ports
	if len(k8sObj.Spec.IngressPorts) > 0 {
		domainService.IngressPorts = make([]models.IngressPort, len(k8sObj.Spec.IngressPorts))
		for i, port := range k8sObj.Spec.IngressPorts {
			domainService.IngressPorts[i] = models.IngressPort{
				Protocol:    models.TransportProtocol(port.Protocol),
				Port:        port.Port,
				Description: port.Description,
			}
		}
	}

	// Convert AddressGroups from Spec
	if len(k8sObj.Spec.AddressGroups) > 0 {
		domainService.AddressGroups = make([]models.AddressGroupRef, len(k8sObj.Spec.AddressGroups))
		for i, agRef := range k8sObj.Spec.AddressGroups {
			domainService.AddressGroups[i] = models.NewAddressGroupRef(agRef.Name, models.WithNamespace(agRef.Namespace))
		}
	}

	// Convert AggregatedAddressGroups (from ROOT level, not Status!)
	aggregatedAGs := convertAddressGroupReferencesToDomain(k8sObj.AggregatedAddressGroups)
	domainService.AggregatedAddressGroups = aggregatedAGs

	return domainService, nil
}

// FromDomain converts a domain Service model to a Kubernetes Service object
func (c *ServiceConverter) FromDomain(ctx context.Context, domainObj *models.Service) (*netguardv1beta1.Service, error) {
	if err := ValidateNilObject(domainObj, "domain Service"); err != nil {
		return nil, err
	}

	// Create k8s service with standard metadata conversion
	k8sService := &netguardv1beta1.Service{
		TypeMeta:   CreateStandardTypeMetaForResource("Service"),
		ObjectMeta: ConvertMetadataFromDomain(domainObj.Meta, domainObj.ResourceIdentifier.Name, domainObj.ResourceIdentifier.Namespace),
		Spec: netguardv1beta1.ServiceSpec{
			Description: domainObj.Description,
		},
	}

	// Convert ingress ports
	if len(domainObj.IngressPorts) > 0 {
		k8sService.Spec.IngressPorts = make([]netguardv1beta1.IngressPort, len(domainObj.IngressPorts))
		for i, port := range domainObj.IngressPorts {
			k8sService.Spec.IngressPorts[i] = netguardv1beta1.IngressPort{
				Protocol:    netguardv1beta1.TransportProtocol(port.Protocol),
				Port:        port.Port,
				Description: port.Description,
			}
		}
	}

	// Convert AddressGroups from domain to Spec
	if len(domainObj.AddressGroups) > 0 {
		k8sService.Spec.AddressGroups = make([]netguardv1beta1.NamespacedObjectReference, len(domainObj.AddressGroups))
		for i, agRef := range domainObj.AddressGroups {
			k8sService.Spec.AddressGroups[i] = netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       agRef.Name,
				},
				Namespace: agRef.Namespace,
			}
		}
	}

	// Convert AggregatedAddressGroups to ROOT level (not Status!)
	aggregatedAGsK8s := convertAddressGroupReferencesToK8s(domainObj.AggregatedAddressGroups)

	// Defensive: If AggregatedAddressGroups is empty but Spec.AddressGroups is not,
	// populate AggregatedAddressGroups from Spec to maintain data consistency.
	// This handles the case before PostgreSQL triggers populate aggregated data.
	if len(aggregatedAGsK8s) == 0 && len(k8sService.Spec.AddressGroups) > 0 {
		aggregatedAGsK8s = make([]netguardv1beta1.AddressGroupReference, len(k8sService.Spec.AddressGroups))
		for i, ag := range k8sService.Spec.AddressGroups {
			aggregatedAGsK8s[i] = netguardv1beta1.AddressGroupReference{
				Ref:    ag,
				Source: netguardv1beta1.AddressGroupSourceSpec,
			}
		}
	}

	k8sService.AggregatedAddressGroups = aggregatedAGsK8s

	// Convert status using standard helper
	conditions, observedGeneration := ConvertStatusFromDomain(domainObj.Meta)
	k8sService.Status = netguardv1beta1.ServiceStatus{
		ObservedGeneration: observedGeneration,
		Conditions:         conditions,
	}

	return k8sService, nil
}

// ToList converts a slice of domain Service models to a Kubernetes ServiceList object
func (c *ServiceConverter) ToList(ctx context.Context, domainObjs []*models.Service) (runtime.Object, error) {
	serviceList := &netguardv1beta1.ServiceList{
		TypeMeta: CreateStandardTypeMetaForList("ServiceList"),
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.Service, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain service %d to k8s: %w", i, err)
		}
		serviceList.Items[i] = *k8sObj
	}

	return serviceList, nil
}

// NewServiceConverter creates a new ServiceConverter instance
func NewServiceConverter() *ServiceConverter {
	return &ServiceConverter{}
}

// convertAddressGroupReferencesToDomain converts K8s AddressGroupReference slice to domain AddressGroupReference slice
func convertAddressGroupReferencesToDomain(k8sAGRefs []netguardv1beta1.AddressGroupReference) []models.AddressGroupReference {
	if k8sAGRefs == nil {
		return nil
	}

	domainAGRefs := make([]models.AddressGroupReference, len(k8sAGRefs))
	for i, k8sRef := range k8sAGRefs {
		domainAGRefs[i] = models.AddressGroupReference{
			Ref:    k8sRef.Ref,
			Source: models.AddressGroupRegistrationSource(k8sRef.Source),
		}
	}
	return domainAGRefs
}

// convertAddressGroupReferencesToK8s converts domain AddressGroupReference slice to K8s AddressGroupReference slice
func convertAddressGroupReferencesToK8s(domainAGRefs []models.AddressGroupReference) []netguardv1beta1.AddressGroupReference {
	if domainAGRefs == nil {
		return nil
	}

	k8sAGRefs := make([]netguardv1beta1.AddressGroupReference, len(domainAGRefs))
	for i, domainRef := range domainAGRefs {
		k8sAGRefs[i] = netguardv1beta1.AddressGroupReference{
			Ref:    domainRef.Ref,
			Source: netguardv1beta1.AddressGroupRegistrationSource(domainRef.Source),
		}
	}
	return k8sAGRefs
}
