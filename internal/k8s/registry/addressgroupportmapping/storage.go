package addressgroupportmapping

import (
	"context"
	"fmt"
	"strings"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	watchpkg "netguard-pg-backend/internal/k8s/registry/watch"
)

// AddressGroupPortMappingStorage implements REST storage for AddressGroupPortMapping resources
type AddressGroupPortMappingStorage struct {
	backendClient client.BackendClient
}

// NewAddressGroupPortMappingStorage creates a new AddressGroupPortMapping storage
func NewAddressGroupPortMappingStorage(backendClient client.BackendClient) *AddressGroupPortMappingStorage {
	return &AddressGroupPortMappingStorage{
		backendClient: backendClient,
	}
}

// New returns an empty AddressGroupPortMapping object
func (s *AddressGroupPortMappingStorage) New() runtime.Object {
	return &netguardv1beta1.AddressGroupPortMapping{}
}

// NewList returns an empty AddressGroupPortMappingList object
func (s *AddressGroupPortMappingStorage) NewList() runtime.Object {
	return &netguardv1beta1.AddressGroupPortMappingList{}
}

// NamespaceScoped returns true as AddressGroupPortMappings are namespaced
func (s *AddressGroupPortMappingStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name for the resource
func (s *AddressGroupPortMappingStorage) GetSingularName() string {
	return "addressgroupportmapping"
}

// Destroy cleans up resources on shutdown
func (s *AddressGroupPortMappingStorage) Destroy() {
	// Nothing to clean up for now
}

// Get retrieves an AddressGroupPortMapping by name from backend
func (s *AddressGroupPortMappingStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace, ok := ctx.Value("namespace").(string)
	if !ok || namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	resourceID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	mapping, err := s.backendClient.GetAddressGroupPortMapping(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get AddressGroupPortMapping %s/%s: %w", namespace, name, err)
	}

	k8sMapping := convertAddressGroupPortMappingToK8s(*mapping)
	return k8sMapping, nil
}

// List retrieves AddressGroupPortMappings from backend with filtering
func (s *AddressGroupPortMappingStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	var scope ports.Scope
	if options != nil && options.FieldSelector != nil {
		scope = ports.NewResourceIdentifierScope()
	}

	mappings, err := s.backendClient.ListAddressGroupPortMappings(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list AddressGroupPortMappings: %w", err)
	}

	k8sMappingList := &netguardv1beta1.AddressGroupPortMappingList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "AddressGroupPortMappingList",
		},
	}

	for _, mapping := range mappings {
		k8sMapping := convertAddressGroupPortMappingToK8s(mapping)
		k8sMappingList.Items = append(k8sMappingList.Items, *k8sMapping)
	}

	return k8sMappingList, nil
}

// Create creates a new AddressGroupPortMapping in backend
func (s *AddressGroupPortMappingStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	k8sMapping, ok := obj.(*netguardv1beta1.AddressGroupPortMapping)
	if !ok {
		return nil, fmt.Errorf("expected AddressGroupPortMapping, got %T", obj)
	}

	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	mapping := convertAddressGroupPortMappingFromK8s(k8sMapping)
	err := s.backendClient.CreateAddressGroupPortMapping(ctx, &mapping)
	if err != nil {
		return nil, fmt.Errorf("failed to create AddressGroupPortMapping: %w", err)
	}

	setCondition(k8sMapping, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "AddressGroupPortMapping successfully created in backend")

	return k8sMapping, nil
}

// Update updates an existing AddressGroupPortMapping in backend
func (s *AddressGroupPortMappingStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	currentObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		if forceAllowCreate {
			newObj, err := objInfo.UpdatedObject(ctx, nil)
			if err != nil {
				return nil, false, err
			}
			createdObj, err := s.Create(ctx, newObj, createValidation, &metav1.CreateOptions{})
			return createdObj, true, err
		}
		return nil, false, err
	}

	updatedObj, err := objInfo.UpdatedObject(ctx, currentObj)
	if err != nil {
		return nil, false, err
	}

	k8sMapping, ok := updatedObj.(*netguardv1beta1.AddressGroupPortMapping)
	if !ok {
		return nil, false, fmt.Errorf("expected AddressGroupPortMapping, got %T", updatedObj)
	}

	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, currentObj); err != nil {
			return nil, false, err
		}
	}

	mapping := convertAddressGroupPortMappingFromK8s(k8sMapping)
	err = s.backendClient.UpdateAddressGroupPortMapping(ctx, &mapping)
	if err != nil {
		return nil, false, fmt.Errorf("failed to update AddressGroupPortMapping: %w", err)
	}

	setCondition(k8sMapping, ConditionReady, metav1.ConditionTrue,
		ReasonBindingCreated, "AddressGroupPortMapping successfully updated in backend")

	return k8sMapping, false, nil
}

// Delete removes an AddressGroupPortMapping from backend
func (s *AddressGroupPortMappingStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	obj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	if deleteValidation != nil {
		if err := deleteValidation(ctx, obj); err != nil {
			return nil, false, err
		}
	}

	namespace, ok := ctx.Value("namespace").(string)
	if !ok || namespace == "" {
		return nil, false, fmt.Errorf("namespace is required")
	}

	id := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	err = s.backendClient.DeleteAddressGroupPortMapping(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("failed to delete AddressGroupPortMapping: %w", err)
	}

	return obj, true, nil
}

