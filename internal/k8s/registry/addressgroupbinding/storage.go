package addressgroupbinding

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

// AddressGroupBindingStorage implements REST storage for AddressGroupBinding resources
type AddressGroupBindingStorage struct {
	backendClient client.BackendClient
}

// NewAddressGroupBindingStorage creates a new AddressGroupBinding storage
func NewAddressGroupBindingStorage(backendClient client.BackendClient) *AddressGroupBindingStorage {
	return &AddressGroupBindingStorage{
		backendClient: backendClient,
	}
}

// New returns an empty AddressGroupBinding object
func (s *AddressGroupBindingStorage) New() runtime.Object {
	return &netguardv1beta1.AddressGroupBinding{}
}

// NewList returns an empty AddressGroupBindingList object
func (s *AddressGroupBindingStorage) NewList() runtime.Object {
	return &netguardv1beta1.AddressGroupBindingList{}
}

// NamespaceScoped returns true as AddressGroupBindings are namespaced
func (s *AddressGroupBindingStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name for the resource
func (s *AddressGroupBindingStorage) GetSingularName() string {
	return "addressgroupbinding"
}

// Destroy cleans up resources on shutdown
func (s *AddressGroupBindingStorage) Destroy() {
	// Nothing to clean up for now
}

// Get retrieves an AddressGroupBinding by name from backend (READ-ONLY, no status changes)
func (s *AddressGroupBindingStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Extract namespace from context
	namespace, ok := ctx.Value("namespace").(string)
	if !ok || namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	// Get from backend
	resourceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	binding, err := s.backendClient.GetAddressGroupBinding(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get AddressGroupBinding %s/%s: %w", namespace, name, err)
	}

	// Convert to Kubernetes format
	k8sBinding := convertAddressGroupBindingToK8s(*binding)
	return k8sBinding, nil
}

// List retrieves AddressGroupBindings from backend with filtering (READ-ONLY, no status changes)
func (s *AddressGroupBindingStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	// Create scope for filtering
	var scope ports.Scope
	if options != nil && options.FieldSelector != nil {
		// Extract namespace from field selector if present
		// For now, implement basic namespace filtering
		scope = ports.NewResourceIdentifierScope()
	}

	// Get from backend
	bindings, err := s.backendClient.ListAddressGroupBindings(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list AddressGroupBindings: %w", err)
	}

	// Convert to Kubernetes format
	k8sBindingList := &netguardv1beta1.AddressGroupBindingList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "AddressGroupBindingList",
		},
	}

	for _, binding := range bindings {
		k8sBinding := convertAddressGroupBindingToK8s(binding)
		k8sBindingList.Items = append(k8sBindingList.Items, *k8sBinding)
	}

	return k8sBindingList, nil
}

// Create creates a new AddressGroupBinding in backend via Sync API
func (s *AddressGroupBindingStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	k8sBinding, ok := obj.(*netguardv1beta1.AddressGroupBinding)
	if !ok {
		return nil, fmt.Errorf("expected AddressGroupBinding, got %T", obj)
	}

	// Run validation if provided
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	// Convert to backend model
	binding := convertAddressGroupBindingFromK8s(k8sBinding)

	// Create via Sync API
	bindings := []models.AddressGroupBinding{binding}
	err := s.backendClient.Sync(ctx, models.SyncOpUpsert, bindings)
	if err != nil {
		return nil, fmt.Errorf("failed to create AddressGroupBinding: %w", err)
	}

	// Set successful status
	setCondition(k8sBinding, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "AddressGroupBinding successfully created in backend")

	return k8sBinding, nil
}

