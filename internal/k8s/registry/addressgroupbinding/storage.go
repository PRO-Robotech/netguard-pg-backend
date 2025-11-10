package addressgroupbinding

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/base"
	"netguard-pg-backend/internal/k8s/registry/convert"
	"netguard-pg-backend/internal/k8s/registry/validation"

	"k8s.io/apiserver/pkg/registry/rest"
)

// AddressGroupBindingStorage implements REST storage for AddressGroupBinding resources using BaseStorage
type AddressGroupBindingStorage struct {
	*base.BaseStorage[*netguardv1beta1.AddressGroupBinding, *models.AddressGroupBinding]
}

// NewAddressGroupBindingStorage creates a new AddressGroupBindingStorage using BaseStorage
func NewAddressGroupBindingStorage(backendClient client.BackendClient) *AddressGroupBindingStorage {
	converter := &convert.AddressGroupBindingConverter{}
	validator := &validation.AddressGroupBindingValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use factory to create backend operations adapter
	backendOps := base.NewAddressGroupBindingPtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.AddressGroupBinding, *models.AddressGroupBinding](
		func() *netguardv1beta1.AddressGroupBinding { return &netguardv1beta1.AddressGroupBinding{} },
		func() runtime.Object { return &netguardv1beta1.AddressGroupBindingList{} },
		backendOps,
		converter,
		validator,
		watcher,
		"addressgroupbindings",
		"AddressGroupBinding",
		true, // namespace scoped
	)

	return &AddressGroupBindingStorage{
		BaseStorage: baseStorage,
	}
}

// GetSingularName returns the singular name for the resource
func (s *AddressGroupBindingStorage) GetSingularName() string {
	return "addressgroupbinding"
}

// ConvertToTable provides a minimal table representation
func (s *AddressGroupBindingStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Service", Type: "string"},
			{Name: "AddressGroup", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(binding *netguardv1beta1.AddressGroupBinding) {
		service := "unknown"
		if binding.Spec.ServiceRef.Name != "" {
			service = binding.Spec.ServiceRef.Name
		}
		addressGroup := "unknown"
		if binding.Spec.AddressGroupRef.Name != "" {
			addressGroup = binding.Spec.AddressGroupRef.Name
		}
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: binding},
			Cells:  []interface{}{binding.Name, service, addressGroup, translateTimestampSince(binding.CreationTimestamp)},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *netguardv1beta1.AddressGroupBinding:
		addRow(v)
	case *netguardv1beta1.AddressGroupBindingList:
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
func (s *AddressGroupBindingStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	obj, err := s.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	bindingList, ok := obj.(*netguardv1beta1.AddressGroupBindingList)
	if !ok {
		return nil, fmt.Errorf("unexpected object type from List: %T", obj)
	}

	deletedItems := &netguardv1beta1.AddressGroupBindingList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AddressGroupBindingList",
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
		},
	}

	for i := range bindingList.Items {
		binding := &bindingList.Items[i]

		if deleteValidation != nil {
			if err := deleteValidation(ctx, binding); err != nil {
				return nil, err
			}
		}

		_, _, err := s.Delete(ctx, binding.Name, deleteValidation, options)
		if err != nil {
			return nil, fmt.Errorf("failed to delete address group binding %s: %w", binding.Name, err)
		}

		deletedItems.Items = append(deletedItems.Items, *binding)
	}

	return deletedItems, nil
}

// Kind implements rest.KindProvider
func (s *AddressGroupBindingStorage) Kind() string {
	return "AddressGroupBinding"
}

var _ rest.CollectionDeleter = &AddressGroupBindingStorage{}
