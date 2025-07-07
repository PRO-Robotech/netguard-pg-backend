package ieagagrule

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	utils2 "netguard-pg-backend/internal/k8s/registry/utils"
	watchpkg "netguard-pg-backend/internal/k8s/registry/watch"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"
)

// IEAgAgRuleStorage implements REST storage for IEAgAgRule resources
type IEAgAgRuleStorage struct {
	backendClient client.BackendClient
}

// NewIEAgAgRuleStorage creates a new IEAgAgRule storage
func NewIEAgAgRuleStorage(backendClient client.BackendClient) *IEAgAgRuleStorage {
	return &IEAgAgRuleStorage{
		backendClient: backendClient,
	}
}

// New returns an empty IEAgAgRule object
func (s *IEAgAgRuleStorage) New() runtime.Object {
	return &netguardv1beta1.IEAgAgRule{}
}

// NewList returns an empty IEAgAgRuleList object
func (s *IEAgAgRuleStorage) NewList() runtime.Object {
	return &netguardv1beta1.IEAgAgRuleList{}
}

// NamespaceScoped returns true as IEAgAgRules are namespaced
func (s *IEAgAgRuleStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name for the resource
func (s *IEAgAgRuleStorage) GetSingularName() string {
	return "ieagagrule"
}

// Destroy cleans up resources on shutdown
func (s *IEAgAgRuleStorage) Destroy() {
	// Nothing to clean up for now
}

// Get retrieves an IEAgAgRule by name from backend
func (s *IEAgAgRuleStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace := utils2.NamespaceFrom(ctx)

	resourceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	rule, err := s.backendClient.GetIEAgAgRule(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get IEAgAgRule %s/%s: %w", namespace, name, err)
	}

	k8sRule := convertIEAgAgRuleToK8s(*rule)
	return k8sRule, nil
}

// List retrieves IEAgAgRules from backend with filtering
func (s *IEAgAgRuleStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	var scope ports.Scope
	if options != nil && options.FieldSelector != nil {
		scope = ports.NewResourceIdentifierScope()
	}

	rules, err := s.backendClient.ListIEAgAgRules(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list IEAgAgRules: %w", err)
	}

	k8sRuleList := &netguardv1beta1.IEAgAgRuleList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "IEAgAgRuleList",
		},
	}

	for _, rule := range rules {
		k8sRule := convertIEAgAgRuleToK8s(rule)
		k8sRuleList.Items = append(k8sRuleList.Items, *k8sRule)
	}

	return k8sRuleList, nil
}

// Create creates a new IEAgAgRule in backend via Sync API
func (s *IEAgAgRuleStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	k8sRule, ok := obj.(*netguardv1beta1.IEAgAgRule)
	if !ok {
		return nil, fmt.Errorf("expected IEAgAgRule, got %T", obj)
	}

	rule := convertIEAgAgRuleFromK8s(*k8sRule)
	rule.Meta.TouchOnCreate()

	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.IEAgAgRule{rule}); err != nil {
		return nil, fmt.Errorf("failed to create IEAgAgRule via sync: %w", err)
	}

	respModel, err := s.backendClient.GetIEAgAgRule(ctx, rule.ResourceIdentifier)
	if err != nil {
		respModel = &rule
	}
	resp := convertIEAgAgRuleToK8s(*respModel)

	return resp, nil
}

// Update updates an IEAgAgRule in backend via Sync API
func (s *IEAgAgRuleStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	existing, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	updated, err := objInfo.UpdatedObject(ctx, existing)
	if err != nil {
		return nil, false, err
	}

	k8sRule, ok := updated.(*netguardv1beta1.IEAgAgRule)
	if !ok {
		return nil, false, fmt.Errorf("expected IEAgAgRule, got %T", updated)
	}

	// Convert to backend model
	rule := convertIEAgAgRuleFromK8s(*k8sRule)

	// Update via Sync API
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.IEAgAgRule{rule}); err != nil {
		return nil, false, fmt.Errorf("failed to update IEAgAgRule via sync: %w", err)
	}

	return k8sRule, false, nil
}

