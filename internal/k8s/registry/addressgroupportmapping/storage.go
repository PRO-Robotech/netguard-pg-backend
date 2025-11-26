package addressgroupportmapping

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

// AddressGroupPortMappingStorage implements REST storage for AddressGroupPortMapping resources using BaseStorage
type AddressGroupPortMappingStorage struct {
	*base.BaseStorage[*netguardv1beta1.AddressGroupPortMapping, *models.AddressGroupPortMapping]
}

// NewAddressGroupPortMappingStorage creates a new AddressGroupPortMappingStorage using BaseStorage
func NewAddressGroupPortMappingStorage(backendClient client.BackendClient) *AddressGroupPortMappingStorage {
	converter := &convert.AddressGroupPortMappingConverter{}
	validator := &validation.AddressGroupPortMappingValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use factory to create backend operations adapter
	backendOps := base.NewAddressGroupPortMappingPtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.AddressGroupPortMapping, *models.AddressGroupPortMapping](
		func() *netguardv1beta1.AddressGroupPortMapping { return &netguardv1beta1.AddressGroupPortMapping{} },
		func() runtime.Object { return &netguardv1beta1.AddressGroupPortMappingList{} },
		backendOps,
		backendClient,
		converter,
		validator,
		watcher,
		"addressgroupportmappings",
		"AddressGroupPortMapping",
		true, // namespace scoped
	)

	return &AddressGroupPortMappingStorage{
		BaseStorage: baseStorage,
	}
}

// GetSingularName returns the singular name for the resource
func (s *AddressGroupPortMappingStorage) GetSingularName() string {
	return "addressgroupportmapping"
}

// ConvertToTable provides a minimal table representation
func (s *AddressGroupPortMappingStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := tableutils.NewTable(
		metav1.TableColumnDefinition{Name: "Name", Type: "string", Format: "name"},
		metav1.TableColumnDefinition{Name: "Age", Type: "string"},
	)
	if tableutils.AppendBookmarkRowIfNeeded(table, object) {
		return table, nil
	}

	addRow := func(mapping *netguardv1beta1.AddressGroupPortMapping) {
		tableutils.AppendRow(table, mapping,
			mapping.Name,
			tableutils.TranslateTimestampSince(mapping.CreationTimestamp),
		)
	}

	switch v := object.(type) {
	case *netguardv1beta1.AddressGroupPortMapping:
		addRow(v)
	case *netguardv1beta1.AddressGroupPortMappingList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}
	return table, nil
}

// DeleteCollection implements rest.CollectionDeleter
func (s *AddressGroupPortMappingStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	obj, err := s.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	list, ok := obj.(*netguardv1beta1.AddressGroupPortMappingList)
	if !ok {
		return nil, fmt.Errorf("unexpected object type from List: %T", obj)
	}

	deletedItems := &netguardv1beta1.AddressGroupPortMappingList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AddressGroupPortMappingList",
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
			return nil, fmt.Errorf("failed to delete addressgroupportmapping %s: %w", item.Name, err)
		}

		deletedItems.Items = append(deletedItems.Items, *item)
	}

	return deletedItems, nil
}

// Kind implements rest.KindProvider
func (s *AddressGroupPortMappingStorage) Kind() string {
	return "AddressGroupPortMapping"
}

var _ rest.CollectionDeleter = &AddressGroupPortMappingStorage{}
