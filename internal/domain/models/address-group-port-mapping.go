package models

import (
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// AddressGroupPortMapping represents a mapping between an AddressGroup and allowed ports
type AddressGroupPortMapping struct {
	SelfRef
	// Note: We keep ServiceRef as key for map compatibility, but store full reference in ServicePortsItem
	AccessPorts map[ServiceRef]ServicePorts // Legacy map structure for compatibility
	// New structure to store full object references
	AccessPortsWithRefs []ServicePortsItem
	Meta                Meta
}

// ServicePortsItem represents a Service reference with its allowed ports
type ServicePortsItem struct {
	ServiceRef v1beta1.NamespacedObjectReference // Full object reference
	Ports      ProtocolPorts
}

// AddressGroupPortMappingRef represents a reference to a Service
type AddressGroupPortMappingRef struct {
	ResourceIdentifier
}

// NewAddressGroupPortMappingRef creates a new AddressGroupPortMappingRef
func NewAddressGroupPortMappingRef(name string, opts ...ResourceIdentifierOption) AddressGroupPortMappingRef {
	return AddressGroupPortMappingRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}

// ServicePorts defines a reference to a Service and its allowed ports
type ServicePorts struct {
	Ports ProtocolPorts
}
