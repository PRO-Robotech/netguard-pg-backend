package network_binding

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/base"
	"netguard-pg-backend/internal/k8s/registry/convert"
	"netguard-pg-backend/internal/k8s/registry/validation"

	"k8s.io/apiserver/pkg/registry/rest"
)

// NetworkBindingConverterAdapter adapts NetworkBindingConverter to BaseStorage interface
type NetworkBindingConverterAdapter struct {
	*convert.NetworkBindingConverter
}

func NewNetworkBindingConverterAdapter() *NetworkBindingConverterAdapter {
	return &NetworkBindingConverterAdapter{
		NetworkBindingConverter: convert.NewNetworkBindingConverter(),
	}
}

func (a *NetworkBindingConverterAdapter) ToList(ctx context.Context, domainObjs []*models.NetworkBinding) (runtime.Object, error) {
	return a.NetworkBindingConverter.ToList(ctx, domainObjs)
}

// NetworkBindingStorage implements REST storage for NetworkBinding resources using BaseStorage
type NetworkBindingStorage struct {
	*base.BaseStorage[*netguardv1beta1.NetworkBinding, *models.NetworkBinding]
}

// Compile-time interface assertions
var _ rest.TableConvertor = &NetworkBindingStorage{}

// NewNetworkBindingStorage creates a new NetworkBindingStorage using BaseStorage with NetguardService
func NewNetworkBindingStorage(netguardService *services.NetguardFacade) *NetworkBindingStorage {
	converter := NewNetworkBindingConverterAdapter()
	validator := validation.NewNetworkBindingValidator()
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use NetguardService operations adapter (with ConditionManager)
	backendOps := base.NewNetworkBindingPtrOpsWithNetguardService(netguardService)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.NetworkBinding, *models.NetworkBinding](
		func() *netguardv1beta1.NetworkBinding { return &netguardv1beta1.NetworkBinding{} },
		func() runtime.Object { return &netguardv1beta1.NetworkBindingList{} },
		backendOps,
		converter,
		validator,
		watcher,
		"networkbindings",
		"NetworkBinding",
		true, // namespace scoped
	)

	return &NetworkBindingStorage{
		BaseStorage: baseStorage,
	}
}

// NewNetworkBindingStorageWithClient creates a new NetworkBindingStorage using old client-based approach (DEPRECATED)
func NewNetworkBindingStorageWithClient(backendClient client.BackendClient) *NetworkBindingStorage {
	converter := NewNetworkBindingConverterAdapter()
	validator := validation.NewNetworkBindingValidator()
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use old client-based backend operations (NO ConditionManager)
	backendOps := base.NewNetworkBindingPtrOpsOld(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.NetworkBinding, *models.NetworkBinding](
		func() *netguardv1beta1.NetworkBinding { return &netguardv1beta1.NetworkBinding{} },
		func() runtime.Object { return &netguardv1beta1.NetworkBindingList{} },
		backendOps,
		converter,
		validator,
		watcher,
		"networkbindings",
		"NetworkBinding",
		true, // namespace scoped
	)

	return &NetworkBindingStorage{
		BaseStorage: baseStorage,
	}
}

// GetSingularName returns the singular name of the resource
func (s *NetworkBindingStorage) GetSingularName() string {
	return "networkbinding"
}

// ConvertToTable implements minimal table output so kubectl can display resources.
func (s *NetworkBindingStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Network", Type: "string"},
			{Name: "AddressGroup", Type: "string"},
			{Name: "Network Item", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(binding *netguardv1beta1.NetworkBinding) {
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: binding},
			Cells: []interface{}{
				binding.Name,
				binding.Spec.NetworkRef,
				binding.Spec.AddressGroupRef,
				binding.NetworkItem.Name,
				networkBindingTranslateTimestampSince(binding.CreationTimestamp),
			},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *netguardv1beta1.NetworkBinding:
		addRow(v)
	case *netguardv1beta1.NetworkBindingList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}
	return table, nil
}

// helper function for networkbinding storage
func networkBindingTranslateTimestampSince(ts metav1.Time) string {
	if ts.IsZero() {
		return "<unknown>"
	}
	return networkBindingDurationShortHumanDuration(time.Since(ts.Time))
}

func networkBindingDurationShortHumanDuration(d time.Duration) string {
	// Round to the nearest second
	d = d.Round(time.Second)

	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
