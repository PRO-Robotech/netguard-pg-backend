package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	utils2 "netguard-pg-backend/internal/k8s/registry/utils"
	watchpkg "netguard-pg-backend/internal/k8s/registry/watch"

	"errors"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

// ServiceStorage implements REST storage for Service resources
type ServiceStorage struct {
	backendClient client.BackendClient
}

// Compile-time assertions to ensure correct verbs are advertised
var (
	_ rest.Storage         = &ServiceStorage{}
	_ rest.Getter          = &ServiceStorage{}
	_ rest.Lister          = &ServiceStorage{}
	_ rest.Watcher         = &ServiceStorage{}
	_ rest.Creater         = &ServiceStorage{}
	_ rest.Updater         = &ServiceStorage{}
	_ rest.GracefulDeleter = &ServiceStorage{}
	_ rest.Patcher         = &ServiceStorage{}
)

// NewServiceStorage creates a new ServiceStorage
func NewServiceStorage(backendClient client.BackendClient) *ServiceStorage {
	return &ServiceStorage{
		backendClient: backendClient,
	}
}

// New returns an empty Service object
func (s *ServiceStorage) New() runtime.Object {
	return &netguardv1beta1.Service{}
}

// NewList returns an empty ServiceList object
func (s *ServiceStorage) NewList() runtime.Object {
	return &netguardv1beta1.ServiceList{}
}

// NamespaceScoped returns true since Service is namespaced
func (s *ServiceStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name of the resource
func (s *ServiceStorage) GetSingularName() string {
	return "service"
}

// Destroy cleans up resources on shutdown
func (s *ServiceStorage) Destroy() {
	// Nothing to clean up for now
}

// Get retrieves a Service by name from backend
func (s *ServiceStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace := utils2.NamespaceFrom(ctx)

	// Create resource identifier
	id := models.NewResourceIdentifier(name, models.WithNamespace(namespace))

	// Fetch from backend first
	svc, err := s.backendClient.GetService(ctx, id)
	if err == nil {
		k8sSvc := convertServiceToK8s(*svc)
		return &k8sSvc, nil
	}
	if errors.Is(err, ports.ErrNotFound) || strings.Contains(err.Error(), "entity not found") {
		return nil, apierrors.NewNotFound(netguardv1beta1.Resource("services"), name)
	}
	return nil, fmt.Errorf("failed to get service: %w", err)
}

// List retrieves Services from backend with optional filtering
func (s *ServiceStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	scope := utils2.ScopeFromContext(ctx)

	// Get from backend
	services, err := s.backendClient.ListServices(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Convert to K8s API format
	serviceList := &netguardv1beta1.ServiceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceList",
			APIVersion: "netguard.sgroups.io/v1beta1",
		},
		Items: make([]netguardv1beta1.Service, 0, len(services)),
	}

	for _, service := range services {
		k8sService := convertServiceToK8s(service)
		serviceList.Items = append(serviceList.Items, k8sService)
	}

	return serviceList, nil
}

// Create creates a new Service in backend
func (s *ServiceStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	service, ok := obj.(*netguardv1beta1.Service)
	if !ok {
		return nil, fmt.Errorf("not a Service object")
	}

	// Run validation if provided
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	// Convert to backend format and ensure system meta is populated
	backendService := convertServiceFromK8s(service)
	backendService.Meta.TouchOnCreate()

	klog.V(2).Infof("ServiceStorage.Create SyncUpsert ns=%q name=%q uid=%s generation=%d rv=%s", backendService.Namespace, backendService.Name, backendService.Meta.UID, backendService.Meta.Generation, backendService.Meta.ResourceVersion)

	// Create in backend
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.Service{backendService}); err != nil {
		return nil, fmt.Errorf("backend sync failed: %w", err)
	}

	klog.V(2).Infof("ServiceStorage.Create backend Sync OK ns=%q name=%q", backendService.Namespace, backendService.Name)

	// Fetch updated object to include server-assigned metadata (UID, timestamps, RV)
	updatedModel, err := s.backendClient.GetService(ctx, backendService.ResourceIdentifier)
	if err != nil {
		// fall back to local copy if backend get failed
		updatedModel = &backendService
	}

	resp := convertServiceToK8s(*updatedModel)
	setServiceCondition(&resp, "Ready", metav1.ConditionTrue, "ServiceCreated", "Service successfully created in backend")
	return &resp, nil
}

