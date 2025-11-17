package svcfqdn_rule

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

// SvcFqdnRuleStorage provides REST storage for SvcFqdnRule resources using BaseStorage helpers.
type SvcFqdnRuleStorage struct {
	*base.BaseStorage[*netguardv1beta1.SvcFqdnRule, *models.SvcFqdnRule]
}

// NewSvcFqdnRuleStorage constructs storage backed by backend gRPC client and shared BaseStorage wiring.
func NewSvcFqdnRuleStorage(backendClient client.BackendClient) *SvcFqdnRuleStorage {
	converter := convert.NewSvcFqdnRuleConverter()
	validator := validation.NewSvcFqdnRuleValidator()
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)
	backendOps := base.NewSvcFqdnRulePtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.SvcFqdnRule, *models.SvcFqdnRule](
		func() *netguardv1beta1.SvcFqdnRule { return &netguardv1beta1.SvcFqdnRule{} },
		func() runtime.Object { return &netguardv1beta1.SvcFqdnRuleList{} },
		backendOps,
		backendClient,
		converter,
		validator,
		watcher,
		"svcfqdnrules",
		"SvcFqdnRule",
		true,
	)

	return &SvcFqdnRuleStorage{BaseStorage: baseStorage}
}

// GetSingularName returns resource singular string used by kubectl table printers.
func (s *SvcFqdnRuleStorage) GetSingularName() string {
	return "svcfqdnrule"
}

// ConvertToTable renders human friendly table view for kubectl get.
func (s *SvcFqdnRuleStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "ServiceFrom", Type: "string"},
			{Name: "FQDN", Type: "string"},
			{Name: "Transport", Type: "string"},
			{Name: "Action", Type: "string"},
			{Name: "Priority", Type: "integer"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(rule *netguardv1beta1.SvcFqdnRule) {
		serviceFrom := fmt.Sprintf("%s/%s", rule.Spec.ServiceFrom.Namespace, rule.Spec.ServiceFrom.Name)
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: rule},
			Cells: []interface{}{
				rule.Name,
				serviceFrom,
				rule.Spec.FQDN,
				string(rule.Spec.Transport),
				string(rule.Spec.Action),
				rule.Spec.Priority,
				translateTimestampSince(rule.CreationTimestamp),
			},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *netguardv1beta1.SvcFqdnRule:
		addRow(v)
	case *netguardv1beta1.SvcFqdnRuleList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}

	return table, nil
}

// DeleteCollection implements bulk deletion in namespace respecting validation hook.
func (s *SvcFqdnRuleStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	obj, err := s.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	list, ok := obj.(*netguardv1beta1.SvcFqdnRuleList)
	if !ok {
		return nil, fmt.Errorf("unexpected object type from List: %T", obj)
	}

	deleted := &netguardv1beta1.SvcFqdnRuleList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SvcFqdnRuleList",
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

		if _, _, err := s.Delete(ctx, item.Name, deleteValidation, options); err != nil {
			return nil, fmt.Errorf("failed to delete svcfqdnrule %s: %w", item.Name, err)
		}
		deleted.Items = append(deleted.Items, *item)
	}

	return deleted, nil
}

// translateTimestampSince renders time difference similar to kubectl output.
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

// Kind returns resource kind for rest.KindProvider implementation.
func (s *SvcFqdnRuleStorage) Kind() string {
	return "SvcFqdnRule"
}

var _ rest.CollectionDeleter = &SvcFqdnRuleStorage{}
