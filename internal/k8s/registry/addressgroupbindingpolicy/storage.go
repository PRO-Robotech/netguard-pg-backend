package addressgroupbindingpolicy

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

// AddressGroupBindingPolicyStorage implements REST storage for AddressGroupBindingPolicy resources using BaseStorage
type AddressGroupBindingPolicyStorage struct {
	*base.BaseStorage[*netguardv1beta1.AddressGroupBindingPolicy, *models.AddressGroupBindingPolicy]
}

// NewAddressGroupBindingPolicyStorage creates a new AddressGroupBindingPolicyStorage using BaseStorage
func NewAddressGroupBindingPolicyStorage(backendClient client.BackendClient) *AddressGroupBindingPolicyStorage {
	converter := &convert.AddressGroupBindingPolicyConverter{}
	validator := &validation.AddressGroupBindingPolicyValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use factory to create backend operations adapter
	backendOps := base.NewAddressGroupBindingPolicyPtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.AddressGroupBindingPolicy, *models.AddressGroupBindingPolicy](
		func() *netguardv1beta1.AddressGroupBindingPolicy { return &netguardv1beta1.AddressGroupBindingPolicy{} },
		func() runtime.Object { return &netguardv1beta1.AddressGroupBindingPolicyList{} },
		backendOps,
		backendClient,
		converter,
		validator,
		watcher,
		"addressgroupbindingpolicies",
		"AddressGroupBindingPolicy",
		true, // namespace scoped
	)

	return &AddressGroupBindingPolicyStorage{
		BaseStorage: baseStorage,
	}
}

// GetSingularName returns the singular name for the resource
func (s *AddressGroupBindingPolicyStorage) GetSingularName() string {
	return "addressgroupbindingpolicy"
}

// ConvertToTable provides a minimal table representation
func (s *AddressGroupBindingPolicyStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := tableutils.NewTable(
		metav1.TableColumnDefinition{Name: "Name", Type: "string", Format: "name"},
		metav1.TableColumnDefinition{Name: "Age", Type: "string"},
	)
	if tableutils.AppendBookmarkRowIfNeeded(table, object) {
		return table, nil
	}

	addRow := func(policy *netguardv1beta1.AddressGroupBindingPolicy) {
		tableutils.AppendRow(table, policy,
			policy.Name,
			tableutils.TranslateTimestampSince(policy.CreationTimestamp),
		)
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

// DeleteCollection implements rest.CollectionDeleter
func (s *AddressGroupBindingPolicyStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	obj, err := s.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	list, ok := obj.(*netguardv1beta1.AddressGroupBindingPolicyList)
	if !ok {
		return nil, fmt.Errorf("unexpected object type from List: %T", obj)
	}

	deletedItems := &netguardv1beta1.AddressGroupBindingPolicyList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AddressGroupBindingPolicyList",
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
			return nil, fmt.Errorf("failed to delete addressgroupbindingpolicy %s: %w", item.Name, err)
		}

		deletedItems.Items = append(deletedItems.Items, *item)
	}

	return deletedItems, nil
}

// Kind implements rest.KindProvider
func (s *AddressGroupBindingPolicyStorage) Kind() string {
	return "AddressGroupBindingPolicy"
}

var _ rest.CollectionDeleter = &AddressGroupBindingPolicyStorage{}
