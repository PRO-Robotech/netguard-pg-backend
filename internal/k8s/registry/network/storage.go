package network

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

// NetworkConverterAdapter adapts NetworkConverter to BaseStorage interface
type NetworkConverterAdapter struct {
	*convert.NetworkConverter
}

func NewNetworkConverterAdapter() *NetworkConverterAdapter {
	return &NetworkConverterAdapter{
		NetworkConverter: convert.NewNetworkConverter(),
	}
}

func (a *NetworkConverterAdapter) ToList(ctx context.Context, domainObjs []*models.Network) (runtime.Object, error) {
	return a.NetworkConverter.ToList(ctx, domainObjs)
}

// NetworkStorage implements REST storage for Network resources using BaseStorage
type NetworkStorage struct {
	*base.BaseStorage[*netguardv1beta1.Network, *models.Network]
}

// Compile-time interface assertions
var _ rest.TableConvertor = &NetworkStorage{}

// NewNetworkStorage creates a new NetworkStorage using BaseStorage with NetguardService
func NewNetworkStorage(netguardService *services.NetguardFacade) *NetworkStorage {
	converter := NewNetworkConverterAdapter()
	validator := validation.NewNetworkValidator()
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use NetguardService operations adapter (with ConditionManager)
	backendOps := base.NewNetworkPtrOpsWithNetguardService(netguardService)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.Network, *models.Network](
		func() *netguardv1beta1.Network { return &netguardv1beta1.Network{} },
		func() runtime.Object { return &netguardv1beta1.NetworkList{} },
		backendOps,
		converter,
		validator,
		watcher,
		"networks",
		"Network",
		true, // namespace scoped
	)

	return &NetworkStorage{
		BaseStorage: baseStorage,
	}
}

// NewNetworkStorageWithClient creates a new NetworkStorage using old client-based approach (DEPRECATED)
func NewNetworkStorageWithClient(backendClient client.BackendClient) *NetworkStorage {
	converter := NewNetworkConverterAdapter()
	validator := validation.NewNetworkValidator()
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use old client-based backend operations (NO ConditionManager)
	backendOps := base.NewNetworkPtrOpsOld(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.Network, *models.Network](
		func() *netguardv1beta1.Network { return &netguardv1beta1.Network{} },
		func() runtime.Object { return &netguardv1beta1.NetworkList{} },
		backendOps,
		converter,
		validator,
		watcher,
		"networks",
		"Network",
		true, // namespace scoped
	)

	return &NetworkStorage{
		BaseStorage: baseStorage,
	}
}

// GetSingularName returns the singular name of the resource
func (s *NetworkStorage) GetSingularName() string {
	return "network"
}

// ConvertToTable implements minimal table output so kubectl can display resources.
func (s *NetworkStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "CIDR", Type: "string"},
			{Name: "Network Name", Type: "string"},
			{Name: "Bound", Type: "boolean"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(network *netguardv1beta1.Network) {
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: network},
			Cells: []interface{}{
				network.Name,
				network.Spec.CIDR,
				network.Status.NetworkName,
				network.Status.IsBound,
				networkTranslateTimestampSince(network.CreationTimestamp),
			},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *netguardv1beta1.Network:
		addRow(v)
	case *netguardv1beta1.NetworkList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}
	return table, nil
}

// helper function for network storage
func networkTranslateTimestampSince(ts metav1.Time) string {
	if ts.IsZero() {
		return "<unknown>"
	}
	return networkDurationShortHumanDuration(time.Since(ts.Time))
}

func networkDurationShortHumanDuration(d time.Duration) string {
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
