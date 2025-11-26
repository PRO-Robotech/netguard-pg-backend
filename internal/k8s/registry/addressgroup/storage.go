package addressgroup

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/base"
	"netguard-pg-backend/internal/k8s/registry/convert"
	tableutils "netguard-pg-backend/internal/k8s/registry/utils"
	"netguard-pg-backend/internal/k8s/registry/validation"

	"k8s.io/apiserver/pkg/registry/rest"
)

// AddressGroupStorage implements REST storage for AddressGroup resources using BaseStorage
type AddressGroupStorage struct {
	*base.BaseStorage[*netguardv1beta1.AddressGroup, *models.AddressGroup]
}

// NewAddressGroupStorage creates a new AddressGroupStorage using BaseStorage
func NewAddressGroupStorage(backendClient client.BackendClient) *AddressGroupStorage {
	converter := &convert.AddressGroupConverter{}
	validator := &validation.AddressGroupValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use factory to create backend operations adapter
	backendOps := base.NewAddressGroupPtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.AddressGroup, *models.AddressGroup](
		func() *netguardv1beta1.AddressGroup { return &netguardv1beta1.AddressGroup{} },
		func() runtime.Object { return &netguardv1beta1.AddressGroupList{} },
		backendOps,
		backendClient,
		converter,
		validator,
		watcher,
		"addressgroups",
		"AddressGroup",
		true, // namespace scoped
	)

	return &AddressGroupStorage{
		BaseStorage: baseStorage,
	}
}

// GetSingularName returns the singular name for the resource
func (s *AddressGroupStorage) GetSingularName() string {
	return "addressgroup"
}

// ConvertToTable provides a minimal table representation so that kubectl
// can print the objects even when "-o wide" или default output запрашивает
// server-side преобразование.
func (s *AddressGroupStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := tableutils.NewTable(
		metav1.TableColumnDefinition{Name: "Name", Type: "string", Format: "name"},
		metav1.TableColumnDefinition{Name: "Default Action", Type: "string"},
		metav1.TableColumnDefinition{Name: "Logs", Type: "boolean"},
		metav1.TableColumnDefinition{Name: "Trace", Type: "boolean"},
		metav1.TableColumnDefinition{Name: "Age", Type: "string"},
	)
	if tableutils.AppendBookmarkRowIfNeeded(table, object) {
		return table, nil
	}

	addRow := func(ag *netguardv1beta1.AddressGroup) {
		tableutils.AppendRow(table, ag,
			ag.Name,
			ag.Spec.DefaultAction,
			ag.Spec.Logs,
			ag.Spec.Trace,
			tableutils.TranslateTimestampSince(ag.CreationTimestamp),
		)
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

// DeleteCollection implements rest.CollectionDeleter
func (s *AddressGroupStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	obj, err := s.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	agList, ok := obj.(*netguardv1beta1.AddressGroupList)
	if !ok {
		return nil, fmt.Errorf("unexpected object type from List: %T", obj)
	}

	deletedItems := &netguardv1beta1.AddressGroupList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AddressGroupList",
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
		},
	}

	for i := range agList.Items {
		ag := &agList.Items[i]

		if deleteValidation != nil {
			if err := deleteValidation(ctx, ag); err != nil {
				return nil, err
			}
		}

		_, _, err := s.Delete(ctx, ag.Name, deleteValidation, options)
		if err != nil {
			return nil, fmt.Errorf("failed to delete address group %s: %w", ag.Name, err)
		}

		deletedItems.Items = append(deletedItems.Items, *ag)
	}

	return deletedItems, nil
}

// Kind implements rest.KindProvider
func (s *AddressGroupStorage) Kind() string {
	return "AddressGroup"
}

var _ rest.CollectionDeleter = &AddressGroupStorage{}
