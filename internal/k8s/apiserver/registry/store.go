package registry

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/utils"
)

// ServiceREST implements REST storage for Service
type ServiceREST struct {
	services map[string]*v1beta1.Service
}

func NewServiceREST() *ServiceREST {
	return &ServiceREST{
		services: make(map[string]*v1beta1.Service),
	}
}

func (r *ServiceREST) New() runtime.Object {
	return &v1beta1.Service{}
}

func (r *ServiceREST) NewList() runtime.Object {
	return &v1beta1.ServiceList{}
}

func (r *ServiceREST) Destroy() {
	// Cleanup resources if needed
}

func (r *ServiceREST) NamespaceScoped() bool {
	return true
}

func (r *ServiceREST) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	service := obj.(*v1beta1.Service)
	if service.Name == "" {
		return nil, errors.NewBadRequest("name is required")
	}

	key := fmt.Sprintf("%s/%s", service.Namespace, service.Name)
	r.services[key] = service.DeepCopy()

	return service, nil
}

func (r *ServiceREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Для простоты используем default namespace
	key := fmt.Sprintf("default/%s", name)
	service, exists := r.services[key]
	if !exists {
		return nil, errors.NewNotFound(v1beta1.Resource("services"), name)
	}
	return service, nil
}

func (r *ServiceREST) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	list := &v1beta1.ServiceList{}
	for _, service := range r.services {
		list.Items = append(list.Items, *service)
	}

	// Apply sorting (по умолчанию namespace + name, или через sortBy параметр)
	sortBy := utils.ExtractSortByFromContext(ctx)
	err := utils.ApplySorting(list.Items, sortBy,
		// idFn для извлечения ResourceIdentifier
		func(item v1beta1.Service) models.ResourceIdentifier {
			return models.ResourceIdentifier{
				Name:      item.ObjectMeta.Name,
				Namespace: item.ObjectMeta.Namespace,
			}
		},
		// k8sObjectFn для конвертации в Kubernetes объект
		func(item v1beta1.Service) runtime.Object {
			return &item
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sort services: %w", err)
	}

	return list, nil
}

func (r *ServiceREST) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	key := fmt.Sprintf("default/%s", name)
	service, exists := r.services[key]
	if !exists {
		return nil, false, errors.NewNotFound(v1beta1.Resource("services"), name)
	}
	delete(r.services, key)
	return service, true, nil
}

func (r *ServiceREST) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	// Простая реализация table conversion
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Age", Type: "date"},
		},
	}
	return table, nil
}

// Временные заглушки для остальных типов - используем интерфейс rest.Storage
func NewAddressGroupREST() rest.Storage {
	return &ServiceREST{services: make(map[string]*v1beta1.Service)}
}

func NewAddressGroupBindingREST() rest.Storage {
	return &ServiceREST{services: make(map[string]*v1beta1.Service)}
}

func NewAddressGroupPortMappingREST() rest.Storage {
	return &ServiceREST{services: make(map[string]*v1beta1.Service)}
}

func NewRuleS2SREST() rest.Storage {
	return &ServiceREST{services: make(map[string]*v1beta1.Service)}
}

func NewServiceAliasREST() rest.Storage {
	return &ServiceREST{services: make(map[string]*v1beta1.Service)}
}

func NewAddressGroupBindingPolicyREST() rest.Storage {
	return &ServiceREST{services: make(map[string]*v1beta1.Service)}
}

func NewIEAgAgRuleREST() rest.Storage {
	return &ServiceREST{services: make(map[string]*v1beta1.Service)}
}
