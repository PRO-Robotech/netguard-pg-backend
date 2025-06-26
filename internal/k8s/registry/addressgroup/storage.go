package addressgroup

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

// AddressGroupStorage implements REST storage for AddressGroup resources
type AddressGroupStorage struct {
	backendClient client.BackendClient
}

// NewAddressGroupStorage creates a new AddressGroup storage
func NewAddressGroupStorage(backendClient client.BackendClient) *AddressGroupStorage {
	return &AddressGroupStorage{
		backendClient: backendClient,
	}
}

// New returns an empty AddressGroup object
func (s *AddressGroupStorage) New() runtime.Object {
	return &netguardv1beta1.AddressGroup{}
}

// NewList returns an empty AddressGroupList object
func (s *AddressGroupStorage) NewList() runtime.Object {
	return &netguardv1beta1.AddressGroupList{}
}

// NamespaceScoped returns true as AddressGroups are namespaced
func (s *AddressGroupStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name for the resource
func (s *AddressGroupStorage) GetSingularName() string {
	return "addressgroup"
}

// Destroy cleans up resources on shutdown
func (s *AddressGroupStorage) Destroy() {
	// Nothing to clean up for now
}

// Get retrieves an AddressGroup by name from backend (READ-ONLY, no status changes)
func (s *AddressGroupStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Extract namespace from context
	namespace, ok := ctx.Value("namespace").(string)
	if !ok || namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	// Get from backend
	resourceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	addressGroup, err := s.backendClient.GetAddressGroup(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get AddressGroup %s/%s: %w", namespace, name, err)
	}

	// Convert to Kubernetes format
	k8sAddressGroup := convertAddressGroupToK8s(*addressGroup)
	return k8sAddressGroup, nil
}

// List retrieves AddressGroups from backend with filtering (READ-ONLY, no status changes)
func (s *AddressGroupStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	// Create scope for filtering
	var scope ports.Scope
	if options != nil && options.FieldSelector != nil {
		// Extract namespace from field selector if present
		// For now, implement basic namespace filtering
		scope = ports.NewResourceIdentifierScope()
	}

	// Get from backend
	addressGroups, err := s.backendClient.ListAddressGroups(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list AddressGroups: %w", err)
	}

	// Convert to Kubernetes format
	k8sAddressGroupList := &netguardv1beta1.AddressGroupList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "AddressGroupList",
		},
	}

	for _, addressGroup := range addressGroups {
		k8sAddressGroup := convertAddressGroupToK8s(addressGroup)
		k8sAddressGroupList.Items = append(k8sAddressGroupList.Items, *k8sAddressGroup)
	}

	return k8sAddressGroupList, nil
}

// Create creates a new AddressGroup in backend via Sync API
func (s *AddressGroupStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	k8sAddressGroup, ok := obj.(*netguardv1beta1.AddressGroup)
	if !ok {
		return nil, fmt.Errorf("expected AddressGroup, got %T", obj)
	}

	// Run validation if provided
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	// Convert to backend model
	addressGroup := convertAddressGroupFromK8s(k8sAddressGroup)

	// Create via Sync API
	addressGroups := []models.AddressGroup{addressGroup}
	err := s.backendClient.Sync(ctx, models.SyncOpUpsert, addressGroups)
	if err != nil {
		return nil, fmt.Errorf("failed to create AddressGroup: %w", err)
	}

	// Set successful status
	setCondition(k8sAddressGroup, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "AddressGroup successfully created in backend")

	return k8sAddressGroup, nil
}

// Update updates an existing AddressGroup in backend via Sync API
func (s *AddressGroupStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
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

	k8sAddressGroup, ok := updatedObj.(*netguardv1beta1.AddressGroup)
	if !ok {
		return nil, false, fmt.Errorf("expected AddressGroup, got %T", updatedObj)
	}

	// Run validation if provided
	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, currentObj); err != nil {
			return nil, false, err
		}
	}

	// Convert to backend model
	addressGroup := convertAddressGroupFromK8s(k8sAddressGroup)

	// Update via Sync API
	addressGroups := []models.AddressGroup{addressGroup}
	err = s.backendClient.Sync(ctx, models.SyncOpUpsert, addressGroups)
	if err != nil {
		return nil, false, fmt.Errorf("failed to update AddressGroup: %w", err)
	}

	// Set successful status
	setCondition(k8sAddressGroup, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "AddressGroup successfully updated in backend")

	return k8sAddressGroup, false, nil
}

// Delete removes an AddressGroup from backend via Sync API
func (s *AddressGroupStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
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

	k8sAddressGroup, ok := obj.(*netguardv1beta1.AddressGroup)
	if !ok {
		return nil, false, fmt.Errorf("expected AddressGroup, got %T", obj)
	}

	// Convert to backend model
	addressGroup := convertAddressGroupFromK8s(k8sAddressGroup)

	// Delete via Sync API
	addressGroups := []models.AddressGroup{addressGroup}
	err = s.backendClient.Sync(ctx, models.SyncOpDelete, addressGroups)
	if err != nil {
		return nil, false, fmt.Errorf("failed to delete AddressGroup: %w", err)
	}

	return k8sAddressGroup, true, nil
}

// Watch implements watch functionality for AddressGroups
func (s *AddressGroupStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	pollerManager := watchpkg.GetPollerManager(s.backendClient)
	poller := pollerManager.GetPoller("addressgroups")

	client, err := poller.AddClient(options)
	if err != nil {
		return nil, fmt.Errorf("failed to add watch client: %w", err)
	}

	return watchpkg.NewPollerWatchInterface(client, poller), nil
}

// GetSupportedVerbs returns the supported verbs for this storage
func (s *AddressGroupStorage) GetSupportedVerbs() []string {
	return []string{"get", "list", "create", "update", "delete", "watch"}
}

// Helper functions for conversion

func convertAddressGroupToK8s(addressGroup models.AddressGroup) *netguardv1beta1.AddressGroup {
	k8sAddressGroup := &netguardv1beta1.AddressGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "AddressGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      addressGroup.ResourceIdentifier.Name,
			Namespace: addressGroup.ResourceIdentifier.Namespace,
		},
		Spec: netguardv1beta1.AddressGroupSpec{
			Description: addressGroup.Description,
			Addresses:   convertStringArrayToAddresses(addressGroup.Addresses),
		},
	}

	return k8sAddressGroup
}

func convertAddressGroupFromK8s(k8sAddressGroup *netguardv1beta1.AddressGroup) models.AddressGroup {
	addressGroup := models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sAddressGroup.Name,
				models.WithNamespace(k8sAddressGroup.Namespace),
			),
		},
		Description: k8sAddressGroup.Spec.Description,
		Addresses:   convertAddressesToStringArray(k8sAddressGroup.Spec.Addresses),
	}

	return addressGroup
}

// Helper functions for Address conversion
func convertStringArrayToAddresses(addresses []string) []netguardv1beta1.Address {
	result := make([]netguardv1beta1.Address, 0, len(addresses))
	for _, addr := range addresses {
		result = append(result, netguardv1beta1.Address{
			Address: addr,
		})
	}
	return result
}

func convertAddressesToStringArray(addresses []netguardv1beta1.Address) []string {
	result := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		result = append(result, addr.Address)
	}
	return result
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
