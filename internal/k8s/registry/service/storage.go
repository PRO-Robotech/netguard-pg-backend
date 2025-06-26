package service

import (
	"context"
	"fmt"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	watchpkg "netguard-pg-backend/internal/k8s/registry/watch"
)

// ServiceStorage implements REST storage for Service resources
type ServiceStorage struct {
	backendClient client.BackendClient
}

// NewServiceStorage creates a new ServiceStorage
func NewServiceStorage(backendClient client.BackendClient) *ServiceStorage {
	return &ServiceStorage{
		backendClient: backendClient,
	}
}

// New returns an empty Service object
func (s *ServiceStorage) New() runtime.Object {
	return &netguardv1beta1.Service{}
}

// NewList returns an empty ServiceList object
func (s *ServiceStorage) NewList() runtime.Object {
	return &netguardv1beta1.ServiceList{}
}

// NamespaceScoped returns true since Service is namespaced
func (s *ServiceStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name of the resource
func (s *ServiceStorage) GetSingularName() string {
	return "service"
}

// Destroy cleans up resources on shutdown
func (s *ServiceStorage) Destroy() {
	// Nothing to clean up for now
}

// Get retrieves a Service by name from backend
func (s *ServiceStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Extract namespace from context
	namespace := ""
	if ns, ok := ctx.Value("namespace").(string); ok {
		namespace = ns
	}

	// Create resource identifier
	id := models.NewResourceIdentifier(name, models.WithNamespace(namespace))

	// Get from backend
	service, err := s.backendClient.GetService(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service %s not found: %w", name, err)
	}

	// Convert to K8s API format
	k8sService := convertServiceToK8s(*service)
	return &k8sService, nil
}

// List retrieves Services from backend with optional filtering
func (s *ServiceStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	// Extract namespace from context
	namespace := ""
	if ns, ok := ctx.Value("namespace").(string); ok {
		namespace = ns
	}

	// Create scope for backend query
	var scope ports.Scope
	if namespace != "" {
		// Namespace-scoped list
		id := models.NewResourceIdentifier("", models.WithNamespace(namespace))
		scope = ports.ResourceIdentifierScope{Identifiers: []models.ResourceIdentifier{id}}
	}
	// If namespace is empty, scope will be nil = list all

	// Get from backend
	services, err := s.backendClient.ListServices(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Convert to K8s API format
	serviceList := &netguardv1beta1.ServiceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceList",
			APIVersion: "netguard.sgroups.io/v1beta1",
		},
		Items: make([]netguardv1beta1.Service, 0, len(services)),
	}

	for _, service := range services {
		k8sService := convertServiceToK8s(service)
		serviceList.Items = append(serviceList.Items, k8sService)
	}

	return serviceList, nil
}

// Create creates a new Service in backend
func (s *ServiceStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	service, ok := obj.(*netguardv1beta1.Service)
	if !ok {
		return nil, fmt.Errorf("not a Service object")
	}

	// Run validation if provided
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	// Convert to backend format
	backendService := convertServiceFromK8s(*service)

	// Create in backend
	err := s.backendClient.CreateService(ctx, &backendService)
	if err != nil {
		return nil, fmt.Errorf("failed to create service in backend: %w", err)
	}

	// Set status to Ready=True (successful creation)
	setServiceCondition(service, "Ready", metav1.ConditionTrue, "ServiceCreated", "Service successfully created in backend")

	return service, nil
}

// Update updates an existing Service in backend
func (s *ServiceStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// Get current object
	currentObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		if !forceAllowCreate {
			return nil, false, err
		}
		// Create new object if not found and forceAllowCreate is true
		newObj, err := objInfo.UpdatedObject(ctx, nil)
		if err != nil {
			return nil, false, err
		}
		createdObj, err := s.Create(ctx, newObj, createValidation, &metav1.CreateOptions{})
		return createdObj, true, err
	}

	// Update object
	updatedObj, err := objInfo.UpdatedObject(ctx, currentObj)
	if err != nil {
		return nil, false, err
	}

	service, ok := updatedObj.(*netguardv1beta1.Service)
	if !ok {
		return nil, false, fmt.Errorf("not a Service object")
	}

	// Run validation if provided
	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, currentObj); err != nil {
			return nil, false, err
		}
	}

	// Convert to backend format
	backendService := convertServiceFromK8s(*service)

	// Update in backend
	err = s.backendClient.UpdateService(ctx, &backendService)
	if err != nil {
		return nil, false, fmt.Errorf("failed to update service in backend: %w", err)
	}

	// Set status to Ready=True (successful update)
	setServiceCondition(service, "Ready", metav1.ConditionTrue, "ServiceUpdated", "Service successfully updated in backend")

	return service, false, nil
}

// Delete removes a Service from backend
func (s *ServiceStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	// Get current object first
	obj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	// Run validation if provided
	if deleteValidation != nil {
		if err := deleteValidation(ctx, obj); err != nil {
			return nil, false, err
		}
	}

	// Extract namespace from context
	namespace := ""
	if ns, ok := ctx.Value("namespace").(string); ok {
		namespace = ns
	}

	// Create resource identifier
	id := models.NewResourceIdentifier(name, models.WithNamespace(namespace))

	// Delete from backend
	err = s.backendClient.DeleteService(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("failed to delete service from backend: %w", err)
	}

	return obj, true, nil
}

// Watch implements watch functionality using Shared Poller
func (s *ServiceStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	pollerManager := watchpkg.GetPollerManager(s.backendClient)
	poller := pollerManager.GetPoller("services")

	client, err := poller.AddClient(options)
	if err != nil {
		return nil, fmt.Errorf("failed to add watch client: %w", err)
	}

	return watchpkg.NewPollerWatchInterface(client, poller), nil
}

// Helper functions for conversion

func convertServiceToK8s(service models.Service) netguardv1beta1.Service {
	k8sService := netguardv1beta1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "netguard.sgroups.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.ResourceIdentifier.Name,
			Namespace: service.ResourceIdentifier.Namespace,
		},
		Spec: netguardv1beta1.ServiceSpec{
			Description: service.Description,
		},
	}

	// Convert IngressPorts - direct mapping since both use string ports
	for _, port := range service.IngressPorts {
		k8sPort := netguardv1beta1.IngressPort{
			Protocol:    netguardv1beta1.TransportProtocol(port.Protocol),
			Port:        port.Port, // Direct string mapping
			Description: port.Description,
		}
		k8sService.Spec.IngressPorts = append(k8sService.Spec.IngressPorts, k8sPort)
	}

	return k8sService
}

func convertServiceFromK8s(k8sService netguardv1beta1.Service) models.Service {
	service := models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sService.Name,
				models.WithNamespace(k8sService.Namespace),
			),
		},
		Description: k8sService.Spec.Description,
	}

	// Convert IngressPorts - direct mapping since both use string ports
	for _, port := range k8sService.Spec.IngressPorts {
		ingressPort := models.IngressPort{
			Protocol:    models.TransportProtocol(port.Protocol),
			Port:        port.Port, // Direct string mapping
			Description: port.Description,
		}
		service.IngressPorts = append(service.IngressPorts, ingressPort)
	}

	return service
}

func setServiceCondition(service *netguardv1beta1.Service, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
		ObservedGeneration: service.Generation,
	}

	// Find existing condition and update or append new one
	for i, existingCondition := range service.Status.Conditions {
		if existingCondition.Type == conditionType {
			service.Status.Conditions[i] = condition
			return
		}
	}

	service.Status.Conditions = append(service.Status.Conditions, condition)
}
