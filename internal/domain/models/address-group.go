package models

// AddressGroup represents a group of addresses
type AddressGroup struct {
	SelfRef
	Description string
	Addresses   []string
	Services    []ServiceRef
	Meta        Meta
}

// AddressGroupRef represents a reference to an AddressGroup
type AddressGroupRef struct {
	ResourceIdentifier
}

// NewAddressGroupRef creates a new AddressGroupRef
func NewAddressGroupRef(name string, opts ...ResourceIdentifierOption) AddressGroupRef {
	return AddressGroupRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}
