package models

// AddressGroup represents an address group configuration for Netguard
type AddressGroup struct {
	SelfRef
	DefaultAction RuleAction `json:"defaultAction"`   // Default action for the address group (ACCEPT/DROP)
	Logs          bool       `json:"logs,omitempty"`  // Whether to enable logs
	Trace         bool       `json:"trace,omitempty"` // Whether to enable trace
	Meta          Meta
}

// AddressGroupRef represents a reference to an AddressGroup
type AddressGroupRef struct {
	ResourceIdentifier
}

// NewAddressGroupRef creates a new AddressGroupRef
func NewAddressGroupRef(name string, opts ...ResourceIdentifierOption) AddressGroupRef {
	return AddressGroupRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}
