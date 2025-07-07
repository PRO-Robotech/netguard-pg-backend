package addressgroupbindingpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// AddressGroupBindingPolicyStorage implements REST storage for AddressGroupBindingPolicy resources
type AddressGroupBindingPolicyStorage struct {
	backendClient client.BackendClient
}

// NewAddressGroupBindingPolicyStorage creates a new AddressGroupBindingPolicy storage
func NewAddressGroupBindingPolicyStorage(backendClient client.BackendClient) *AddressGroupBindingPolicyStorage {
	return &AddressGroupBindingPolicyStorage{
		backendClient: backendClient,
	}
}

// New returns an empty AddressGroupBindingPolicy object
func (s *AddressGroupBindingPolicyStorage) New() runtime.Object {
	return &netguardv1beta1.AddressGroupBindingPolicy{}
}

// NewList returns an empty AddressGroupBindingPolicyList object
func (s *AddressGroupBindingPolicyStorage) NewList() runtime.Object {
	return &netguardv1beta1.AddressGroupBindingPolicyList{}
}

// NamespaceScoped returns true as AddressGroupBindingPolicies are namespaced
func (s *AddressGroupBindingPolicyStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name for the resource
func (s *AddressGroupBindingPolicyStorage) GetSingularName() string {
	return "addressgroupbindingpolicy"
}

// Destroy cleans up resources on shutdown
func (s *AddressGroupBindingPolicyStorage) Destroy() {
	// Nothing to clean up for now
}

// Get retrieves an AddressGroupBindingPolicy by name from backend
func (s *AddressGroupBindingPolicyStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace := utils2.NamespaceFrom(ctx)

	resourceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	policy, err := s.backendClient.GetAddressGroupBindingPolicy(ctx, resourceID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) || strings.Contains(err.Error(), "entity not found") {
			return nil, apierrors.NewNotFound(netguardv1beta1.Resource("addressgroupbindingpolicies"), name)
		}
		return nil, fmt.Errorf("failed to get AddressGroupBindingPolicy %s/%s: %w", namespace, name, err)
	}

	k8sPolicy := convertAddressGroupBindingPolicyToK8s(*policy)
	return k8sPolicy, nil
}

// List retrieves AddressGroupBindingPolicies from backend with filtering
func (s *AddressGroupBindingPolicyStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	scope := utils2.ScopeFromContext(ctx)

	policies, err := s.backendClient.ListAddressGroupBindingPolicies(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list AddressGroupBindingPolicies: %w", err)
	}

	k8sPolicyList := &netguardv1beta1.AddressGroupBindingPolicyList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "AddressGroupBindingPolicyList",
		},
	}

	for _, policy := range policies {
		k8sPolicy := convertAddressGroupBindingPolicyToK8s(policy)
		k8sPolicyList.Items = append(k8sPolicyList.Items, *k8sPolicy)
	}

	return k8sPolicyList, nil
}

// Create creates a new AddressGroupBindingPolicy in backend
func (s *AddressGroupBindingPolicyStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	k8sPolicy, ok := obj.(*netguardv1beta1.AddressGroupBindingPolicy)
	if !ok {
		return nil, fmt.Errorf("expected AddressGroupBindingPolicy, got %T", obj)
	}

	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	policy := convertAddressGroupBindingPolicyFromK8s(k8sPolicy)
	policy.Meta.TouchOnCreate()
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.AddressGroupBindingPolicy{policy}); err != nil {
		return nil, fmt.Errorf("backend sync failed: %w", err)
	}

	respModel, err := s.backendClient.GetAddressGroupBindingPolicy(ctx, policy.ResourceIdentifier)
	if err != nil {
		respModel = &policy
	}
	resp := convertAddressGroupBindingPolicyToK8s(*respModel)

	setCondition(resp, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "AddressGroupBindingPolicy successfully created in backend")

	return resp, nil
}

