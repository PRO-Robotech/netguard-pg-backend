package convert

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/registry/base"
)

// AddressGroupPortMappingConverter implements conversion between k8s AddressGroupPortMapping and domain AddressGroupPortMapping
type AddressGroupPortMappingConverter struct{}

// Compile-time interface assertion
var _ base.Converter[*netguardv1beta1.AddressGroupPortMapping, *models.AddressGroupPortMapping] = &AddressGroupPortMappingConverter{}

// ToDomain converts a Kubernetes AddressGroupPortMapping object to a domain AddressGroupPortMapping model
func (c *AddressGroupPortMappingConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.AddressGroupPortMapping) (*models.AddressGroupPortMapping, error) {
	if k8sObj == nil {
		return nil, fmt.Errorf("k8s AddressGroupPortMapping object is nil")
	}

	// Create domain address group port mapping
	domainMapping := &models.AddressGroupPortMapping{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sObj.Name,
				Namespace: k8sObj.Namespace,
			},
		},
		Meta: models.Meta{
			UID:                string(k8sObj.UID),
			ResourceVersion:    k8sObj.ResourceVersion,
			Generation:         k8sObj.Generation,
			CreationTS:         k8sObj.CreationTimestamp,
			ObservedGeneration: k8sObj.Status.ObservedGeneration,
			Conditions:         k8sObj.Status.Conditions,
		},
		AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
	}

	// Copy metadata
	if k8sObj.Labels != nil {
		domainMapping.Meta.Labels = make(map[string]string)
		for k, v := range k8sObj.Labels {
			domainMapping.Meta.Labels[k] = v
		}
	}

	if k8sObj.Annotations != nil {
		domainMapping.Meta.Annotations = make(map[string]string)
		for k, v := range k8sObj.Annotations {
			domainMapping.Meta.Annotations[k] = v
		}
	}

	// Convert access ports from AccessPortsSpec
	if len(k8sObj.AccessPorts.Items) > 0 {
		for _, servicePortsRef := range k8sObj.AccessPorts.Items {
			serviceRef := models.ServiceRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      servicePortsRef.Name,
					Namespace: servicePortsRef.Namespace,
				},
			}

			servicePorts := models.ServicePorts{
				Ports: make(models.ProtocolPorts),
			}

			// Convert TCP ports
			if len(servicePortsRef.Ports.TCP) > 0 {
				tcpPortRanges := make([]models.PortRange, 0, len(servicePortsRef.Ports.TCP))
				for _, portConfig := range servicePortsRef.Ports.TCP {
					portRanges, err := c.parsePortConfig(portConfig.Port)
					if err != nil {
						return nil, fmt.Errorf("failed to parse TCP port config %s: %w", portConfig.Port, err)
					}
					tcpPortRanges = append(tcpPortRanges, portRanges...)
				}
				servicePorts.Ports[models.TCP] = tcpPortRanges
			}

			// Convert UDP ports
			if len(servicePortsRef.Ports.UDP) > 0 {
				udpPortRanges := make([]models.PortRange, 0, len(servicePortsRef.Ports.UDP))
				for _, portConfig := range servicePortsRef.Ports.UDP {
					portRanges, err := c.parsePortConfig(portConfig.Port)
					if err != nil {
						return nil, fmt.Errorf("failed to parse UDP port config %s: %w", portConfig.Port, err)
					}
					udpPortRanges = append(udpPortRanges, portRanges...)
				}
				servicePorts.Ports[models.UDP] = udpPortRanges
			}

			domainMapping.AccessPorts[serviceRef] = servicePorts
		}
	}

	return domainMapping, nil
}

