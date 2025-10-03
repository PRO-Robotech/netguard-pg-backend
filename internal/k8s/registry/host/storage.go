package host

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

// HostConverterAdapter adapts HostConverter to BaseStorage interface
type HostConverterAdapter struct {
	*convert.HostConverter
}

func NewHostConverterAdapter() *HostConverterAdapter {
	return &HostConverterAdapter{
		HostConverter: &convert.HostConverter{},
	}
}

func (a *HostConverterAdapter) ToList(ctx context.Context, domainObjs []*models.Host) (runtime.Object, error) {
	if domainObjs == nil {
		return &netguardv1beta1.HostList{}, nil
	}

	k8sObjs := make([]netguardv1beta1.Host, 0, len(domainObjs))
	for _, domainObj := range domainObjs {
		if domainObj == nil {
			continue
		}
		k8sObj, err := a.HostConverter.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain Host to k8s: %w", err)
		}
		k8sObjs = append(k8sObjs, *k8sObj)
	}

	return &netguardv1beta1.HostList{
		Items: k8sObjs,
	}, nil
}

// HostStorage implements REST storage for Host resources using BaseStorage
type HostStorage struct {
	*base.BaseStorage[*netguardv1beta1.Host, *models.Host]
}

// NewHostStorage creates a new HostStorage using BaseStorage
func NewHostStorage(backendClient client.BackendClient) *HostStorage {
	converter := NewHostConverterAdapter()
	validator := &validation.HostValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use factory to create backend operations adapter
	backendOps := base.NewHostPtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.Host, *models.Host](
		func() *netguardv1beta1.Host { return &netguardv1beta1.Host{} },
		func() runtime.Object { return &netguardv1beta1.HostList{} },
		backendOps,
		converter,
		validator,
		watcher,
		"hosts",
		"Host",
		true, // namespace scoped
	)

	storage := &HostStorage{
		BaseStorage: baseStorage,
	}

	return storage
}

// handleHostCreate implements custom logic when a Host is created
func (s *HostStorage) handleHostCreate(ctx context.Context, obj *netguardv1beta1.Host, domainObj *models.Host) error {

	// Initialize Host as unbound initially
	obj.Status.IsBound = false
	obj.Status.BindingRef = nil
	obj.Status.AddressGroupRef = nil
	obj.Status.AddressGroupName = ""

	return nil
}

// handleHostUpdate implements custom logic when a Host is updated
func (s *HostStorage) handleHostUpdate(ctx context.Context, obj, oldObj *netguardv1beta1.Host, domainObj *models.Host) error {

	return nil
}

// handleHostDelete implements custom logic when a Host is deleted
func (s *HostStorage) handleHostDelete(ctx context.Context, obj *netguardv1beta1.Host, domainObj *models.Host) error {

	return nil
}

// GetSingularName returns the singular name for this resource
func (s *HostStorage) GetSingularName() string {
	return "host"
}

// ConvertToTable implements minimal table output so kubectl can display resources.
func (s *HostStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "UUID", Type: "string"},
			{Name: "Bound", Type: "boolean"},
			{Name: "AddressGroup", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(host *netguardv1beta1.Host) {
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: host},
			Cells: []interface{}{
				host.Name,
				host.Spec.UUID,
				host.Status.IsBound,
				host.Status.AddressGroupName,
				translateTimestampSince(host.CreationTimestamp),
			},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *netguardv1beta1.Host:
		addRow(v)
	case *netguardv1beta1.HostList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}
	return table, nil
}

// DeleteCollection implements rest.CollectionDeleter
func (s *HostStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	obj, err := s.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	hostList, ok := obj.(*netguardv1beta1.HostList)
	if !ok {
		return nil, fmt.Errorf("unexpected object type from List: %T", obj)
	}

	deletedItems := &netguardv1beta1.HostList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HostList",
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
		},
	}

	for i := range hostList.Items {
		host := &hostList.Items[i]

		if deleteValidation != nil {
			if err := deleteValidation(ctx, host); err != nil {
				return nil, err
			}
		}

		_, _, err := s.Delete(ctx, host.Name, deleteValidation, options)
		if err != nil {
			return nil, fmt.Errorf("failed to delete host %s: %w", host.Name, err)
		}

		deletedItems.Items = append(deletedItems.Items, *host)
	}

	return deletedItems, nil
}

// Kind implements rest.KindProvider
func (s *HostStorage) Kind() string {
	return "Host"
}

// translateTimestampSince returns the elapsed time since timestamp in human-readable form.
func translateTimestampSince(ts metav1.Time) string {
	if ts.IsZero() {
		return "<unknown>"
	}
	return durationShortHumanDuration(time.Since(ts.Time))
}

// durationShortHumanDuration is a copy of kubectl printing helper (short).
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

// Ensure HostStorage implements the required interfaces
var _ rest.StandardStorage = &HostStorage{}
var _ rest.KindProvider = &HostStorage{}
var _ rest.SingularNameProvider = &HostStorage{}
