package models

import (
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// AddressGroupBindingPolicy представляет политику, разрешающую привязку Service к AddressGroup
type AddressGroupBindingPolicy struct {
	SelfRef
	AddressGroupRef v1beta1.NamespacedObjectReference // Full object reference with apiVersion, kind, name, namespace
	ServiceRef      v1beta1.NamespacedObjectReference // Full object reference with apiVersion, kind, name, namespace
	Meta            Meta
}

// ServiceRefKey returns the key for the ServiceRef (namespace/name)
func (p *AddressGroupBindingPolicy) ServiceRefKey() string {
	if p.ServiceRef.Namespace == "" {
		return p.ServiceRef.Name
	}
	return p.ServiceRef.Namespace + "/" + p.ServiceRef.Name
}

// AddressGroupRefKey returns the key for the AddressGroupRef (namespace/name)
func (p *AddressGroupBindingPolicy) AddressGroupRefKey() string {
	if p.AddressGroupRef.Namespace == "" {
		return p.AddressGroupRef.Name
	}
	return p.AddressGroupRef.Namespace + "/" + p.AddressGroupRef.Name
}

// AddressGroupBindingPolicyRef представляет ссылку на AddressGroupBindingPolicy
type AddressGroupBindingPolicyRef struct {
	ResourceIdentifier
}

// NewAddressGroupBindingPolicyRef создает новую ссылку на AddressGroupBindingPolicy
func NewAddressGroupBindingPolicyRef(name string, opts ...ResourceIdentifierOption) AddressGroupBindingPolicyRef {
	return AddressGroupBindingPolicyRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}