// Update updates an existing Service in backend
func (s *ServiceStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// Get current object
	currentObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		if !forceAllowCreate {
			return nil, false, err
		}
		// Create new object if not found and forceAllowCreate is true
		newObj, err := objInfo.UpdatedObject(ctx, nil)
		if err != nil {
			return nil, false, err
		}
		createdObj, err := s.Create(ctx, newObj, createValidation, &metav1.CreateOptions{})
		return createdObj, true, err
	}

	// Update object
	updatedObj, err := objInfo.UpdatedObject(ctx, currentObj)
	if err != nil {
		return nil, false, err
	}

	service, ok := updatedObj.(*netguardv1beta1.Service)
	if !ok {
		return nil, false, fmt.Errorf("not a Service object")
	}

	// Run validation if provided
	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, currentObj); err != nil {
			return nil, false, err
		}
	}

	// Convert to backend format
	backendService := convertServiceFromK8s(service)

	// Update in backend
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.Service{backendService}); err != nil {
		return nil, false, fmt.Errorf("backend sync failed: %w", err)
	}

	updatedModel, err := s.backendClient.GetService(ctx, backendService.ResourceIdentifier)
	if err != nil {
		updatedModel = &backendService
	}
	resp := convertServiceToK8s(*updatedModel)
	setServiceCondition(&resp, "Ready", metav1.ConditionTrue, "ServiceUpdated", "Service successfully updated in backend")
	return &resp, false, nil
}

// Delete removes a Service from backend
func (s *ServiceStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	// Get current object first
	obj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	// Run validation if provided
	if deleteValidation != nil {
		if err := deleteValidation(ctx, obj); err != nil {
			return nil, false, err
		}
	}

	// Extract namespace from context
	namespace := utils2.NamespaceFrom(ctx)

	// Create resource identifier
	id := models.NewResourceIdentifier(name, models.WithNamespace(namespace))

	// Delete from backend
	err = s.backendClient.DeleteService(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("failed to delete service from backend: %w", err)
	}

	return obj, true, nil
}

// Watch implements watch functionality using Shared Poller
func (s *ServiceStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	pollerManager := watchpkg.GetPollerManager(s.backendClient)
	poller := pollerManager.GetPoller("services")

	client, err := poller.AddClient(options)
	if err != nil {
		return nil, fmt.Errorf("failed to add watch client: %w", err)
	}

	return watchpkg.NewPollerWatchInterface(client, poller), nil
}

// GetSupportedVerbs returns REST verbs supported by this storage (for discovery)
func (s *ServiceStorage) GetSupportedVerbs() []string {
	return []string{"get", "list", "create", "update", "delete", "watch", "patch"}
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

// Helper functions for conversion

func convertServiceToK8s(service models.Service) netguardv1beta1.Service {
	k8sService := netguardv1beta1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "netguard.sgroups.io/v1beta1",
		},
		ObjectMeta: func() metav1.ObjectMeta {
			uid := types.UID(service.Meta.UID)
			if uid == "" {
				uid = types.UID(fmt.Sprintf("%s.%s", service.ResourceIdentifier.Namespace, service.ResourceIdentifier.Name))
			}
			return metav1.ObjectMeta{
				Name:              service.ResourceIdentifier.Name,
				Namespace:         service.ResourceIdentifier.Namespace,
				UID:               uid,
				ResourceVersion:   service.Meta.ResourceVersion,
				Generation:        service.Meta.Generation,
				CreationTimestamp: service.Meta.CreationTS,
				Labels:            service.Meta.Labels,
				Annotations:       service.Meta.Annotations,
			}
		}(),
		Spec: netguardv1beta1.ServiceSpec{
			Description: service.Description,
		},
	}

	// Convert IngressPorts - direct mapping since both use string ports
	for _, port := range service.IngressPorts {
		k8sPort := netguardv1beta1.IngressPort{
			Protocol:    netguardv1beta1.TransportProtocol(port.Protocol),
			Port:        port.Port, // Direct string mapping
			Description: port.Description,
		}
		k8sService.Spec.IngressPorts = append(k8sService.Spec.IngressPorts, k8sPort)
	}

	return k8sService
}

