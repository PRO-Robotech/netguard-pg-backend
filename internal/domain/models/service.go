package models

// Service represents a service with its ports
type Service struct {
	SelfRef
	Description   string
	IngressPorts  []IngressPort
	AddressGroups []AddressGroupRef
	Meta          Meta
}

// ServiceRef represents a reference to a Service
type ServiceRef struct {
	ResourceIdentifier
}

// NewServiceRef creates a new ServiceRef
func NewServiceRef(name string, opts ...ResourceIdentifierOption) ServiceRef {
	return ServiceRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}
