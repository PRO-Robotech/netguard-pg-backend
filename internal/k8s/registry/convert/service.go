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

	"k8s.io/klog/v2"
)

// ServiceConverter implements conversion between k8s Service and domain Service
type ServiceConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.Service, *models.Service] = &ServiceConverter{}

// ToDomain converts a Kubernetes Service object to a domain Service model
func (c *ServiceConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.Service) (*models.Service, error) {
	if k8sObj == nil {
		return nil, fmt.Errorf("k8s Service object is nil")
	}

	// Create domain service
	domainService := &models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		Description: k8sObj.Spec.Description,
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
		domainService.Meta.Labels = make(map[string]string)
		for k, v := range k8sObj.Labels {
			domainService.Meta.Labels[k] = v
		}
	}

	if k8sObj.Annotations != nil {
		domainService.Meta.Annotations = make(map[string]string)
		for k, v := range k8sObj.Annotations {
			domainService.Meta.Annotations[k] = v
		}
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

	return domainService, nil
}

// FromDomain converts a domain Service model to a Kubernetes Service object
func (c *ServiceConverter) FromDomain(ctx context.Context, domainObj *models.Service) (*netguardv1beta1.Service, error) {
	if domainObj == nil {
		return nil, fmt.Errorf("domain Service object is nil")
	}

	// 🔍 ДИАГНОСТИКА: логируем что приходит на вход конвертера
	klog.Infof("🔄 ServiceConverter.FromDomain: converting service %s with %d conditions", domainObj.Key(), len(domainObj.Meta.Conditions))
	for i, cond := range domainObj.Meta.Conditions {
		klog.Infof("  🔍 CONV_IN[%d]: Type=%s, Status=%s, Reason=%s", i, cond.Type, cond.Status, cond.Reason)
	}

	// Create k8s service
	k8sService := &netguardv1beta1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              domainObj.ResourceIdentifier.Name,
			Namespace:         domainObj.ResourceIdentifier.Namespace,
			UID:               types.UID(domainObj.Meta.UID),
			ResourceVersion:   domainObj.Meta.ResourceVersion,
			Generation:        domainObj.Meta.Generation,
			CreationTimestamp: domainObj.Meta.CreationTS,
		},
		Spec: netguardv1beta1.ServiceSpec{
			Description: domainObj.Description,
		},
	}

	// Copy metadata
	if domainObj.Meta.Labels != nil {
		k8sService.Labels = make(map[string]string)
		for k, v := range domainObj.Meta.Labels {
			k8sService.Labels[k] = v
		}
	}

	if domainObj.Meta.Annotations != nil {
		k8sService.Annotations = make(map[string]string)
		for k, v := range domainObj.Meta.Annotations {
			k8sService.Annotations[k] = v
		}
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

	// Convert status - переносим условия из Meta в Status
	k8sService.Status = netguardv1beta1.ServiceStatus{
		ObservedGeneration: domainObj.Meta.ObservedGeneration,
		Conditions:         domainObj.Meta.Conditions, // Backend формирует условия
	}

	// 🔍 ДИАГНОСТИКА: логируем что отдаем на выход
	klog.Infof("✅ ServiceConverter.FromDomain: converted service %s with %d conditions in status", k8sService.Name, len(k8sService.Status.Conditions))
	for i, cond := range k8sService.Status.Conditions {
		klog.Infof("  ✅ CONV_OUT[%d]: Type=%s, Status=%s, Reason=%s", i, cond.Type, cond.Status, cond.Reason)
	}

	return k8sService, nil
}

// ToList converts a slice of domain Service models to a Kubernetes ServiceList object
func (c *ServiceConverter) ToList(ctx context.Context, domainObjs []*models.Service) (runtime.Object, error) {
	serviceList := &netguardv1beta1.ServiceList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "ServiceList",
		},
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
