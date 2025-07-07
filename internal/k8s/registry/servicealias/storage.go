package servicealias

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	utils2 "netguard-pg-backend/internal/k8s/registry/utils"
	watchpkg "netguard-pg-backend/internal/k8s/registry/watch"

	"errors"
	"strings"

	"k8s.io/apimachinery/pkg/types"
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
	namespace := utils2.NamespaceFrom(ctx)

	// Get from backend
	resourceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	alias, err := s.backendClient.GetServiceAlias(ctx, resourceID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) || strings.Contains(err.Error(), "entity not found") {
			return nil, apierrors.NewNotFound(netguardv1beta1.Resource("servicealiases"), name)
		}
		return nil, fmt.Errorf("failed to get ServiceAlias %s/%s: %w", namespace, name, err)
	}

	// Convert to Kubernetes format
	k8sAlias := convertServiceAliasToK8s(*alias)
	return k8sAlias, nil
}

// List retrieves ServiceAliases from backend with filtering (READ-ONLY, no status changes)
func (s *ServiceAliasStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	// Create scope for filtering
	scope := utils2.ScopeFromContext(ctx)

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
	backendAlias := convertServiceAliasFromK8s(k8sAlias)
	backendAlias.Meta.TouchOnCreate()

	// Create via Sync API
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.ServiceAlias{backendAlias}); err != nil {
		return nil, fmt.Errorf("backend sync failed: %w", err)
	}

	respModel, err := s.backendClient.GetServiceAlias(ctx, backendAlias.ResourceIdentifier)
	if err != nil {
		respModel = &backendAlias
	}
	resp := convertServiceAliasToK8s(*respModel)
	return resp, nil
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
	backendAlias := convertServiceAliasFromK8s(k8sAlias)

	// Update via Sync API
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.ServiceAlias{backendAlias}); err != nil {
		return nil, false, fmt.Errorf("backend sync failed: %w", err)
	}

	respModel, err := s.backendClient.GetServiceAlias(ctx, backendAlias.ResourceIdentifier)
	if err != nil {
		respModel = &backendAlias
	}
	resp := convertServiceAliasToK8s(*respModel)
	return resp, false, nil
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
	backendAlias := convertServiceAliasFromK8s(k8sAlias)

	// Delete via Sync API
	if err := s.backendClient.Sync(ctx, models.SyncOpDelete, []models.ServiceAlias{backendAlias}); err != nil {
		return nil, false, fmt.Errorf("failed to delete ServiceAlias: %w", err)
	}

	respModel, err := s.backendClient.GetServiceAlias(ctx, backendAlias.ResourceIdentifier)
	if err != nil {
		respModel = &backendAlias
	}
	resp := convertServiceAliasToK8s(*respModel)
	return resp, true, nil
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
	return []string{"get", "list", "create", "update", "delete", "watch", "patch"}
}

// Helper functions for conversion

