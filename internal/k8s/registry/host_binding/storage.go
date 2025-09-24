package host_binding

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/base"
	"netguard-pg-backend/internal/k8s/registry/convert"
	"netguard-pg-backend/internal/k8s/registry/validation"

	"k8s.io/apiserver/pkg/registry/rest"
)

// HostBindingConverterAdapter adapts HostBindingConverter to BaseStorage interface
type HostBindingConverterAdapter struct {
	*convert.HostBindingConverter
}

func NewHostBindingConverterAdapter() *HostBindingConverterAdapter {
	return &HostBindingConverterAdapter{
		HostBindingConverter: &convert.HostBindingConverter{},
	}
}

func (a *HostBindingConverterAdapter) ToList(ctx context.Context, domainObjs []*models.HostBinding) (runtime.Object, error) {
	if domainObjs == nil {
		return &netguardv1beta1.HostBindingList{}, nil
	}

	k8sObjs := make([]netguardv1beta1.HostBinding, 0, len(domainObjs))
	for _, domainObj := range domainObjs {
		if domainObj == nil {
			continue
		}
		k8sObj, err := a.HostBindingConverter.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain HostBinding to k8s: %w", err)
		}
		k8sObjs = append(k8sObjs, *k8sObj)
	}

	return &netguardv1beta1.HostBindingList{
		Items: k8sObjs,
	}, nil
}

// HostBindingStorage implements REST storage for HostBinding resources using BaseStorage
type HostBindingStorage struct {
	*base.BaseStorage[*netguardv1beta1.HostBinding, *models.HostBinding]
	backendClient client.BackendClient // Direct access to backend for Host operations
}

// NewHostBindingStorage creates a new HostBindingStorage using BaseStorage
func NewHostBindingStorage(backendClient client.BackendClient) *HostBindingStorage {
	converter := NewHostBindingConverterAdapter()
	validator := &validation.HostBindingValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	backendOps := base.NewHostBindingPtrOps(backendClient)
	baseStorage := base.NewBaseStorage[*netguardv1beta1.HostBinding, *models.HostBinding](
		func() *netguardv1beta1.HostBinding { return &netguardv1beta1.HostBinding{} },
		func() runtime.Object { return &netguardv1beta1.HostBindingList{} },
		backendOps,
		converter,
		validator,
		watcher,
		"hostbindings",
		"HostBinding",
		true, // namespace scoped
	)

	storage := &HostBindingStorage{
		BaseStorage:   baseStorage,
		backendClient: backendClient,
	}

	return storage
}

func (s *HostBindingStorage) handleHostBindingCreate(ctx context.Context, obj *netguardv1beta1.HostBinding, domainObj *models.HostBinding) error {
	// Update Host status to reflect the binding
	if err := s.updateHostBindingStatus(ctx, domainObj, true); err != nil {
		return fmt.Errorf("failed to update host status on binding create: %w", err)
	}

	return nil
}

func (s *HostBindingStorage) handleHostBindingUpdate(ctx context.Context, obj, oldObj *netguardv1beta1.HostBinding, domainObj *models.HostBinding) error {
	// Update Host status to reflect any changes in the binding
	if err := s.updateHostBindingStatus(ctx, domainObj, true); err != nil {
		return fmt.Errorf("failed to update host status on binding update: %w", err)
	}

	return nil
}

func (s *HostBindingStorage) handleHostBindingDelete(ctx context.Context, obj *netguardv1beta1.HostBinding, domainObj *models.HostBinding) error {
	if err := s.updateHostBindingStatus(ctx, domainObj, false); err != nil {
		return fmt.Errorf("failed to clear host status on binding delete: %w", err)
	}

	return nil
}

// GetSingularName returns the singular name for this resource
func (s *HostBindingStorage) GetSingularName() string {
	return "hostbinding"
}

// DeleteCollection implements rest.CollectionDeleter
func (s *HostBindingStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	return &netguardv1beta1.HostBindingList{}, nil
}

// updateHostBindingStatus updates the Host resource status based on HostBinding changes
func (s *HostBindingStorage) updateHostBindingStatus(ctx context.Context, hostBinding *models.HostBinding, isBound bool) error {
	// Get the Host resource
	hostID := models.ResourceIdentifier{
		Name:      hostBinding.HostRef.Name,
		Namespace: hostBinding.HostRef.Namespace,
	}

	host, err := s.backendClient.GetHost(ctx, hostID)
	if err != nil {
		if err == ports.ErrNotFound {
			return fmt.Errorf("referenced host %s/%s not found", hostID.Namespace, hostID.Name)
		}
		return fmt.Errorf("failed to get host %s/%s: %w", hostID.Namespace, hostID.Name, err)
	}

	// Update Host status based on binding state
	if isBound {
		// Set binding information
		host.IsBound = true
		host.AddressGroupName = hostBinding.AddressGroupRef.Name
		host.BindingRef = &netguardv1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "HostBinding",
			Name:       hostBinding.Name,
		}
		host.AddressGroupRef = &netguardv1beta1.ObjectReference{
			APIVersion: hostBinding.AddressGroupRef.APIVersion,
			Kind:       hostBinding.AddressGroupRef.Kind,
			Name:       hostBinding.AddressGroupRef.Name,
		}
	} else {
		host.IsBound = false
		host.AddressGroupName = ""
		host.BindingRef = nil
		host.AddressGroupRef = nil
	}

	if err := s.backendClient.UpdateHost(ctx, host); err != nil {
		return fmt.Errorf("failed to update host status: %w", err)
	}

	return nil
}

// Kind implements rest.KindProvider
func (s *HostBindingStorage) Kind() string {
	return "HostBinding"
}

// Ensure HostBindingStorage implements the required interfaces
var _ rest.StandardStorage = &HostBindingStorage{}
var _ rest.KindProvider = &HostBindingStorage{}
var _ rest.SingularNameProvider = &HostBindingStorage{}