// Delete deletes an IEAgAgRule from backend via Sync API
func (s *IEAgAgRuleStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	existing, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	k8sRule := existing.(*netguardv1beta1.IEAgAgRule)

	// Convert to backend model
	rule := convertIEAgAgRuleFromK8s(*k8sRule)

	// Delete via Sync API
	if err := s.backendClient.Sync(ctx, models.SyncOpDelete, []models.IEAgAgRule{rule}); err != nil {
		return nil, false, fmt.Errorf("failed to delete IEAgAgRule via sync: %w", err)
	}

	return k8sRule, true, nil
}

// Watch implements watch functionality
func (s *IEAgAgRuleStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	pollerManager := watchpkg.GetPollerManager(s.backendClient)
	poller := pollerManager.GetPoller("ieagagrules")

	client, err := poller.AddClient(options)
	if err != nil {
		return nil, fmt.Errorf("failed to add watch client: %w", err)
	}

	return watchpkg.NewPollerWatchInterface(client, poller), nil
}

// convertIEAgAgRuleToK8s converts from backend model to K8s API
func convertIEAgAgRuleToK8s(rule models.IEAgAgRule) *netguardv1beta1.IEAgAgRule {
	return &netguardv1beta1.IEAgAgRule{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "IEAgAgRule",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rule.SelfRef.Name,
			Namespace: rule.SelfRef.Namespace,
		},
		Spec: netguardv1beta1.IEAgAgRuleSpec{
			Transport: string(rule.Transport),
			Traffic:   string(rule.Traffic),
			AddressGroupLocal: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       rule.AddressGroupLocal.Name,
			},
			AddressGroup: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       rule.AddressGroup.Name,
			},
			Ports:    convertPortSpecsToK8s(rule.Ports),
			Action:   string(rule.Action),
			Priority: rule.Priority,
		},
	}
}

// convertIEAgAgRuleFromK8s converts from K8s API to backend model
func convertIEAgAgRuleFromK8s(k8sRule netguardv1beta1.IEAgAgRule) models.IEAgAgRule {
	return models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sRule.Name,
				models.WithNamespace(k8sRule.Namespace),
			),
		},
		Transport: models.TransportProtocol(k8sRule.Spec.Transport),
		Traffic:   models.Traffic(k8sRule.Spec.Traffic),
		AddressGroupLocal: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sRule.Spec.AddressGroupLocal.Name,
				models.WithNamespace(k8sRule.Namespace), // Same namespace as rule
			),
		},
		AddressGroup: models.AddressGroupRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sRule.Spec.AddressGroup.Name,
				models.WithNamespace(k8sRule.Namespace), // Same namespace as rule
			),
		},
		Ports:    convertPortSpecsFromK8s(k8sRule.Spec.Ports),
		Action:   models.RuleAction(k8sRule.Spec.Action),
		Priority: k8sRule.Spec.Priority,
	}
}

// Helper functions for port conversion
func convertPortSpecsToK8s(portSpecs []models.PortSpec) []netguardv1beta1.PortSpec {
	var k8sPortSpecs []netguardv1beta1.PortSpec
	for _, portSpec := range portSpecs {
		k8sPortSpec := netguardv1beta1.PortSpec{}

		// Convert destination port string to PortRange
		if portSpec.Destination != "" {
			// Parse port string using existing validation function
			portRanges, err := validation.ParsePortRanges(portSpec.Destination)
			if err == nil && len(portRanges) > 0 {
				// Use first port range
				portRange := portRanges[0]
				if portRange.Start == portRange.End {
					// Single port
					k8sPortSpec.Port = int32(portRange.Start)
				} else {
					// Port range
					k8sPortSpec.PortRange = &netguardv1beta1.PortRange{
						From: int32(portRange.Start),
						To:   int32(portRange.End),
					}
				}
			}
		}

		k8sPortSpecs = append(k8sPortSpecs, k8sPortSpec)
	}
	return k8sPortSpecs
}

