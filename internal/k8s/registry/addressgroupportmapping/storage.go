package addressgroupportmapping

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/base"
	"netguard-pg-backend/internal/k8s/registry/convert"
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
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(mapping *netguardv1beta1.AddressGroupPortMapping) {
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: mapping},
			Cells:  []interface{}{mapping.Name, translateTimestampSince(mapping.CreationTimestamp)},
		}
		table.Rows = append(table.Rows, row)
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
