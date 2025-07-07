package rules2s

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	utils2 "netguard-pg-backend/internal/k8s/registry/utils"
	watchpkg "netguard-pg-backend/internal/k8s/registry/watch"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"
)

// RuleS2SStorage implements REST storage for RuleS2S resources
type RuleS2SStorage struct {
	backendClient client.BackendClient
}

// NewRuleS2SStorage creates a new RuleS2S storage
func NewRuleS2SStorage(backendClient client.BackendClient) *RuleS2SStorage {
	return &RuleS2SStorage{
		backendClient: backendClient,
	}
}

// New returns an empty RuleS2S object
func (s *RuleS2SStorage) New() runtime.Object {
	return &netguardv1beta1.RuleS2S{}
}

// NewList returns an empty RuleS2SList object
func (s *RuleS2SStorage) NewList() runtime.Object {
	return &netguardv1beta1.RuleS2SList{}
}

// NamespaceScoped returns true as RuleS2S are namespaced
func (s *RuleS2SStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name for the resource
func (s *RuleS2SStorage) GetSingularName() string {
	return "rules2s"
}

// Destroy cleans up resources on shutdown
func (s *RuleS2SStorage) Destroy() {
	// Nothing to clean up for now
}

// Get retrieves a RuleS2S by name from backend (READ-ONLY, no status changes)
func (s *RuleS2SStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Extract namespace from context
	namespace := utils2.NamespaceFrom(ctx)

	// Get from backend
	resourceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	rule, err := s.backendClient.GetRuleS2S(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get RuleS2S %s/%s: %w", namespace, name, err)
	}

	// Convert to Kubernetes format
	k8sRule := convertRuleS2SToK8s(*rule)
	return k8sRule, nil
}

// List retrieves RuleS2S from backend with filtering (READ-ONLY, no status changes)
func (s *RuleS2SStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	// Create scope for filtering
	var scope ports.Scope
	if options != nil && options.FieldSelector != nil {
		// Extract namespace from field selector if present
		// For now, implement basic namespace filtering
		scope = ports.NewResourceIdentifierScope()
	}

	// Get from backend
	rules, err := s.backendClient.ListRuleS2S(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list RuleS2S: %w", err)
	}

	// Convert to Kubernetes format
	k8sRuleList := &netguardv1beta1.RuleS2SList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "RuleS2SList",
		},
	}

	for _, rule := range rules {
		k8sRule := convertRuleS2SToK8s(rule)
		k8sRuleList.Items = append(k8sRuleList.Items, *k8sRule)
	}

	return k8sRuleList, nil
}

// Create creates a new RuleS2S in backend via Sync API
func (s *RuleS2SStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	k8sRule, ok := obj.(*netguardv1beta1.RuleS2S)
	if !ok {
		return nil, fmt.Errorf("expected RuleS2S, got %T", obj)
	}

	// Run validation if provided
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	rule := convertRuleS2SFromK8s(k8sRule)
	rule.Meta.TouchOnCreate()

	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.RuleS2S{rule}); err != nil {
		return nil, fmt.Errorf("failed to create RuleS2S: %w", err)
	}

	respModel, err := s.backendClient.GetRuleS2S(ctx, rule.ResourceIdentifier)
	if err != nil {
		respModel = &rule
	}
	resp := convertRuleS2SToK8s(*respModel)

	setCondition(resp, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "RuleS2S successfully created in backend")

	return resp, nil
}

// Update updates an existing RuleS2S in backend via Sync API
func (s *RuleS2SStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
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

	k8sRule, ok := updatedObj.(*netguardv1beta1.RuleS2S)
	if !ok {
		return nil, false, fmt.Errorf("expected RuleS2S, got %T", updatedObj)
	}

	// Run validation if provided
	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, currentObj); err != nil {
			return nil, false, err
		}
	}

	// Convert to backend model
	rule := convertRuleS2SFromK8s(k8sRule)

	// Update via Sync API
	rules := []models.RuleS2S{rule}
	err = s.backendClient.Sync(ctx, models.SyncOpUpsert, rules)
	if err != nil {
		return nil, false, fmt.Errorf("failed to update RuleS2S: %w", err)
	}

	// Set successful status
	setCondition(k8sRule, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "RuleS2S successfully updated in backend")

	return k8sRule, false, nil
}

// Delete removes a RuleS2S from backend via Sync API
func (s *RuleS2SStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
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

	k8sRule, ok := obj.(*netguardv1beta1.RuleS2S)
	if !ok {
		return nil, false, fmt.Errorf("expected RuleS2S, got %T", obj)
	}

	// Convert to backend model
	rule := convertRuleS2SFromK8s(k8sRule)

	// Delete via Sync API
	rules := []models.RuleS2S{rule}
	err = s.backendClient.Sync(ctx, models.SyncOpDelete, rules)
	if err != nil {
		return nil, false, fmt.Errorf("failed to delete RuleS2S: %w", err)
	}

	return k8sRule, true, nil
}

