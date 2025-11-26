package ieagagrule

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
		backendClient,
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
	table := tableutils.NewTable(
		metav1.TableColumnDefinition{Name: "Name", Type: "string", Format: "name"},
		metav1.TableColumnDefinition{Name: "Traffic", Type: "string"},
		metav1.TableColumnDefinition{Name: "Action", Type: "string"},
		metav1.TableColumnDefinition{Name: "Age", Type: "string"},
	)
	if tableutils.AppendBookmarkRowIfNeeded(table, object) {
		return table, nil
	}

	addRow := func(rule *netguardv1beta1.IEAgAgRule) {
		traffic := string(rule.Spec.Traffic)
		action := string(rule.Spec.Action)
		tableutils.AppendRow(table, rule,
			rule.Name,
			traffic,
			action,
			tableutils.TranslateTimestampSince(rule.CreationTimestamp),
		)
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