func convertPortSpecsFromK8s(k8sPortSpecs []netguardv1beta1.PortSpec) []models.PortSpec {
	var portSpecs []models.PortSpec
	for _, k8sPortSpec := range k8sPortSpecs {
		portSpec := models.PortSpec{}

		if k8sPortSpec.Port != 0 {
			portSpec.Destination = fmt.Sprintf("%d", k8sPortSpec.Port)
		} else if k8sPortSpec.PortRange != nil {
			portSpec.Destination = fmt.Sprintf("%d-%d", k8sPortSpec.PortRange.From, k8sPortSpec.PortRange.To)
		}

		portSpecs = append(portSpecs, portSpec)
	}
	return portSpecs
}

// formatPortRangesToString converts []models.PortRange to comma-separated string like "80,443,8080-9090"
func formatPortRangesToString(ranges []models.PortRange) string {
	var parts []string
	for _, portRange := range ranges {
		if portRange.Start == portRange.End {
			// Single port
			parts = append(parts, fmt.Sprintf("%d", portRange.Start))
		} else {
			// Port range
			parts = append(parts, fmt.Sprintf("%d-%d", portRange.Start, portRange.End))
		}
	}
	return strings.Join(parts, ",")
}

func (s *IEAgAgRuleStorage) GetSupportedVerbs() []string {
	return []string{"get", "list", "create", "update", "delete", "watch", "patch"}
}

// Patch applies JSON/MergePatch to IEAgAgRule and syncs backend.
func (s *IEAgAgRuleStorage) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts *metav1.PatchOptions, subresources ...string) (runtime.Object, error) {
	if len(subresources) > 0 {
		return nil, apierrors.NewBadRequest("subresources are not supported for IEAgAgRule")
	}

	currObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	curr := currObj.(*netguardv1beta1.IEAgAgRule)
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

	var updated netguardv1beta1.IEAgAgRule
	if err := json.Unmarshal(patchedBytes, &updated); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid patched object: %v", err))
	}

	if updated.Name == "" {
		updated.Name = curr.Name
	}
	if updated.Namespace == "" {
		updated.Namespace = curr.Namespace
	}

	model := convertIEAgAgRuleFromK8s(updated)
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.IEAgAgRule{model}); err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to sync patched IEAgAgRule: %w", err))
	}

	return convertIEAgAgRuleToK8s(model), nil
}

// ConvertToTable provides kubectl columnar output.
func (s *IEAgAgRuleStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Dir", Type: "string"},
			{Name: "Transport", Type: "string"},
			{Name: "LocalAG", Type: "string"},
			{Name: "RemoteAG", Type: "string"},
			{Name: "Ports", Type: "string"},
			{Name: "Priority", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	formatPorts := func(ps []netguardv1beta1.PortSpec) string {
		var arr []string
		for _, p := range ps {
			if p.Port != 0 {
				arr = append(arr, fmt.Sprintf("%d", p.Port))
			} else if p.PortRange != nil {
				arr = append(arr, fmt.Sprintf("%d-%d", p.PortRange.From, p.PortRange.To))
			}
		}
		return strings.Join(arr, ",")
	}

	addRow := func(obj *netguardv1beta1.IEAgAgRule) {
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: obj},
			Cells: []interface{}{
				obj.Name,
				obj.Spec.Traffic,
				obj.Spec.Transport,
				obj.Spec.AddressGroupLocal.Name,
				obj.Spec.AddressGroup.Name,
				formatPorts(obj.Spec.Ports),
				obj.Spec.Priority,
				translateTimestampSince(obj.CreationTimestamp),
			},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *netguardv1beta1.IEAgAgRule:
		addRow(v)
	case *netguardv1beta1.IEAgAgRuleList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}
	return table, nil
}

// helpers
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
	_ rest.Storage         = &IEAgAgRuleStorage{}
	_ rest.Getter          = &IEAgAgRuleStorage{}
	_ rest.Lister          = &IEAgAgRuleStorage{}
	_ rest.Watcher         = &IEAgAgRuleStorage{}
	_ rest.Creater         = &IEAgAgRuleStorage{}
	_ rest.Updater         = &IEAgAgRuleStorage{}
	_ rest.GracefulDeleter = &IEAgAgRuleStorage{}
	_ rest.Patcher         = &IEAgAgRuleStorage{}
)
