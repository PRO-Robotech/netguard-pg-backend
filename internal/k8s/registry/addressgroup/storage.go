package addressgroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	utils2 "netguard-pg-backend/internal/k8s/registry/utils"
	watchpkg "netguard-pg-backend/internal/k8s/registry/watch"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

// AddressGroupStorage implements REST storage for AddressGroup resources
type AddressGroupStorage struct {
	backendClient client.BackendClient
}

// Compile-time assertions to ensure we expose required verbs.
var (
	_ rest.Storage         = &AddressGroupStorage{}
	_ rest.Getter          = &AddressGroupStorage{}
	_ rest.Lister          = &AddressGroupStorage{}
	_ rest.Watcher         = &AddressGroupStorage{}
	_ rest.Creater         = &AddressGroupStorage{}
	_ rest.Updater         = &AddressGroupStorage{}
	_ rest.GracefulDeleter = &AddressGroupStorage{}
	_ rest.Patcher         = &AddressGroupStorage{}
)

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
	namespace := utils2.NamespaceFrom(ctx)
	klog.V(4).Infof("AddressGroupStorage.Get namespace=%q name=%q", namespace, name)
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	// Get from backend
	resourceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	ag, err := s.backendClient.GetAddressGroup(ctx, resourceID)
	if err == nil {
		return convertAddressGroupToK8s(*ag), nil
	}
	if errors.Is(err, ports.ErrNotFound) || strings.Contains(err.Error(), "entity not found") {
		// fallback: object still in kube storage but not synced
		return nil, apierrors.NewNotFound(netguardv1beta1.Resource("addressgroups"), name)
	}
	return nil, fmt.Errorf("failed to get AddressGroup %s/%s: %w", namespace, name, err)
}

// List retrieves AddressGroups from backend with filtering (READ-ONLY, no status changes)
func (s *AddressGroupStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	// Create scope for filtering
	scope := utils2.ScopeFromContext(ctx)

	// Get from backend
	addressGroups, err := s.backendClient.ListAddressGroups(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list AddressGroups: %w", err)
	}

	// Sort deterministically by namespace, then name to match default kube-apiserver ordering.
	utils2.SortByNamespaceName(addressGroups, func(ag models.AddressGroup) models.ResourceIdentifier {
		return ag.ResourceIdentifier
	})

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

	// Synchronous create in backend
	addressGroups := []models.AddressGroup{addressGroup}
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, addressGroups); err != nil {
		return nil, fmt.Errorf("backend sync failed: %w", err)
	}

	resp := convertAddressGroupToK8s(addressGroup)

	// Set successful status
	setCondition(resp, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "AddressGroup successfully created in backend")

	return resp, nil
}

// Update updates an existing AddressGroup in backend via Sync API
func (s *AddressGroupStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// Get current object
	currentObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		// If object not found, attempt to create irrespective of forceAllowCreate to
		// better support kubectl apply on non-existent resources.
		if apierrors.IsNotFound(err) || forceAllowCreate {
			newObj, err2 := objInfo.UpdatedObject(ctx, nil)
			if err2 != nil {
				return nil, false, err2
			}
			createdObj, err2 := s.Create(ctx, newObj, createValidation, &metav1.CreateOptions{})
			return createdObj, true, err2
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

	resp := convertAddressGroupToK8s(addressGroup)

	// Set successful status
	setCondition(resp, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "AddressGroup successfully updated in backend")

	return resp, false, nil
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
	return []string{"get", "list", "create", "update", "delete", "watch", "patch"}
}

// ConvertToTable provides a minimal table representation so that kubectl
// can print the objects even when "-o wide" или default output запрашивает
// server-side преобразование.
func (s *AddressGroupStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Addresses", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(ag *netguardv1beta1.AddressGroup) {
		addrs := make([]string, 0, len(ag.Spec.Addresses))
		for _, a := range ag.Spec.Addresses {
			addrs = append(addrs, a.Address)
		}
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: ag},
			Cells:  []interface{}{ag.Name, strings.Join(addrs, ","), translateTimestampSince(ag.CreationTimestamp)},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *netguardv1beta1.AddressGroup:
		addRow(v)
	case *netguardv1beta1.AddressGroupList:
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

// Helper functions for conversion

func convertAddressGroupToK8s(addressGroup models.AddressGroup) *netguardv1beta1.AddressGroup {
	uid := types.UID(addressGroup.Meta.UID)
	if uid == "" {
		uid = types.UID(fmt.Sprintf("%s.%s", addressGroup.ResourceIdentifier.Namespace, addressGroup.ResourceIdentifier.Name))
	}
	resourceVersion := addressGroup.Meta.ResourceVersion
	k8sAddressGroup := &netguardv1beta1.AddressGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "AddressGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              addressGroup.ResourceIdentifier.Name,
			Namespace:         addressGroup.ResourceIdentifier.Namespace,
			UID:               uid,
			ResourceVersion:   resourceVersion,
			CreationTimestamp: addressGroup.Meta.CreationTS,
			Labels:            addressGroup.Meta.Labels,
			Annotations:       addressGroup.Meta.Annotations,
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
		Meta: models.Meta{
			UID:             string(k8sAddressGroup.UID),
			ResourceVersion: k8sAddressGroup.ResourceVersion,
			Generation:      k8sAddressGroup.Generation,
			CreationTS:      k8sAddressGroup.CreationTimestamp,
			Labels:          k8sAddressGroup.Labels,
			Annotations:     k8sAddressGroup.Annotations,
		},
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

// Patch applies a patch to an existing AddressGroup.
// It supports JSONPatch (RFC6902) and JSONMerge/Apply patches. Strategic patches are not implemented.
// Subresources are not supported and will return a BadRequest error.
func (s *AddressGroupStorage) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts *metav1.PatchOptions, subresources ...string) (runtime.Object, error) {
	// Reject subresource patches – not implemented for AddressGroup.
	if len(subresources) > 0 {
		return nil, apierrors.NewBadRequest("subresources are not supported for AddressGroup")
	}

	// Fetch current object
	currObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	currAG, ok := currObj.(*netguardv1beta1.AddressGroup)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("unexpected object type %T", currObj))
	}

	// Marshal current object to JSON for patch application
	currBytes, err := json.Marshal(currAG)
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
		return nil, apierrors.NewBadRequest("strategic merge patches are not supported for AddressGroup")
	default:
		return nil, apierrors.NewBadRequest(fmt.Sprintf("unsupported patch type %s", pt))
	}

	var updatedAG netguardv1beta1.AddressGroup
	if err := json.Unmarshal(patchedBytes, &updatedAG); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid patched object: %v", err))
	}

	// Ensure name/namespace are kept (patch spec may omit them)
	if updatedAG.Name == "" {
		updatedAG.Name = currAG.Name
	}
	if updatedAG.Namespace == "" {
		updatedAG.Namespace = currAG.Namespace
	}

	// Convert to backend model and upsert.
	agModel := convertAddressGroupFromK8s(&updatedAG)
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.AddressGroup{agModel}); err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to sync patched AddressGroup: %w", err))
	}

	resp := convertAddressGroupToK8s(agModel)

	return resp, nil
}