// Watch implements watch functionality for RuleS2S
func (s *RuleS2SStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	pollerManager := watchpkg.GetPollerManager(s.backendClient)
	poller := pollerManager.GetPoller("rules2s")

	client, err := poller.AddClient(options)
	if err != nil {
		return nil, fmt.Errorf("failed to add watch client: %w", err)
	}

	return watchpkg.NewPollerWatchInterface(client, poller), nil
}

// GetSupportedVerbs returns the supported verbs for this storage
func (s *RuleS2SStorage) GetSupportedVerbs() []string {
	return []string{"get", "list", "create", "update", "delete", "watch", "patch"}
}

// Patch applies JSON/Merge patch to RuleS2S and syncs backend.
func (s *RuleS2SStorage) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts *metav1.PatchOptions, subresources ...string) (runtime.Object, error) {
	if len(subresources) > 0 {
		return nil, apierrors.NewBadRequest("subresources are not supported for RuleS2S")
	}

	currObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	curr := currObj.(*netguardv1beta1.RuleS2S)
	currBytes, _ := json.Marshal(curr)

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
	default:
		return nil, apierrors.NewBadRequest("unsupported patch type")
	}

	var updated netguardv1beta1.RuleS2S
	if err := json.Unmarshal(patchedBytes, &updated); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid patched object: %v", err))
	}

	if updated.Name == "" {
		updated.Name = curr.Name
	}
	if updated.Namespace == "" {
		updated.Namespace = curr.Namespace
	}

	model := convertRuleS2SFromK8s(&updated)
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.RuleS2S{model}); err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to sync patched RuleS2S: %w", err))
	}

	return convertRuleS2SToK8s(model), nil
}

// ConvertToTable for kubectl printing.
func (s *RuleS2SStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Traffic", Type: "string"},
			{Name: "SrcService", Type: "string"},
			{Name: "DstService", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(obj *netguardv1beta1.RuleS2S) {
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: obj},
			Cells: []interface{}{
				obj.Name,
				obj.Spec.Traffic,
				obj.Spec.ServiceLocalRef.Name,
				obj.Spec.ServiceRef.Name,
				translateTimestampSince(obj.CreationTimestamp),
			},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *netguardv1beta1.RuleS2S:
		addRow(v)
	case *netguardv1beta1.RuleS2SList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}
	return table, nil
}

// Timestamp helpers
func translateTimestampSince(ts metav1.Time) string {
	if ts.IsZero() {
		return "<unknown>"
	}
	return durationShortHumanDuration(time.Since(ts.Time))
}

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

var (
	_ rest.Storage         = &RuleS2SStorage{}
	_ rest.Getter          = &RuleS2SStorage{}
	_ rest.Lister          = &RuleS2SStorage{}
	_ rest.Watcher         = &RuleS2SStorage{}
	_ rest.Creater         = &RuleS2SStorage{}
	_ rest.Updater         = &RuleS2SStorage{}
	_ rest.GracefulDeleter = &RuleS2SStorage{}
	_ rest.Patcher         = &RuleS2SStorage{}
)

// Helper functions for conversion

func convertRuleS2SToK8s(rule models.RuleS2S) *netguardv1beta1.RuleS2S {
	k8sRule := &netguardv1beta1.RuleS2S{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "RuleS2S",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rule.ResourceIdentifier.Name,
			Namespace: rule.ResourceIdentifier.Namespace,
		},
		Spec: netguardv1beta1.RuleS2SSpec{
			Traffic: string(rule.Traffic),
			ServiceLocalRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       rule.ServiceLocalRef.Name,
				},
				Namespace: rule.ServiceLocalRef.Namespace,
			},
			ServiceRef: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       rule.ServiceRef.Name,
				},
				Namespace: rule.ServiceRef.Namespace,
			},
		},
	}

	return k8sRule
}

func convertRuleS2SFromK8s(k8sRule *netguardv1beta1.RuleS2S) models.RuleS2S {
	rule := models.RuleS2S{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sRule.Name,
				models.WithNamespace(k8sRule.Namespace),
			),
		},
		Traffic: models.Traffic(k8sRule.Spec.Traffic),
		ServiceLocalRef: models.ServiceAliasRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sRule.Spec.ServiceLocalRef.Name,
				models.WithNamespace(k8sRule.Spec.ServiceLocalRef.Namespace),
			),
		},
		ServiceRef: models.ServiceAliasRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sRule.Spec.ServiceRef.Name,
				models.WithNamespace(k8sRule.Spec.ServiceRef.Namespace),
			),
		},
	}

	return rule
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