func convertServiceAliasToK8s(alias models.ServiceAlias) *netguardv1beta1.ServiceAlias {
	uid := types.UID(alias.Meta.UID)
	if uid == "" {
		uid = types.UID(fmt.Sprintf("%s.%s", alias.ResourceIdentifier.Namespace, alias.ResourceIdentifier.Name))
	}

	k8sAlias := &netguardv1beta1.ServiceAlias{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "ServiceAlias",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              alias.ResourceIdentifier.Name,
			Namespace:         alias.ResourceIdentifier.Namespace,
			UID:               uid,
			ResourceVersion:   alias.Meta.ResourceVersion,
			Generation:        alias.Meta.Generation,
			CreationTimestamp: alias.Meta.CreationTS,
			Labels:            alias.Meta.Labels,
			Annotations:       alias.Meta.Annotations,
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
		Meta: models.Meta{
			UID:             string(k8sAlias.UID),
			ResourceVersion: k8sAlias.ResourceVersion,
			Generation:      k8sAlias.Generation,
			CreationTS:      k8sAlias.CreationTimestamp,
			Labels:          k8sAlias.Labels,
			Annotations:     k8sAlias.Annotations,
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

// Compile-time assertions to ensure we expose required verbs.
var (
	_ rest.Storage         = &ServiceAliasStorage{}
	_ rest.Getter          = &ServiceAliasStorage{}
	_ rest.Lister          = &ServiceAliasStorage{}
	_ rest.Watcher         = &ServiceAliasStorage{}
	_ rest.Creater         = &ServiceAliasStorage{}
	_ rest.Updater         = &ServiceAliasStorage{}
	_ rest.GracefulDeleter = &ServiceAliasStorage{}
	_ rest.Patcher         = &ServiceAliasStorage{}
)

func (s *ServiceAliasStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Service", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(sa *netguardv1beta1.ServiceAlias) {
		serviceName := sa.Spec.ServiceRef.Name
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: sa},
			Cells:  []interface{}{sa.Name, serviceName, translateTimestampSince(sa.CreationTimestamp)},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *netguardv1beta1.ServiceAlias:
		addRow(v)
	case *netguardv1beta1.ServiceAliasList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}
	return table, nil
}

// translateTimestampSince returns the elapsed time since timestamp in human-readable form.
func translateTimestampSince(ts metav1.Time) string {
	if ts.IsZero() {
		return "<unknown>"
	}
	return durationShortHumanDuration(time.Since(ts.Time))
}

// durationShortHumanDuration is a copy of kube ctl printing helper (short).
func durationShortHumanDuration(d time.Duration) string {
	if seconds := int(d.Seconds()); seconds < 90 {
		return fmt.Sprintf("%ds", seconds)
	}
	if minutes := int(d.Minutes()); minutes < 90 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := int(d.Round(time.Hour).Hours())
	if hours < 48 {
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%dd", days)
}

// Patch applies a patch to an existing ServiceAlias.
// It supports JSONPatch (RFC6902) and JSONMerge/Apply patches. Strategic patches are not implemented.
// Subresources are not supported and will return a BadRequest error.
func (s *ServiceAliasStorage) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts *metav1.PatchOptions, subresources ...string) (runtime.Object, error) {
	// Reject subresource patches – not implemented for ServiceAlias.
	if len(subresources) > 0 {
		return nil, apierrors.NewBadRequest("subresources are not supported for ServiceAlias")
	}

	// Fetch current object
	currObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	currSA, ok := currObj.(*netguardv1beta1.ServiceAlias)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("unexpected object type %T", currObj))
	}

	// Marshal current object to JSON for patch application
	currBytes, err := json.Marshal(currSA)
	if err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to marshal current object: %w", err))
	}

	var patchedBytes []byte
	switch pt {
	case types.JSONPatchType:
		patch, err := jsonpatch.DecodePatch(data)
		if err != nil {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid JSON patch: %v", err))
		}
		patchedBytes, err = patch.Apply(currBytes)
		if err != nil {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("failed to apply JSON patch: %v", err))
		}
	case types.MergePatchType, types.ApplyPatchType:
		patchedBytes, err = jsonpatch.MergePatch(currBytes, data)
		if err != nil {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("failed to apply merge patch: %v", err))
		}
	case types.StrategicMergePatchType:
		return nil, apierrors.NewBadRequest("strategic merge patches are not supported for ServiceAlias")
	default:
		return nil, apierrors.NewBadRequest(fmt.Sprintf("unsupported patch type %s", pt))
	}

	var updatedSA netguardv1beta1.ServiceAlias
	if err := json.Unmarshal(patchedBytes, &updatedSA); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid patched object: %v", err))
	}

	// Ensure name/namespace are kept (patch spec may omit them)
	if updatedSA.Name == "" {
		updatedSA.Name = currSA.Name
	}
	if updatedSA.Namespace == "" {
		updatedSA.Namespace = currSA.Namespace
	}

	// Convert to backend model and upsert.
	saModel := convertServiceAliasFromK8s(&updatedSA)
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.ServiceAlias{saModel}); err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to sync patched ServiceAlias: %w", err))
	}

	resp := convertServiceAliasToK8s(saModel)

	return resp, nil
}
