package models

import (
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// AddressGroupBinding represents a binding between a Service and an AddressGroup
type AddressGroupBinding struct {
	SelfRef
	ServiceRef      v1beta1.NamespacedObjectReference // Full object reference with apiVersion, kind and namespace
	AddressGroupRef v1beta1.NamespacedObjectReference // Full object reference with apiVersion, kind and namespace
	Meta            Meta
}

// ServiceRefKey returns the key for the ServiceRef (namespace/name)
func (b *AddressGroupBinding) ServiceRefKey() string {
	// ServiceRef has its own namespace now
	if b.ServiceRef.Namespace == "" {
		return b.ServiceRef.Name
	}
	return b.ServiceRef.Namespace + "/" + b.ServiceRef.Name
}

// AddressGroupRefKey returns the key for the AddressGroupRef (namespace/name)
func (b *AddressGroupBinding) AddressGroupRefKey() string {
	if b.AddressGroupRef.Namespace == "" {
		return b.AddressGroupRef.Name
	}
	return b.AddressGroupRef.Namespace + "/" + b.AddressGroupRef.Name
}

// AddressGroupBindingRef represents a reference to an AddressGroupBinding
type AddressGroupBindingRef struct {
	ResourceIdentifier
}

// NewAddressGroupBindingRef creates a new ServiceRef
func NewAddressGroupBindingRef(name string, opts ...ResourceIdentifierOption) AddressGroupBindingRef {
	return AddressGroupBindingRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}
