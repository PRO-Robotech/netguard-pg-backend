package models

// AddressGroupBinding represents a binding between a Service and an AddressGroup
type AddressGroupBinding struct {
	SelfRef
	ServiceRef      ServiceRef
	AddressGroupRef AddressGroupRef
	Meta            Meta
}

// AddressGroupBindingRef represents a reference to an AddressGroupBinding
type AddressGroupBindingRef struct {
	ResourceIdentifier
}

// NewAddressGroupBindingRef creates a new ServiceRef
func NewAddressGroupBindingRef(name string, opts ...ResourceIdentifierOption) AddressGroupBindingRef {
	return AddressGroupBindingRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}