// Watch implements watch functionality for AddressGroupPortMappings
func (s *AddressGroupPortMappingStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	pollerManager := watchpkg.GetPollerManager(s.backendClient)
	poller := pollerManager.GetPoller("addressgroupportmappings")

	client, err := poller.AddClient(options)
	if err != nil {
		return nil, fmt.Errorf("failed to add watch client: %w", err)
	}

	return watchpkg.NewPollerWatchInterface(client, poller), nil
}

// Helper functions for conversion
func convertAddressGroupPortMappingToK8s(mapping models.AddressGroupPortMapping) *netguardv1beta1.AddressGroupPortMapping {
	k8sMapping := &netguardv1beta1.AddressGroupPortMapping{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
			Kind:       "AddressGroupPortMapping",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      mapping.ResourceIdentifier.Name,
			Namespace: mapping.ResourceIdentifier.Namespace,
		},
		Spec: netguardv1beta1.AddressGroupPortMappingSpec{
			// Empty spec as in controller
		},
	}

	// Convert AccessPorts map to AccessPortsSpec
	var items []netguardv1beta1.ServicePortsRef
	for serviceRef, servicePorts := range mapping.AccessPorts {
		item := netguardv1beta1.ServicePortsRef{
			NamespacedObjectReference: netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       serviceRef.Name,
				},
				Namespace: serviceRef.Namespace,
			},
			Ports: netguardv1beta1.ProtocolPorts{},
		}

		// Convert ProtocolPorts
		for protocol, portRanges := range servicePorts.Ports {
			if len(portRanges) == 0 {
				continue
			}

			// Build single port string with comma-separated values
			portStr := formatPortRangesToString(portRanges)

			// Create single PortConfig with comma-separated ports
			portConfig := netguardv1beta1.PortConfig{
				Port: portStr,
			}

			switch protocol {
			case models.TCP:
				item.Ports.TCP = []netguardv1beta1.PortConfig{portConfig}
			case models.UDP:
				item.Ports.UDP = []netguardv1beta1.PortConfig{portConfig}
			}
		}

		items = append(items, item)
	}

	k8sMapping.AccessPorts = netguardv1beta1.AccessPortsSpec{
		Items: items,
	}

	return k8sMapping
}

// formatPortRangesToString converts []models.PortRange to comma-separated string like "80,443,8080-9090"
func formatPortRangesToString(ranges []models.PortRange) string {
	var parts []string
	for _, portRange := range ranges {
		if portRange.Start == portRange.End {
			// Single port
			parts = append(parts, fmt.Sprintf("%d", portRange.Start))
		} else {
			// Port range
			parts = append(parts, fmt.Sprintf("%d-%d", portRange.Start, portRange.End))
		}
	}
	return strings.Join(parts, ",")
}

func convertAddressGroupPortMappingFromK8s(k8sMapping *netguardv1beta1.AddressGroupPortMapping) models.AddressGroupPortMapping {
	mapping := models.AddressGroupPortMapping{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				k8sMapping.Name,
				models.WithNamespace(k8sMapping.Namespace),
			),
		},
		AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
	}

	// Convert AccessPortsSpec to AccessPorts map
	for _, item := range k8sMapping.AccessPorts.Items {
		serviceRef := models.ServiceRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				item.Name,
				models.WithNamespace(item.Namespace),
			),
		}

		servicePorts := models.ServicePorts{
			Ports: make(models.ProtocolPorts),
		}

		// Convert TCP ports
		if len(item.Ports.TCP) > 0 {
			var ranges []models.PortRange
			for _, portConfig := range item.Ports.TCP {
				parsedRanges, err := validation.ParsePortRanges(portConfig.Port)
				if err != nil {
					// Skip invalid port configs or log error
					continue
				}
				ranges = append(ranges, parsedRanges...)
			}
			servicePorts.Ports[models.TCP] = ranges
		}

		// Convert UDP ports
		if len(item.Ports.UDP) > 0 {
			var ranges []models.PortRange
			for _, portConfig := range item.Ports.UDP {
				parsedRanges, err := validation.ParsePortRanges(portConfig.Port)
				if err != nil {
					// Skip invalid port configs or log error
					continue
				}
				ranges = append(ranges, parsedRanges...)
			}
			servicePorts.Ports[models.UDP] = ranges
		}

		mapping.AccessPorts[serviceRef] = servicePorts
	}

	return mapping
}

// Status condition helpers
const (
	ConditionReady = "Ready"

	ReasonBindingCreated       = "BindingCreated"
	ReasonServiceNotFound      = "ServiceNotFound"
	ReasonAddressGroupNotFound = "AddressGroupNotFound"
	ReasonSyncFailed           = "SyncFailed"
	ReasonDeletionFailed       = "DeletionFailed"
)

func setCondition(obj runtime.Object, conditionType string, status metav1.ConditionStatus, reason, message string) {
	// TODO: Implement proper condition setting
}
