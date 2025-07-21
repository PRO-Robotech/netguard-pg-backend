package service

import (
	"context"
	"fmt"
	"strings"
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

// ServiceStorage implements REST storage for Service resources using BaseStorage
type ServiceStorage struct {
	*base.BaseStorage[*netguardv1beta1.Service, *models.Service]
}

// Compile-time interface assertions
var _ rest.TableConvertor = &ServiceStorage{}

// NewServiceStorage creates a new ServiceStorage using BaseStorage with NetguardService
func NewServiceStorage(netguardService *services.NetguardService) *ServiceStorage {
	converter := &convert.ServiceConverter{}
	validator := &validation.ServiceValidator{}
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)

	// Use NetguardService operations adapter (with ConditionManager)
	backendOps := base.NewServicePtrOpsWithNetguardService(netguardService)

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

	return &ServiceStorage{
		BaseStorage: baseStorage,
	}
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

	return &ServiceStorage{
		BaseStorage: baseStorage,
	}
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
