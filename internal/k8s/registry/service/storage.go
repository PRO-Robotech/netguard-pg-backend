package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/base"
	"netguard-pg-backend/internal/k8s/registry/convert"
	"netguard-pg-backend/internal/k8s/registry/utils"
	"netguard-pg-backend/internal/k8s/registry/validation"
)

// ServiceStorage implements REST storage for Service resources using BaseStorage
type ServiceStorage struct {
	*base.BaseStorage[*netguardv1beta1.Service, *models.Service]
}

// Patch adds logging to track PATCH requests at ServiceStorage level
func (s *ServiceStorage) Patch(ctx context.Context, name string, patchType types.PatchType, data []byte, options *metav1.PatchOptions, subresources ...string) (runtime.Object, error) {
	klog.InfoS("🔥🔥🔥 ServiceStorage.Patch CALLED - HIGHER LEVEL PATCH ENTRY POINT",
		"name", name,
		"patchType", string(patchType),
		"subresources", subresources)

	// Delegate to BaseStorage
	return s.BaseStorage.Patch(ctx, name, patchType, data, options, subresources...)
}

// Get adds logging to compare with Patch flow
func (s *ServiceStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace := utils.NamespaceFrom(ctx)
	klog.InfoS("🔍🔍🔍 ServiceStorage.Get CALLED - DETAILED ANALYSIS",
		"name", name,
		"namespace", namespace,
		"options", fmt.Sprintf("%+v", options),
		"call_source", "Could be external kubectl OR internal objInfo circular call")

	// Check if this might be the problematic objInfo internal call
	if requestInfo, ok := request.RequestInfoFrom(ctx); ok {
		klog.InfoS("🔍 ServiceStorage.Get - REQUEST CONTEXT ANALYSIS",
			"verb", requestInfo.Verb,
			"resource", requestInfo.Resource,
			"name", requestInfo.Name,
			"namespace", requestInfo.Namespace,
			"apiGroup", requestInfo.APIGroup,
			"apiVersion", requestInfo.APIVersion,
			"isResourceRequest", requestInfo.IsResourceRequest)
	}

	// Check user context to distinguish internal vs external calls
	if userInfo, ok := request.UserFrom(ctx); ok {
		klog.InfoS("🔍 ServiceStorage.Get - USER CONTEXT",
			"username", userInfo.GetName(),
			"uid", userInfo.GetUID(),
			"groups", userInfo.GetGroups(),
			"note", "If this fails, compare with successful objInfo context")
	}

	// Delegate to BaseStorage with enhanced error tracking
	result, err := s.BaseStorage.Get(ctx, name, options)

	if err != nil {
		klog.InfoS("❌ ServiceStorage.Get FAILED - CRITICAL ERROR ANALYSIS",
			"name", name,
			"namespace", namespace,
			"error", err.Error(),
			"errorType", fmt.Sprintf("%T", err),
			"theory", "This might be the objInfo internal GET failure!")
	} else {
		klog.InfoS("✅ ServiceStorage.Get SUCCESS",
			"name", name,
			"namespace", namespace,
			"note", "GET worked fine - check if this was objInfo or external call")
	}

	return result, err
}

// Update adds logging to track UPDATE calls during PATCH operations
func (s *ServiceStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	klog.InfoS("🔄🔄🔄 ServiceStorage.Update CALLED - PATCH MIGHT USE THIS",
		"name", name,
		"forceAllowCreate", forceAllowCreate)

	// Delegate to BaseStorage with comprehensive error handling
	result, created, err := s.BaseStorage.Update(ctx, name, objInfo, createValidation, updateValidation, forceAllowCreate, options)

	if err != nil {
		klog.InfoS("❌ ServiceStorage.Update FAILED",
			"name", name,
			"error", err.Error(),
			"errorType", fmt.Sprintf("%T", err),
			"created", created)
		return nil, created, err
	}

	klog.InfoS("✅ ServiceStorage.Update SUCCESS",
		"name", name,
		"created", created)

	return result, created, nil
}

// Compile-time interface assertions
var _ rest.TableConvertor = &ServiceStorage{}

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

	// DEBUG: Check interface assertions at runtime
	klog.InfoS("🔧 INTERFACE DEBUG: Checking ServiceStorage interface assertions",
		"implements_rest.Storage", func() bool { _, ok := interface{}(storage).(rest.Storage); return ok }(),
		"implements_rest.Patcher", func() bool { _, ok := interface{}(storage).(rest.Patcher); return ok }(),
		"implements_rest.Getter", func() bool { _, ok := interface{}(storage).(rest.Getter); return ok }(),
		"baseStorage_implements_rest.Patcher", func() bool { _, ok := interface{}(baseStorage).(rest.Patcher); return ok }())

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

	// DEBUG: Check interface assertions for OLD method (the one actually used)
	klog.InfoS("🔧 OLD METHOD INTERFACE DEBUG: Checking ServiceStorage interface assertions",
		"method", "NewServiceStorageWithClient",
		"implements_rest.Storage", func() bool { _, ok := interface{}(storage).(rest.Storage); return ok }(),
		"implements_rest.Patcher", func() bool { _, ok := interface{}(storage).(rest.Patcher); return ok }(),
		"implements_rest.Getter", func() bool { _, ok := interface{}(storage).(rest.Getter); return ok }(),
		"baseStorage_implements_rest.Patcher", func() bool { _, ok := interface{}(baseStorage).(rest.Patcher); return ok }())

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