// Update updates an existing AddressGroupBinding in backend via Sync API
func (s *AddressGroupBindingStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// Get current object
	currentObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		if forceAllowCreate {
			// Convert to create operation
			newObj, err := objInfo.UpdatedObject(ctx, nil)
			if err != nil {
				return nil, false, err
			}
			createdObj, err := s.Create(ctx, newObj, createValidation, &metav1.CreateOptions{})
			return createdObj, true, err
		}
		return nil, false, err
	}

	// Get updated object
	updatedObj, err := objInfo.UpdatedObject(ctx, currentObj)
	if err != nil {
		return nil, false, err
	}

	k8sBinding, ok := updatedObj.(*netguardv1beta1.AddressGroupBinding)
	if !ok {
		return nil, false, fmt.Errorf("expected AddressGroupBinding, got %T", updatedObj)
	}

	// Run validation if provided
	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, currentObj); err != nil {
			return nil, false, err
		}
	}

	// Convert to backend model
	binding := convertAddressGroupBindingFromK8s(k8sBinding)

	// Update via Sync API
	bindings := []models.AddressGroupBinding{binding}
	err = s.backendClient.Sync(ctx, models.SyncOpUpsert, bindings)
	if err != nil {
		return nil, false, fmt.Errorf("failed to update AddressGroupBinding: %w", err)
	}

	// Set successful status
	setCondition(k8sBinding, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "AddressGroupBinding successfully updated in backend")

	return k8sBinding, false, nil
}

// Delete removes an AddressGroupBinding from backend via Sync API
func (s *AddressGroupBindingStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	// Get current object
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

	k8sBinding, ok := obj.(*netguardv1beta1.AddressGroupBinding)
	if !ok {
		return nil, false, fmt.Errorf("expected AddressGroupBinding, got %T", obj)
	}

	// Convert to backend model
	binding := convertAddressGroupBindingFromK8s(k8sBinding)

	// Delete via Sync API
	bindings := []models.AddressGroupBinding{binding}
	err = s.backendClient.Sync(ctx, models.SyncOpDelete, bindings)
	if err != nil {
		return nil, false, fmt.Errorf("failed to delete AddressGroupBinding: %w", err)
	}

	return k8sBinding, true, nil
}

// Watch implements watch functionality for AddressGroupBindings
func (s *AddressGroupBindingStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	pollerManager := watchpkg.GetPollerManager(s.backendClient)
	poller := pollerManager.GetPoller("addressgroupbindings")

	client, err := poller.AddClient(options)
	if err != nil {
		return nil, fmt.Errorf("failed to add watch client: %w", err)
	}

	return watchpkg.NewPollerWatchInterface(client, poller), nil
}

// GetSupportedVerbs returns the supported verbs for this storage
func (s *AddressGroupBindingStorage) GetSupportedVerbs() []string {
	return []string{"get", "list", "create", "update", "delete", "watch"}
}

// Helper functions for conversion

func convertAddressGroupBindingToK8s(binding models.AddressGroupBinding) *netguardv1beta1.AddressGroupBinding {
	k8sBinding := &netguardv1beta1.AddressGroupBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "AddressGroupBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      binding.ResourceIdentifier.Name,
			Namespace: binding.ResourceIdentifier.Namespace,
		},
		Spec: netguardv1beta1.AddressGroupBindingSpec{
			ServiceRef: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       binding.ServiceRef.Name,
			},
			AddressGroupRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       binding.AddressGroupRef.Name,
				},
				Namespace: binding.AddressGroupRef.Namespace,
			},
		},
	}

	return k8sBinding
}

func convertAddressGroupBindingFromK8s(k8sBinding *netguardv1beta1.AddressGroupBinding) models.AddressGroupBinding {
	binding := models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sBinding.Name,
				models.WithNamespace(k8sBinding.Namespace),
			),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sBinding.Spec.ServiceRef.Name,
				models.WithNamespace(k8sBinding.Namespace), // Service is in same namespace
			),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sBinding.Spec.AddressGroupRef.Name,
				models.WithNamespace(k8sBinding.Spec.AddressGroupRef.Namespace),
			),
		},
	}

	return binding
}

// Status condition helpers (same as in Service)
const (
	ConditionReady = "Ready"

	ReasonBindingCreated       = "BindingCreated"
	ReasonServiceNotFound      = "ServiceNotFound"
	ReasonAddressGroupNotFound = "AddressGroupNotFound"
	ReasonSyncFailed           = "SyncFailed"
	ReasonDeletionFailed       = "DeletionFailed"
)

func setCondition(obj runtime.Object, conditionType string, status metav1.ConditionStatus, reason, message string) {
	// TODO: Implement proper condition setting
	// This should be moved to a shared helper
}
