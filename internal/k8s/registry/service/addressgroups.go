package service

import (
	"context"
	"fmt"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/utils"
)

// AddressGroupsREST implements the addressGroups subresource for Service
type AddressGroupsREST struct {
	backendClient client.BackendClient
}

// NewAddressGroupsREST creates a new addressGroups subresource handler
func NewAddressGroupsREST(backendClient client.BackendClient) *AddressGroupsREST {
	return &AddressGroupsREST{
		backendClient: backendClient,
	}
}

// Compile-time interface assertions
var _ rest.Getter = &AddressGroupsREST{}
var _ rest.Lister = &AddressGroupsREST{}
var _ rest.TableConvertor = &AddressGroupsREST{}

// New returns a new AddressGroupsSpec object
func (r *AddressGroupsREST) New() runtime.Object {
	return &netguardv1beta1.AddressGroupsSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupsSpec",
		},
	}
}

// NewList returns a new AddressGroupsSpecList object
func (r *AddressGroupsREST) NewList() runtime.Object {
	return &netguardv1beta1.AddressGroupsSpecList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupsSpecList",
		},
	}
}

// Destroy cleans up resources
func (r *AddressGroupsREST) Destroy() {}

// Get retrieves the addressGroups for a specific Service
func (r *AddressGroupsREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace := utils.NamespaceFrom(ctx)

	// Get the Service from backend
	serviceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	service, err := r.backendClient.GetService(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	// Get address groups for this service
	addressGroupsSpec, err := r.getAddressGroupsForService(ctx, service)
	if err != nil {
		return nil, err
	}

	// Set metadata for identification
	addressGroupsSpec.ObjectMeta = metav1.ObjectMeta{
		Name:      service.Name,
		Namespace: service.Namespace,
	}

	return addressGroupsSpec, nil
}

// List retrieves addressGroups for all Services in the namespace
func (r *AddressGroupsREST) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	scope := utils.ScopeFromContext(ctx)

	// Get all Services in scope
	services, err := r.backendClient.ListServices(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Build list response
	addressGroupsSpecList := &netguardv1beta1.AddressGroupsSpecList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupsSpecList",
		},
		Items: []netguardv1beta1.AddressGroupsSpec{},
	}

	// Get address groups for each service
	for _, service := range services {
		addressGroupsSpec, err := r.getAddressGroupsForService(ctx, &service)
		if err != nil {
			// Log error but continue processing other services
			continue
		}

		// Set service name in metadata for identification
		addressGroupsSpec.ObjectMeta = metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
		}

		addressGroupsSpecList.Items = append(addressGroupsSpecList.Items, *addressGroupsSpec)
	}

	// Apply sorting (по умолчанию namespace + name, или через sortBy параметр)
	sortBy := utils.ExtractSortByFromContext(ctx)
	err = utils.ApplySorting(addressGroupsSpecList.Items, sortBy,
		// idFn для извлечения ResourceIdentifier
		func(item netguardv1beta1.AddressGroupsSpec) models.ResourceIdentifier {
			return models.ResourceIdentifier{
				Name:      item.ObjectMeta.Name,
				Namespace: item.ObjectMeta.Namespace,
			}
		},
		// k8sObjectFn для конвертации в Kubernetes объект
		func(item netguardv1beta1.AddressGroupsSpec) runtime.Object {
			return &item
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sort address groups: %w", err)
	}

	return addressGroupsSpecList, nil
}

// ConvertToTable converts objects to tabular format for kubectl
func (r *AddressGroupsREST) ConvertToTable(ctx context.Context, obj runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Service", Type: "string", Format: "name", Description: "Service name"},
			{Name: "Namespace", Type: "string", Description: "Service namespace"},
			{Name: "AddressGroups", Type: "integer", Description: "Number of address groups"},
		},
	}

	switch t := obj.(type) {
	case *netguardv1beta1.AddressGroupsSpec:
		table.Rows = []metav1.TableRow{
			{
				Cells: []interface{}{
					t.ObjectMeta.Name,
					t.ObjectMeta.Namespace,
					len(t.Items),
				},
				Object: runtime.RawExtension{Object: t},
			},
		}
	case *netguardv1beta1.AddressGroupsSpecList:
		for _, item := range t.Items {
			table.Rows = append(table.Rows, metav1.TableRow{
				Cells: []interface{}{
					item.ObjectMeta.Name,
					item.ObjectMeta.Namespace,
					len(item.Items),
				},
				Object: runtime.RawExtension{Object: &item},
			})
		}
	default:
		return nil, fmt.Errorf("unsupported object type: %T", obj)
	}

	return table, nil
}

// getAddressGroupsForService is a helper function to get address groups for a service
func (r *AddressGroupsREST) getAddressGroupsForService(ctx context.Context, service *models.Service) (*netguardv1beta1.AddressGroupsSpec, error) {
	// Get all AddressGroupBindings that reference this Service
	scope := utils.ScopeFromContext(ctx)
	bindings, err := r.backendClient.ListAddressGroupBindings(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list address group bindings: %w", err)
	}

	// Build AddressGroupsSpec from bindings
	addressGroupsSpec := &netguardv1beta1.AddressGroupsSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupsSpec",
		},
		Items: []netguardv1beta1.NamespacedObjectReference{},
	}

	// Collect unique address groups referenced by this service
	addressGroupMap := make(map[string]netguardv1beta1.NamespacedObjectReference)

	for _, binding := range bindings {
		// Check if this binding references our service
		if binding.ServiceRef.Name == service.Name && binding.ServiceRef.Namespace == service.Namespace {
			ref := netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       binding.AddressGroupRef.Name,
				},
				Namespace: binding.AddressGroupRef.Namespace,
			}

			// Use key to deduplicate
			key := fmt.Sprintf("%s/%s", ref.Namespace, ref.Name)
			addressGroupMap[key] = ref
		}
	}

	// Convert map to slice
	for _, ref := range addressGroupMap {
		addressGroupsSpec.Items = append(addressGroupsSpec.Items, ref)
	}

	return addressGroupsSpec, nil
}
