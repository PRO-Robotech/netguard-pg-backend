package rules2s

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

// RuleS2SStorage implements REST storage for RuleS2S resources using BaseStorage
type RuleS2SStorage struct {
	*base.BaseStorage[*netguardv1beta1.RuleS2S, *models.RuleS2S]
}

// NewRuleS2SStorage creates a new RuleS2SStorage using BaseStorage
func NewRuleS2SStorage(backendClient client.BackendClient) *RuleS2SStorage {
	converter := &convert.RuleS2SConverter{}
	validator := &validation.RuleS2SValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use factory to create backend operations adapter
	backendOps := base.NewRuleS2SPtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.RuleS2S, *models.RuleS2S](
		func() *netguardv1beta1.RuleS2S { return &netguardv1beta1.RuleS2S{} },
		func() runtime.Object { return &netguardv1beta1.RuleS2SList{} },
		backendOps,
		backendClient,
		converter,
		validator,
		watcher,
		"rules2s",
		"RuleS2S",
		true, // namespace scoped
	)

	return &RuleS2SStorage{
		BaseStorage: baseStorage,
	}
}

// GetSingularName returns the singular name for the resource
func (s *RuleS2SStorage) GetSingularName() string {
	return "rules2s"
}

// ConvertToTable provides a minimal table representation
func (s *RuleS2SStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := tableutils.NewTable(
		metav1.TableColumnDefinition{Name: "Name", Type: "string", Format: "name"},
		metav1.TableColumnDefinition{Name: "Traffic", Type: "string"},
		metav1.TableColumnDefinition{Name: "Age", Type: "string"},
	)
	if tableutils.AppendBookmarkRowIfNeeded(table, object) {
		return table, nil
	}

	addRow := func(rule *netguardv1beta1.RuleS2S) {
		traffic := string(rule.Spec.Traffic)
		tableutils.AppendRow(table, rule,
			rule.Name,
			traffic,
			tableutils.TranslateTimestampSince(rule.CreationTimestamp),
		)
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

// DeleteCollection implements rest.CollectionDeleter
func (s *RuleS2SStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	obj, err := s.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	list, ok := obj.(*netguardv1beta1.RuleS2SList)
	if !ok {
		return nil, fmt.Errorf("unexpected object type from List: %T", obj)
	}

	deletedItems := &netguardv1beta1.RuleS2SList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RuleS2SList",
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
			return nil, fmt.Errorf("failed to delete rules2s %s: %w", item.Name, err)
		}

		deletedItems.Items = append(deletedItems.Items, *item)
	}

	return deletedItems, nil
}

// Kind implements rest.KindProvider
func (s *RuleS2SStorage) Kind() string {
	return "RuleS2S"
}

var _ rest.CollectionDeleter = &RuleS2SStorage{}