// Update updates an existing AddressGroupBindingPolicy in backend
func (s *AddressGroupBindingPolicyStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	currentObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		if forceAllowCreate {
			newObj, err := objInfo.UpdatedObject(ctx, nil)
			if err != nil {
				return nil, false, err
			}
			createdObj, err := s.Create(ctx, newObj, createValidation, &metav1.CreateOptions{})
			return createdObj, true, err
		}
		return nil, false, err
	}

	updatedObj, err := objInfo.UpdatedObject(ctx, currentObj)
	if err != nil {
		return nil, false, err
	}

	k8sPolicy, ok := updatedObj.(*netguardv1beta1.AddressGroupBindingPolicy)
	if !ok {
		return nil, false, fmt.Errorf("expected AddressGroupBindingPolicy, got %T", updatedObj)
	}

	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, currentObj); err != nil {
			return nil, false, err
		}
	}

	policy := convertAddressGroupBindingPolicyFromK8s(k8sPolicy)
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.AddressGroupBindingPolicy{policy}); err != nil {
		return nil, false, fmt.Errorf("backend sync failed: %w", err)
	}

	respModel, err := s.backendClient.GetAddressGroupBindingPolicy(ctx, policy.ResourceIdentifier)
	if err != nil {
		respModel = &policy
	}
	resp := convertAddressGroupBindingPolicyToK8s(*respModel)

	setCondition(resp, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "AddressGroupBindingPolicy successfully updated in backend")

	return resp, false, nil
}

// Delete removes an AddressGroupBindingPolicy from backend
func (s *AddressGroupBindingPolicyStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	obj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	if deleteValidation != nil {
		if err := deleteValidation(ctx, obj); err != nil {
			return nil, false, err
		}
	}

	namespace := utils2.NamespaceFrom(ctx)
	id := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	if err := s.backendClient.Sync(ctx, models.SyncOpDelete, []models.AddressGroupBindingPolicy{{SelfRef: models.SelfRef{ResourceIdentifier: id}}}); err != nil {
		return nil, false, fmt.Errorf("backend delete sync failed: %w", err)
	}

	return obj, true, nil
}

// Watch implements watch functionality for AddressGroupBindingPolicies
func (s *AddressGroupBindingPolicyStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	pollerManager := watchpkg.GetPollerManager(s.backendClient)
	poller := pollerManager.GetPoller("addressgroupbindingpolicies")

	client, err := poller.AddClient(options)
	if err != nil {
		return nil, fmt.Errorf("failed to add watch client: %w", err)
	}

	return watchpkg.NewPollerWatchInterface(client, poller), nil
}

// Helper functions for conversion
func convertAddressGroupBindingPolicyToK8s(policy models.AddressGroupBindingPolicy) *netguardv1beta1.AddressGroupBindingPolicy {
	k8sPolicy := &netguardv1beta1.AddressGroupBindingPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "AddressGroupBindingPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              policy.ResourceIdentifier.Name,
			Namespace:         policy.ResourceIdentifier.Namespace,
			UID:               types.UID(policy.Meta.UID),
			ResourceVersion:   policy.Meta.ResourceVersion,
			Generation:        policy.Meta.Generation,
			CreationTimestamp: policy.Meta.CreationTS,
			Labels:            policy.Meta.Labels,
			Annotations:       policy.Meta.Annotations,
		},
		Spec: netguardv1beta1.AddressGroupBindingPolicySpec{
			AddressGroupRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       policy.AddressGroupRef.Name,
				},
				Namespace: policy.AddressGroupRef.Namespace,
			},
			ServiceRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       policy.ServiceRef.Name,
				},
				Namespace: policy.ServiceRef.Namespace,
			},
		},
	}

	return k8sPolicy
}

func convertAddressGroupBindingPolicyFromK8s(k8sPolicy *netguardv1beta1.AddressGroupBindingPolicy) models.AddressGroupBindingPolicy {
	policy := models.AddressGroupBindingPolicy{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sPolicy.Name,
				models.WithNamespace(k8sPolicy.Namespace),
			),
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sPolicy.Spec.ServiceRef.Name,
				models.WithNamespace(k8sPolicy.Spec.ServiceRef.Namespace),
			),
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sPolicy.Spec.AddressGroupRef.Name,
				models.WithNamespace(k8sPolicy.Spec.AddressGroupRef.Namespace),
			),
		},
		Meta: models.Meta{
			UID:             string(k8sPolicy.UID),
			ResourceVersion: k8sPolicy.ResourceVersion,
			Generation:      k8sPolicy.Generation,
			CreationTS:      k8sPolicy.CreationTimestamp,
			Labels:          k8sPolicy.Labels,
			Annotations:     k8sPolicy.Annotations,
		},
	}

	return policy
}

