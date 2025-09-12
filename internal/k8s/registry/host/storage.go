package host

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/base"
	"netguard-pg-backend/internal/k8s/registry/convert"
	"netguard-pg-backend/internal/k8s/registry/validation"

	"k8s.io/apiserver/pkg/registry/rest"
)

// HostConverterAdapter adapts HostConverter to BaseStorage interface
type HostConverterAdapter struct {
	*convert.HostConverter
}

func NewHostConverterAdapter() *HostConverterAdapter {
	return &HostConverterAdapter{
		HostConverter: &convert.HostConverter{},
	}
}

func (a *HostConverterAdapter) ToList(ctx context.Context, domainObjs []*models.Host) (runtime.Object, error) {
	if domainObjs == nil {
		return &netguardv1beta1.HostList{}, nil
	}

	k8sObjs := make([]netguardv1beta1.Host, 0, len(domainObjs))
	for _, domainObj := range domainObjs {
		if domainObj == nil {
			continue
		}
		k8sObj, err := a.HostConverter.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain Host to k8s: %w", err)
		}
		k8sObjs = append(k8sObjs, *k8sObj)
	}

	return &netguardv1beta1.HostList{
		Items: k8sObjs,
	}, nil
}

// HostStorage implements REST storage for Host resources using BaseStorage
type HostStorage struct {
	*base.BaseStorage[*netguardv1beta1.Host, *models.Host]
}

// NewHostStorage creates a new HostStorage using BaseStorage
func NewHostStorage(backendClient client.BackendClient) *HostStorage {
	converter := NewHostConverterAdapter()
	validator := &validation.HostValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use factory to create backend operations adapter
	backendOps := base.NewHostPtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.Host, *models.Host](
		func() *netguardv1beta1.Host { return &netguardv1beta1.Host{} },
		func() runtime.Object { return &netguardv1beta1.HostList{} },
		backendOps,
		converter,
		validator,
		watcher,
		"hosts",
		"Host",
		true, // namespace scoped
	)

	storage := &HostStorage{
		BaseStorage: baseStorage,
	}

	return storage
}

// handleHostCreate implements custom logic when a Host is created
func (s *HostStorage) handleHostCreate(ctx context.Context, obj *netguardv1beta1.Host, domainObj *models.Host) error {

	// Initialize Host as unbound initially
	obj.Status.IsBound = false
	obj.Status.BindingRef = nil
	obj.Status.AddressGroupRef = nil
	obj.Status.AddressGroupName = ""

	return nil
}

// handleHostUpdate implements custom logic when a Host is updated
func (s *HostStorage) handleHostUpdate(ctx context.Context, obj, oldObj *netguardv1beta1.Host, domainObj *models.Host) error {

	return nil
}

// handleHostDelete implements custom logic when a Host is deleted
func (s *HostStorage) handleHostDelete(ctx context.Context, obj *netguardv1beta1.Host, domainObj *models.Host) error {

	return nil
}

// GetSingularName returns the singular name for this resource
func (s *HostStorage) GetSingularName() string {
	return "host"
}

// DeleteCollection implements rest.CollectionDeleter
func (s *HostStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	return &netguardv1beta1.HostList{}, nil
}

// Kind implements rest.KindProvider
func (s *HostStorage) Kind() string {
	return "Host"
}

// Ensure HostStorage implements the required interfaces
var _ rest.StandardStorage = &HostStorage{}
var _ rest.KindProvider = &HostStorage{}
var _ rest.SingularNameProvider = &HostStorage{}
