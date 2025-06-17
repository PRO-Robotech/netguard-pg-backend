package models

// AddressGroupBindingPolicy представляет политику, разрешающую привязку Service к AddressGroup
type AddressGroupBindingPolicy struct {
	SelfRef
	AddressGroupRef AddressGroupRef
	ServiceRef      ServiceRef
}

// AddressGroupBindingPolicyRef представляет ссылку на AddressGroupBindingPolicy
type AddressGroupBindingPolicyRef struct {
	ResourceIdentifier
}

// NewAddressGroupBindingPolicyRef создает новую ссылку на AddressGroupBindingPolicy
func NewAddressGroupBindingPolicyRef(name string, opts ...ResourceIdentifierOption) AddressGroupBindingPolicyRef {
	return AddressGroupBindingPolicyRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}
