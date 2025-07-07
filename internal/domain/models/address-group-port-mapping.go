package models

// AddressGroupPortMapping represents a mapping between an AddressGroup and allowed ports
type AddressGroupPortMapping struct {
	SelfRef
	AccessPorts map[ServiceRef]ServicePorts
	Meta        Meta
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