func convertServiceFromK8s(k8sService *netguardv1beta1.Service) models.Service {
	svc := models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(k8sService.Name,
				models.WithNamespace(k8sService.Namespace)),
		},
		Description: k8sService.Spec.Description,
		Meta: models.Meta{
			UID:             string(k8sService.UID),
			ResourceVersion: k8sService.ResourceVersion,
			Generation:      k8sService.Generation,
			CreationTS:      k8sService.CreationTimestamp,
			Labels:          k8sService.Labels,
			Annotations:     k8sService.Annotations,
		},
	}

	for _, port := range k8sService.Spec.IngressPorts {
		svc.IngressPorts = append(svc.IngressPorts, models.IngressPort{
			Protocol:    models.TransportProtocol(port.Protocol),
			Port:        port.Port,
			Description: port.Description,
		})
	}

	return svc
}

func setServiceCondition(service *netguardv1beta1.Service, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
		ObservedGeneration: service.Generation,
	}

	// Find existing condition and update or append new one
	for i, existingCondition := range service.Status.Conditions {
		if existingCondition.Type == conditionType {
			service.Status.Conditions[i] = condition
			return
		}
	}

	service.Status.Conditions = append(service.Status.Conditions, condition)
}

// Patch applies a patch to an existing Service. Supports JSON/merge patches.
func (s *ServiceStorage) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts *metav1.PatchOptions, subresources ...string) (runtime.Object, error) {
	if len(subresources) > 0 {
		return nil, apierrors.NewBadRequest("subresources are not supported for Service")
	}

	// current object
	currObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	currSvc := currObj.(*netguardv1beta1.Service)

	currBytes, err := json.Marshal(currSvc)
	if err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("marshal current svc: %w", err))
	}

	var patchedBytes []byte
	switch pt {
	case types.JSONPatchType:
		patch, err := jsonpatch.DecodePatch(data)
		if err != nil {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid JSON patch: %v", err))
		}
		patchedBytes, err = patch.Apply(currBytes)
		if err != nil {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("apply JSON patch: %v", err))
		}
	case types.MergePatchType, types.ApplyPatchType:
		patchedBytes, err = jsonpatch.MergePatch(currBytes, data)
		if err != nil {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("apply merge patch: %v", err))
		}
	case types.StrategicMergePatchType:
		return nil, apierrors.NewBadRequest("strategic merge patch not supported for Service")
	default:
		return nil, apierrors.NewBadRequest(fmt.Sprintf("unsupported patch type %s", pt))
	}

	var updatedSvc netguardv1beta1.Service
	if err := json.Unmarshal(patchedBytes, &updatedSvc); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid patched object: %v", err))
	}

	if updatedSvc.Name == "" {
		updatedSvc.Name = currSvc.Name
	}
	if updatedSvc.Namespace == "" {
		updatedSvc.Namespace = currSvc.Namespace
	}

	// sync to backend
	model := convertServiceFromK8s(&updatedSvc)
	if err := s.backendClient.Sync(ctx, models.SyncOpUpsert, []models.Service{model}); err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("backend sync failed: %w", err))
	}

	updatedModel, err := s.backendClient.GetService(ctx, model.ResourceIdentifier)
	if err != nil {
		updatedModel = &model
	}
	resp := convertServiceToK8s(*updatedModel)
	return &resp, nil
}
