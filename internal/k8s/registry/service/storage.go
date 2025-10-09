package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/base"
	"netguard-pg-backend/internal/k8s/registry/convert"
	"netguard-pg-backend/internal/k8s/registry/validation"
)

// ServiceStorage implements REST storage for Service resources using BaseStorage
type ServiceStorage struct {
	*base.BaseStorage[*netguardv1beta1.Service, *models.Service]
}

// Patch delegates to BaseStorage
func (s *ServiceStorage) Patch(ctx context.Context, name string, patchType types.PatchType, data []byte, options *metav1.PatchOptions, subresources ...string) (runtime.Object, error) {
	klog.V(4).InfoS("ServiceStorage.Patch called",
		"name", name,
		"patchType", string(patchType),
		"subresources", subresources)

	return s.BaseStorage.Patch(ctx, name, patchType, data, options, subresources...)
}

// Get delegates to BaseStorage
func (s *ServiceStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	result, err := s.BaseStorage.Get(ctx, name, options)
	if err != nil {
		klog.V(4).InfoS("ServiceStorage.Get failed", "name", name, "error", err.Error())
	}
	return result, err
}

// Update delegates to BaseStorage
func (s *ServiceStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	result, created, err := s.BaseStorage.Update(ctx, name, objInfo, createValidation, updateValidation, forceAllowCreate, options)
	if err != nil {
		klog.V(4).InfoS("ServiceStorage.Update failed", "name", name, "error", err.Error())
	}
	return result, created, err
}

// Compile-time interface assertions
var _ rest.TableConvertor = &ServiceStorage{}
var _ rest.CollectionDeleter = &ServiceStorage{}

// NewServiceStorage creates a new ServiceStorage using BaseStorage with direct client (like AddressGroup)
func NewServiceStorage(backendClient client.BackendClient) *ServiceStorage {
	converter := &convert.ServiceConverter{}
	validator := &validation.ServiceValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use direct client operations (same pattern as AddressGroup)
	backendOps := base.NewServicePtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.Service, *models.Service](
		func() *netguardv1beta1.Service { return &netguardv1beta1.Service{} },
		func() runtime.Object { return &netguardv1beta1.ServiceList{} },
		backendOps,
		converter,
		validator,
		watcher,
		"services",
		"Service",
		true, // namespace scoped
	)

	storage := &ServiceStorage{
		BaseStorage: baseStorage,
	}

	return storage
}

// NewServiceStorageWithClient creates a new ServiceStorage using old client-based approach (DEPRECATED)
func NewServiceStorageWithClient(backendClient client.BackendClient) *ServiceStorage {
	converter := &convert.ServiceConverter{}
	validator := &validation.ServiceValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use old client-based backend operations (NO ConditionManager)
	backendOps := base.NewServicePtrOpsOld(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.Service, *models.Service](
		func() *netguardv1beta1.Service { return &netguardv1beta1.Service{} },
		func() runtime.Object { return &netguardv1beta1.ServiceList{} },
		backendOps,
		converter,
		validator,
		watcher,
		"services",
		"Service",
		true, // namespace scoped
	)

	storage := &ServiceStorage{
		BaseStorage: baseStorage,
	}

	return storage
}

// GetSingularName returns the singular name of the resource
func (s *ServiceStorage) GetSingularName() string {
	return "service"
}

// ConvertToTable implements minimal table output so kubectl can display resources.
func (s *ServiceStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Ports", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(svc *netguardv1beta1.Service) {
		ports := make([]string, 0, len(svc.Spec.IngressPorts))
		for _, p := range svc.Spec.IngressPorts {
			ports = append(ports, fmt.Sprintf("%s/%s", p.Protocol, p.Port))
		}
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: svc},
			Cells:  []interface{}{svc.Name, strings.Join(ports, ","), translateTimestampSince(svc.CreationTimestamp)},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *netguardv1beta1.Service:
		addRow(v)
	case *netguardv1beta1.ServiceList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}
	return table, nil
}

// helper similar to addressgroup
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

// DeleteCollection implements rest.CollectionDeleter
func (s *ServiceStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	obj, err := s.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	list, ok := obj.(*netguardv1beta1.ServiceList)
	if !ok {
		return nil, fmt.Errorf("unexpected object type from List: %T", obj)
	}

	deletedItems := &netguardv1beta1.ServiceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceList",
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
			return nil, fmt.Errorf("failed to delete service %s: %w", item.Name, err)
		}

		deletedItems.Items = append(deletedItems.Items, *item)
	}

	return deletedItems, nil
}

// Kind implements rest.KindProvider
func (s *ServiceStorage) Kind() string {
	return "Service"
}