// FromDomain converts a domain AddressGroupPortMapping model to a Kubernetes AddressGroupPortMapping object
func (c *AddressGroupPortMappingConverter) FromDomain(ctx context.Context, domainObj *models.AddressGroupPortMapping) (*netguardv1beta1.AddressGroupPortMapping, error) {
	if domainObj == nil {
		return nil, fmt.Errorf("domain AddressGroupPortMapping object is nil")
	}

	// Create k8s address group port mapping
	k8sMapping := &netguardv1beta1.AddressGroupPortMapping{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupPortMapping",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              domainObj.ResourceIdentifier.Name,
			Namespace:         domainObj.ResourceIdentifier.Namespace,
			UID:               types.UID(domainObj.Meta.UID),
			ResourceVersion:   domainObj.Meta.ResourceVersion,
			Generation:        domainObj.Meta.Generation,
			CreationTimestamp: domainObj.Meta.CreationTS,
		},
		Spec: netguardv1beta1.AddressGroupPortMappingSpec{
			// Empty spec as in controller
		},
		AccessPorts: netguardv1beta1.AccessPortsSpec{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AccessPortsSpec",
			},
			Items: make([]netguardv1beta1.ServicePortsRef, 0, len(domainObj.AccessPorts)),
		},
	}

	// Copy metadata
	if domainObj.Meta.Labels != nil {
		k8sMapping.Labels = make(map[string]string)
		for k, v := range domainObj.Meta.Labels {
			k8sMapping.Labels[k] = v
		}
	}

	if domainObj.Meta.Annotations != nil {
		k8sMapping.Annotations = make(map[string]string)
		for k, v := range domainObj.Meta.Annotations {
			k8sMapping.Annotations[k] = v
		}
	}

	// Convert access ports to AccessPortsSpec
	for serviceRef, servicePorts := range domainObj.AccessPorts {
		servicePortsRef := netguardv1beta1.ServicePortsRef{
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

		// Convert TCP ports
		if tcpRanges, exists := servicePorts.Ports[models.TCP]; exists {
			tcpConfigs := make([]netguardv1beta1.PortConfig, 0, len(tcpRanges))
			for _, portRange := range tcpRanges {
				portConfig := netguardv1beta1.PortConfig{
					Port: c.formatPortRange(portRange),
				}
				tcpConfigs = append(tcpConfigs, portConfig)
			}
			servicePortsRef.Ports.TCP = tcpConfigs
		}

		// Convert UDP ports
		if udpRanges, exists := servicePorts.Ports[models.UDP]; exists {
			udpConfigs := make([]netguardv1beta1.PortConfig, 0, len(udpRanges))
			for _, portRange := range udpRanges {
				portConfig := netguardv1beta1.PortConfig{
					Port: c.formatPortRange(portRange),
				}
				udpConfigs = append(udpConfigs, portConfig)
			}
			servicePortsRef.Ports.UDP = udpConfigs
		}

		k8sMapping.AccessPorts.Items = append(k8sMapping.AccessPorts.Items, servicePortsRef)
	}

	// Convert status - переносим условия из Meta в Status
	k8sMapping.Status = netguardv1beta1.AddressGroupPortMappingStatus{
		ObservedGeneration: domainObj.Meta.ObservedGeneration,
		Conditions:         domainObj.Meta.Conditions, // Backend формирует условия
	}

	return k8sMapping, nil
}

// ToList converts a slice of domain AddressGroupPortMapping models to a Kubernetes AddressGroupPortMappingList object
func (c *AddressGroupPortMappingConverter) ToList(ctx context.Context, domainObjs []*models.AddressGroupPortMapping) (runtime.Object, error) {
	mappingList := &netguardv1beta1.AddressGroupPortMappingList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupPortMappingList",
		},
		ListMeta: metav1.ListMeta{},
		Items:    make([]netguardv1beta1.AddressGroupPortMapping, len(domainObjs)),
	}

	for i, domainObj := range domainObjs {
		k8sObj, err := c.FromDomain(ctx, domainObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert domain address group port mapping %d to k8s: %w", i, err)
		}
		mappingList.Items[i] = *k8sObj
	}

	return mappingList, nil
}

// Helper methods for port parsing and formatting

// parsePortConfig parses a port configuration string into PortRange(s)
// Supports formats like "80", "8080-9090"
func (c *AddressGroupPortMappingConverter) parsePortConfig(portStr string) ([]models.PortRange, error) {
	if strings.Contains(portStr, "-") {
		// Port range format: "8080-9090"
		parts := strings.Split(portStr, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid port range format: %s", portStr)
		}

		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid start port: %s", parts[0])
		}

		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid end port: %s", parts[1])
		}

		return []models.PortRange{{Start: start, End: end}}, nil
	} else {
		// Single port format: "80"
		port, err := strconv.Atoi(strings.TrimSpace(portStr))
		if err != nil {
			return nil, fmt.Errorf("invalid port: %s", portStr)
		}

		return []models.PortRange{{Start: port, End: port}}, nil
	}
}

// formatPortRange formats a PortRange back to a string
func (c *AddressGroupPortMappingConverter) formatPortRange(portRange models.PortRange) string {
	if portRange.Start == portRange.End {
		return fmt.Sprintf("%d", portRange.Start)
	}
	return fmt.Sprintf("%d-%d", portRange.Start, portRange.End)
}

// NewAddressGroupPortMappingConverter creates a new AddressGroupPortMappingConverter instance
func NewAddressGroupPortMappingConverter() *AddressGroupPortMappingConverter {
	return &AddressGroupPortMappingConverter{}
}
