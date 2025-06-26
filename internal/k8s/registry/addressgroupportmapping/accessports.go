package addressgroupportmapping

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
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

var _ rest.Getter = &AccessPortsREST{}

// New returns a new AccessPortsSpec object
func (r *AccessPortsREST) New() runtime.Object {
	return &netguardv1beta1.AccessPortsSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AccessPortsSpec",
		},
	}
}

// Destroy cleans up resources
func (r *AccessPortsREST) Destroy() {}

// Get retrieves the accessPorts for an AddressGroupPortMapping
func (r *AccessPortsREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	// Get the AddressGroupPortMapping from backend
	mappingID := models.NewResourceIdentifier(name, models.WithNamespace(ctx.Value("namespace").(string)))
	mapping, err := r.backendClient.GetAddressGroupPortMapping(ctx, mappingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get address group port mapping: %w", err)
	}

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

		accessPortsSpec.Items = append(accessPortsSpec.Items, item)
	}

	return accessPortsSpec, nil
}
