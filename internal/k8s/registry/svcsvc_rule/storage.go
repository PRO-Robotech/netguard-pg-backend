package svcsvc_rule

import (
	"context"
	"fmt"

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
	tableutils "netguard-pg-backend/internal/k8s/registry/utils"
	"netguard-pg-backend/internal/k8s/registry/validation"
)

// SvcSvcRuleStorage implements REST storage for SvcSvcRule resources using BaseStorage
type SvcSvcRuleStorage struct {
	*base.BaseStorage[*netguardv1beta1.SvcSvcRule, *models.SvcSvcRule]
}

// NewSvcSvcRuleStorage creates a new SvcSvcRuleStorage using BaseStorage
func NewSvcSvcRuleStorage(backendClient client.BackendClient) *SvcSvcRuleStorage {
	converter := &convert.SvcSvcRuleConverter{}
	validator := &validation.SvcSvcRuleValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use factory to create backend operations adapter
	backendOps := base.NewSvcSvcRulePtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.SvcSvcRule, *models.SvcSvcRule](
		func() *netguardv1beta1.SvcSvcRule { return &netguardv1beta1.SvcSvcRule{} },
		func() runtime.Object { return &netguardv1beta1.SvcSvcRuleList{} },
		backendOps,
		backendClient,
		converter,
		validator,
		watcher,
		"svcsvcrules",
		"SvcSvcRule",
		true, // namespace scoped
	)

	return &SvcSvcRuleStorage{
		BaseStorage: baseStorage,
	}
}

// GetSingularName returns the singular name for the resource
func (s *SvcSvcRuleStorage) GetSingularName() string {
	return "svcsvcrule"
}

// ConvertToTable provides a minimal table representation
func (s *SvcSvcRuleStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := tableutils.NewTable(
		metav1.TableColumnDefinition{Name: "Name", Type: "string", Format: "name"},
		metav1.TableColumnDefinition{Name: "ServiceFrom", Type: "string"},
		metav1.TableColumnDefinition{Name: "ServiceTo", Type: "string"},
		metav1.TableColumnDefinition{Name: "Action", Type: "string"},
		metav1.TableColumnDefinition{Name: "Priority", Type: "integer"},
		metav1.TableColumnDefinition{Name: "Age", Type: "string"},
	)
	if tableutils.AppendBookmarkRowIfNeeded(table, object) {
		return table, nil
	}

	addRow := func(rule *netguardv1beta1.SvcSvcRule) {
		serviceFrom := fmt.Sprintf("%s/%s", rule.Spec.ServiceFrom.Namespace, rule.Spec.ServiceFrom.Name)
		serviceTo := fmt.Sprintf("%s/%s", rule.Spec.ServiceTo.Namespace, rule.Spec.ServiceTo.Name)
		action := string(rule.Spec.Action)
		tableutils.AppendRow(table, rule,
			rule.Name,
			serviceFrom,
			serviceTo,
			action,
			rule.Spec.Priority,
			tableutils.TranslateTimestampSince(rule.CreationTimestamp),
		)
	}

	switch v := object.(type) {
	case *netguardv1beta1.SvcSvcRule:
		addRow(v)
	case *netguardv1beta1.SvcSvcRuleList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}
	return table, nil
}

// DeleteCollection implements rest.CollectionDeleter
func (s *SvcSvcRuleStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	obj, err := s.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	list, ok := obj.(*netguardv1beta1.SvcSvcRuleList)
	if !ok {
		return nil, fmt.Errorf("unexpected object type from List: %T", obj)
	}

	deletedItems := &netguardv1beta1.SvcSvcRuleList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SvcSvcRuleList",
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
			return nil, fmt.Errorf("failed to delete svcsvcrule %s: %w", item.Name, err)
		}

		deletedItems.Items = append(deletedItems.Items, *item)
	}

	return deletedItems, nil
}

// Kind implements rest.KindProvider
func (s *SvcSvcRuleStorage) Kind() string {
	return "SvcSvcRule"
}

var _ rest.CollectionDeleter = &SvcSvcRuleStorage{}
