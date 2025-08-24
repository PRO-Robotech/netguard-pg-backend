package rules2s

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

// RuleS2SStorage implements REST storage for RuleS2S resources using BaseStorage
type RuleS2SStorage struct {
	*base.BaseStorage[*netguardv1beta1.RuleS2S, *models.RuleS2S]
}

// NewRuleS2SStorage creates a new RuleS2SStorage using BaseStorage
func NewRuleS2SStorage(backendClient client.BackendClient) *RuleS2SStorage {
	converter := &convert.RuleS2SConverter{}
	validator := &validation.RuleS2SValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use factory to create backend operations adapter
	backendOps := base.NewRuleS2SPtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.RuleS2S, *models.RuleS2S](
		func() *netguardv1beta1.RuleS2S { return &netguardv1beta1.RuleS2S{} },
		func() runtime.Object { return &netguardv1beta1.RuleS2SList{} },
		backendOps,
		converter,
		validator,
		watcher,
		"rules2s",
		"RuleS2S",
		true, // namespace scoped
	)

	return &RuleS2SStorage{
		BaseStorage: baseStorage,
	}
}

// GetSingularName returns the singular name for the resource
func (s *RuleS2SStorage) GetSingularName() string {
	return "rules2s"
}

// ConvertToTable provides a minimal table representation
func (s *RuleS2SStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Traffic", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	addRow := func(rule *netguardv1beta1.RuleS2S) {
		traffic := string(rule.Spec.Traffic)
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: rule},
			Cells:  []interface{}{rule.Name, traffic, translateTimestampSince(rule.CreationTimestamp)},
		}
		table.Rows = append(table.Rows, row)
	}

	switch v := object.(type) {
	case *netguardv1beta1.RuleS2S:
		addRow(v)
	case *netguardv1beta1.RuleS2SList:
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