// Status condition helpers
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
}

// GetSupportedVerbs returns supported verbs for this storage
func (s *AddressGroupBindingPolicyStorage) GetSupportedVerbs() []string {
	return []string{"get", "list", "create", "update", "delete", "watch", "patch"}
}

// Patch applies JSON or Merge patches to an existing AddressGroupBindingPolicy.
// Strategic merge patches are not supported.
func (s *AddressGroupBindingPolicyStorage) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts *metav1.PatchOptions, subresources ...string) (runtime.Object, error) {
	if len(subresources) > 0 {
		return nil, apierrors.NewBadRequest("subresources are not supported for AddressGroupBindingPolicy")
	}

	currObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	currPolicy, ok := currObj.(*netguardv1beta1.AddressGroupBindingPolicy)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("unexpected object type %T", currObj))
	}

	currBytes, err := json.Marshal(currPolicy)
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
		return nil, apierrors.NewBadRequest("strategic merge patches are not supported for AddressGroupBindingPolicy")
	default:
		return nil, apierrors.NewBadRequest(fmt.Sprintf("unsupported patch type %s", pt))
	}

	var updated netguardv1beta1.AddressGroupBindingPolicy
	if err := json.Unmarshal(patchedBytes, &updated); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid patched object: %v", err))
	}

	if updated.Name == "" {
		updated.Name = currPolicy.Name
	}
	if updated.Namespace == "" {
		updated.Namespace = currPolicy.Namespace
	}

	model := convertAddressGroupBindingPolicyFromK8s(&updated)
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.AddressGroupBindingPolicy{model}); err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to sync patched AddressGroupBindingPolicy: %w", err))
	}

	respModel, err := s.backendClient.GetAddressGroupBindingPolicy(ctx, model.ResourceIdentifier)
	if err != nil {
		respModel = &model
	}
	return convertAddressGroupBindingPolicyToK8s(*respModel), nil
}

// ConvertToTable provides a table representation for kubectl printers.
func (s *AddressGroupBindingPolicyStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Service", Type: "string"},
			{Name: "AddressGroup", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(p *netguardv1beta1.AddressGroupBindingPolicy) {
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: p},
			Cells: []interface{}{
				p.Name,
				p.Spec.ServiceRef.Name,
				p.Spec.AddressGroupRef.Name,
				translateTimestampSince(p.CreationTimestamp),
			},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *netguardv1beta1.AddressGroupBindingPolicy:
		addRow(v)
	case *netguardv1beta1.AddressGroupBindingPolicyList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}
	return table, nil
}

// Helper for age formatting
func translateTimestampSince(ts metav1.Time) string {
	if ts.IsZero() {
		return "<unknown>"
	}
	return durationHumanShort(time.Since(ts.Time))
}

func durationHumanShort(d time.Duration) string {
	if seconds := int(d.Seconds()); seconds < 90 {
		return fmt.Sprintf("%ds", seconds)
	}
	if minutes := int(d.Minutes()); minutes < 90 {
		return fmt.Sprintf("%dm", minutes)
	}
	h := int(d.Round(time.Hour).Hours())
	if h < 48 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dd", h/24)
}

// Compile-time assertions to verify interface conformance
var (
	_ rest.Storage         = &AddressGroupBindingPolicyStorage{}
	_ rest.Getter          = &AddressGroupBindingPolicyStorage{}
	_ rest.Lister          = &AddressGroupBindingPolicyStorage{}
	_ rest.Watcher         = &AddressGroupBindingPolicyStorage{}
	_ rest.Creater         = &AddressGroupBindingPolicyStorage{}
	_ rest.Updater         = &AddressGroupBindingPolicyStorage{}
	_ rest.GracefulDeleter = &AddressGroupBindingPolicyStorage{}
	_ rest.Patcher         = &AddressGroupBindingPolicyStorage{}
	_ rest.TableConvertor  = &AddressGroupBindingPolicyStorage{}
)
