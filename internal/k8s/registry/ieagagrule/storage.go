package ieagagrule

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/base"
	"netguard-pg-backend/internal/k8s/registry/convert"
	"netguard-pg-backend/internal/k8s/registry/validation"
)

// IEAgAgRuleStorage implements REST storage for IEAgAgRule resources using BaseStorage
type IEAgAgRuleStorage struct {
	*base.BaseStorage[*netguardv1beta1.IEAgAgRule, *models.IEAgAgRule]
}

// NewIEAgAgRuleStorage creates a new IEAgAgRuleStorage using BaseStorage
func NewIEAgAgRuleStorage(backendClient client.BackendClient) *IEAgAgRuleStorage {
	converter := &convert.IEAgAgRuleConverter{}
	validator := &validation.IEAgAgRuleValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use factory to create backend operations adapter
	backendOps := base.NewIEAgAgRulePtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.IEAgAgRule, *models.IEAgAgRule](
		func() *netguardv1beta1.IEAgAgRule { return &netguardv1beta1.IEAgAgRule{} },
		func() runtime.Object { return &netguardv1beta1.IEAgAgRuleList{} },
		backendOps,
		converter,
		validator,
		watcher,
		"ieagagrules",
		"IEAgAgRule",
		true, // namespace scoped
	)

	return &IEAgAgRuleStorage{
		BaseStorage: baseStorage,
	}
}

// GetSingularName returns the singular name for the resource
func (s *IEAgAgRuleStorage) GetSingularName() string {
	return "ieagagrule"
}

// ConvertToTable provides a minimal table representation
func (s *IEAgAgRuleStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Traffic", Type: "string"},
			{Name: "Action", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(rule *netguardv1beta1.IEAgAgRule) {
		traffic := string(rule.Spec.Traffic)
		action := string(rule.Spec.Action)
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: rule},
			Cells:  []interface{}{rule.Name, traffic, action, translateTimestampSince(rule.CreationTimestamp)},
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

// DeleteCollection implements rest.CollectionDeleter
func (s *IEAgAgRuleStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	obj, err := s.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	list, ok := obj.(*netguardv1beta1.IEAgAgRuleList)
	if !ok {
		return nil, fmt.Errorf("unexpected object type from List: %T", obj)
	}

	deletedItems := &netguardv1beta1.IEAgAgRuleList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IEAgAgRuleList",
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
		},
	}

	for i := range list.Items {
		item := &list.Items[i]

		if deleteValidation != nil {
			if err := deleteValidation(ctx, item); err != nil {
				return nil, err
			}
		}

		_, _, err := s.Delete(ctx, item.Name, deleteValidation, options)
		if err != nil {
			return nil, fmt.Errorf("failed to delete ieagagrule %s: %w", item.Name, err)
		}

		deletedItems.Items = append(deletedItems.Items, *item)
	}

	return deletedItems, nil
}

// Kind implements rest.KindProvider
func (s *IEAgAgRuleStorage) Kind() string {
	return "IEAgAgRule"
}

var _ rest.CollectionDeleter = &IEAgAgRuleStorage{}
