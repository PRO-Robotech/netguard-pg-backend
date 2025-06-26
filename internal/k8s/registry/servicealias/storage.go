package servicealias

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

// ServiceAliasStorage implements REST storage for ServiceAlias resources
type ServiceAliasStorage struct {
	backendClient client.BackendClient
}

// NewServiceAliasStorage creates a new ServiceAlias storage
func NewServiceAliasStorage(backendClient client.BackendClient) *ServiceAliasStorage {
	return &ServiceAliasStorage{
		backendClient: backendClient,
	}
}

// New returns an empty ServiceAlias object
func (s *ServiceAliasStorage) New() runtime.Object {
	return &netguardv1beta1.ServiceAlias{}
}

// NewList returns an empty ServiceAliasList object
func (s *ServiceAliasStorage) NewList() runtime.Object {
	return &netguardv1beta1.ServiceAliasList{}
}

// NamespaceScoped returns true as ServiceAliases are namespaced
func (s *ServiceAliasStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name for the resource
func (s *ServiceAliasStorage) GetSingularName() string {
	return "servicealias"
}

// Destroy cleans up resources on shutdown
func (s *ServiceAliasStorage) Destroy() {
	// Nothing to clean up for now
}

// Get retrieves a ServiceAlias by name from backend (READ-ONLY, no status changes)
func (s *ServiceAliasStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Extract namespace from context
	namespace, ok := ctx.Value("namespace").(string)
	if !ok || namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	// Get from backend
	resourceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	alias, err := s.backendClient.GetServiceAlias(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ServiceAlias %s/%s: %w", namespace, name, err)
	}

	// Convert to Kubernetes format
	k8sAlias := convertServiceAliasToK8s(*alias)
	return k8sAlias, nil
}

// List retrieves ServiceAliases from backend with filtering (READ-ONLY, no status changes)
func (s *ServiceAliasStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	// Create scope for filtering
	var scope ports.Scope
	if options != nil && options.FieldSelector != nil {
		// Extract namespace from field selector if present
		// For now, implement basic namespace filtering
		scope = ports.NewResourceIdentifierScope()
	}

	// Get from backend
	aliases, err := s.backendClient.ListServiceAliases(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list ServiceAliases: %w", err)
	}

	// Convert to Kubernetes format
	k8sAliasList := &netguardv1beta1.ServiceAliasList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "ServiceAliasList",
		},
	}

	for _, alias := range aliases {
		k8sAlias := convertServiceAliasToK8s(alias)
		k8sAliasList.Items = append(k8sAliasList.Items, *k8sAlias)
	}

	return k8sAliasList, nil
}

// Create creates a new ServiceAlias in backend via Sync API
func (s *ServiceAliasStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	k8sAlias, ok := obj.(*netguardv1beta1.ServiceAlias)
	if !ok {
		return nil, fmt.Errorf("expected ServiceAlias, got %T", obj)
	}

	// Run validation if provided
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	// Convert to backend model
	alias := convertServiceAliasFromK8s(k8sAlias)

	// Create via Sync API
	aliases := []models.ServiceAlias{alias}
	err := s.backendClient.Sync(ctx, models.SyncOpUpsert, aliases)
	if err != nil {
		return nil, fmt.Errorf("failed to create ServiceAlias: %w", err)
	}

	// Set successful status
	setCondition(k8sAlias, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "ServiceAlias successfully created in backend")

	return k8sAlias, nil
}

// Update updates an existing ServiceAlias in backend via Sync API
func (s *ServiceAliasStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
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

	k8sAlias, ok := updatedObj.(*netguardv1beta1.ServiceAlias)
	if !ok {
		return nil, false, fmt.Errorf("expected ServiceAlias, got %T", updatedObj)
	}

	// Run validation if provided
	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, currentObj); err != nil {
			return nil, false, err
		}
	}

	// Convert to backend model
	alias := convertServiceAliasFromK8s(k8sAlias)

	// Update via Sync API
	aliases := []models.ServiceAlias{alias}
	err = s.backendClient.Sync(ctx, models.SyncOpUpsert, aliases)
	if err != nil {
		return nil, false, fmt.Errorf("failed to update ServiceAlias: %w", err)
	}

	// Set successful status
	setCondition(k8sAlias, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "ServiceAlias successfully updated in backend")

	return k8sAlias, false, nil
}

// Delete removes a ServiceAlias from backend via Sync API
func (s *ServiceAliasStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
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

	k8sAlias, ok := obj.(*netguardv1beta1.ServiceAlias)
	if !ok {
		return nil, false, fmt.Errorf("expected ServiceAlias, got %T", obj)
	}

	// Convert to backend model
	alias := convertServiceAliasFromK8s(k8sAlias)

	// Delete via Sync API
	aliases := []models.ServiceAlias{alias}
	err = s.backendClient.Sync(ctx, models.SyncOpDelete, aliases)
	if err != nil {
		return nil, false, fmt.Errorf("failed to delete ServiceAlias: %w", err)
	}

	return k8sAlias, true, nil
}

// Watch implements watch functionality for ServiceAliases
func (s *ServiceAliasStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	pollerManager := watchpkg.GetPollerManager(s.backendClient)
	poller := pollerManager.GetPoller("servicealiases")

	client, err := poller.AddClient(options)
	if err != nil {
		return nil, fmt.Errorf("failed to add watch client: %w", err)
	}

	return watchpkg.NewPollerWatchInterface(client, poller), nil
}

// GetSupportedVerbs returns the supported verbs for this storage
func (s *ServiceAliasStorage) GetSupportedVerbs() []string {
	return []string{"get", "list", "create", "update", "delete", "watch"}
}

// Helper functions for conversion

func convertServiceAliasToK8s(alias models.ServiceAlias) *netguardv1beta1.ServiceAlias {
	k8sAlias := &netguardv1beta1.ServiceAlias{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "ServiceAlias",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      alias.ResourceIdentifier.Name,
			Namespace: alias.ResourceIdentifier.Namespace,
		},
		Spec: netguardv1beta1.ServiceAliasSpec{
			ServiceRef: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       alias.ServiceRef.Name,
			},
		},
	}

	return k8sAlias
}

func convertServiceAliasFromK8s(k8sAlias *netguardv1beta1.ServiceAlias) models.ServiceAlias {
	alias := models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sAlias.Name,
				models.WithNamespace(k8sAlias.Namespace),
			),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sAlias.Spec.ServiceRef.Name,
				models.WithNamespace(k8sAlias.Namespace),
			),
		},
	}

	return alias
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
