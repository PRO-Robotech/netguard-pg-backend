package addressgroupportmapping

import (
	"context"
	"fmt"
	"sort"
	"strings"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/utils"
)

// AccessPortsREST implements the accessPorts subresource for AddressGroupPortMapping
type AccessPortsREST struct {
	backendClient client.BackendClient
}

// NewAccessPortsREST creates a new accessPorts subresource handler
func NewAccessPortsREST(backendClient client.BackendClient) *AccessPortsREST {
	return &AccessPortsREST{
		backendClient: backendClient,
	}
}

// Compile-time interface assertions
var _ rest.Getter = &AccessPortsREST{}
var _ rest.Lister = &AccessPortsREST{}
var _ rest.TableConvertor = &AccessPortsREST{}

// New returns a new AccessPortsSpec object
func (r *AccessPortsREST) New() runtime.Object {
	return &netguardv1beta1.AccessPortsSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AccessPortsSpec",
		},
	}
}

// NewList returns a new AccessPortsSpecList object
func (r *AccessPortsREST) NewList() runtime.Object {
	return &netguardv1beta1.AccessPortsSpecList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AccessPortsSpecList",
		},
	}
}

// Destroy cleans up resources
func (r *AccessPortsREST) Destroy() {}

// Get retrieves the accessPorts for a specific AddressGroupPortMapping
func (r *AccessPortsREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace := utils.NamespaceFrom(ctx)

	// Get the AddressGroupPortMapping from backend
	mappingID := models.NewResourceIdentifier(name, models.WithNamespace(namespace))
	mapping, err := r.backendClient.GetAddressGroupPortMapping(ctx, mappingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get address group port mapping: %w", err)
	}

	// Get access ports for this mapping
	accessPortsSpec, err := r.getAccessPortsForMapping(ctx, mapping)
	if err != nil {
		return nil, err
	}

	// Set metadata for identification
	accessPortsSpec.ObjectMeta = metav1.ObjectMeta{
		Name:      mapping.Name,
		Namespace: mapping.Namespace,
	}

	return accessPortsSpec, nil
}

// List retrieves accessPorts for all AddressGroupPortMappings in the namespace
func (r *AccessPortsREST) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	scope := utils.ScopeFromContext(ctx)

	// Get all AddressGroupPortMappings in scope
	mappings, err := r.backendClient.ListAddressGroupPortMappings(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to list address group port mappings: %w", err)
	}

	// Build list response
	accessPortsSpecList := &netguardv1beta1.AccessPortsSpecList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AccessPortsSpecList",
		},
		Items: []netguardv1beta1.AccessPortsSpec{},
	}

	// Get access ports for each mapping
	for _, mapping := range mappings {
		accessPortsSpec, err := r.getAccessPortsForMapping(ctx, &mapping)
		if err != nil {
			// Log error but continue processing other mappings
			continue
		}

		// Set mapping name in metadata for identification
		accessPortsSpec.ObjectMeta = metav1.ObjectMeta{
			Name:      mapping.Name,
			Namespace: mapping.Namespace,
		}

		accessPortsSpecList.Items = append(accessPortsSpecList.Items, *accessPortsSpec)
	}

	// Apply sorting (по умолчанию namespace + name, или через sortBy параметр)
	sortBy := utils.ExtractSortByFromContext(ctx)
	err = utils.ApplySorting(accessPortsSpecList.Items, sortBy,
		// idFn для извлечения ResourceIdentifier
		func(item netguardv1beta1.AccessPortsSpec) models.ResourceIdentifier {
			return models.ResourceIdentifier{
				Name:      item.ObjectMeta.Name,
				Namespace: item.ObjectMeta.Namespace,
			}
		},
		// k8sObjectFn для конвертации в Kubernetes объект
		func(item netguardv1beta1.AccessPortsSpec) runtime.Object {
			return &item
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sort access ports: %w", err)
	}

	return accessPortsSpecList, nil
}

// ConvertToTable converts objects to tabular format for kubectl
func (r *AccessPortsREST) ConvertToTable(ctx context.Context, obj runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Mapping", Type: "string", Format: "name", Description: "AddressGroupPortMapping name"},
			{Name: "Namespace", Type: "string", Description: "Mapping namespace"},
			{Name: "Services", Type: "integer", Description: "Number of services with access"},
		},
	}

	switch t := obj.(type) {
	case *netguardv1beta1.AccessPortsSpec:
		table.Rows = []metav1.TableRow{
			{
				Cells: []interface{}{
					t.ObjectMeta.Name,
					t.ObjectMeta.Namespace,
					len(t.Items),
				},
				Object: runtime.RawExtension{Object: t},
			},
		}
	case *netguardv1beta1.AccessPortsSpecList:
		for _, item := range t.Items {
			table.Rows = append(table.Rows, metav1.TableRow{
				Cells: []interface{}{
					item.ObjectMeta.Name,
					item.ObjectMeta.Namespace,
					len(item.Items),
				},
				Object: runtime.RawExtension{Object: &item},
			})
		}
	default:
		return nil, fmt.Errorf("unsupported object type: %T", obj)
	}

	return table, nil
}

// getAccessPortsForMapping is a helper function to get access ports for a mapping
func (r *AccessPortsREST) getAccessPortsForMapping(ctx context.Context, mapping *models.AddressGroupPortMapping) (*netguardv1beta1.AccessPortsSpec, error) {
	// Build AccessPortsSpec from mapping.AccessPorts
	accessPortsSpec := &netguardv1beta1.AccessPortsSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AccessPortsSpec",
		},
		Items: []netguardv1beta1.ServicePortsRef{},
	}

	// Convert AccessPorts map to ServicePortsRef slice
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

		// Convert ProtocolPorts in deterministic order (TCP first, then UDP)
		protocols := []models.TransportProtocol{models.TCP, models.UDP}
		for _, protocol := range protocols {
			portRanges, exists := servicePorts.Ports[protocol]
			if !exists || len(portRanges) == 0 {
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

		accessPortsSpec.Items = append(accessPortsSpec.Items, item)
	}

	// Sort services by namespace + name to ensure consistent ordering
	utils.SortByNamespaceName(accessPortsSpec.Items, func(item netguardv1beta1.ServicePortsRef) models.ResourceIdentifier {
		return models.ResourceIdentifier{
			Name:      item.Name,
			Namespace: item.Namespace,
		}
	})

	return accessPortsSpec, nil
}

// formatPortRangesToString converts []models.PortRange to comma-separated string like "80,443,8080-9090"
func formatPortRangesToString(ranges []models.PortRange) string {
	// Sort port ranges by start port to ensure consistent ordering
	sortedRanges := make([]models.PortRange, len(ranges))
	copy(sortedRanges, ranges)
	sort.Slice(sortedRanges, func(i, j int) bool {
		return sortedRanges[i].Start < sortedRanges[j].Start
	})

	var parts []string
	for _, portRange := range sortedRanges {
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
