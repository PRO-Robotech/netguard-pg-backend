package addressgroupbindingpolicy

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
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(policy *netguardv1beta1.AddressGroupBindingPolicy) {
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: policy},
			Cells:  []interface{}{policy.Name, translateTimestampSince(policy.CreationTimestamp)},
		}
		table.Rows = append(table.Rows, row)
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
