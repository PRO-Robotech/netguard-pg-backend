package models

import (
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// Service represents a service with its ports
type Service struct {
	SelfRef
	Description   string
	IngressPorts  []IngressPort
	AddressGroups []AddressGroupRef

	// AggregatedAddressGroups contains all address groups from both spec and bindings
	AggregatedAddressGroups []AddressGroupReference

	Meta Meta
}

// ServiceRef represents a reference to a Service
type ServiceRef = netguardv1beta1.NamespacedObjectReference

// ServiceRefKey generates a key from ServiceRef for maps
func ServiceRefKey(ref ServiceRef) string {
	return ref.Namespace + "/" + ref.Name
}

// NewServiceRef creates a new ServiceRef
func NewServiceRef(name string, opts ...ResourceIdentifierOption) ServiceRef {
	id := NewResourceIdentifier(name, opts...)
	return netguardv1beta1.NamespacedObjectReference{
		ObjectReference: netguardv1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Service",
			Name:       id.Name,
		},
		Namespace: id.Namespace,
	}
}

// AddressGroupReference represents a reference to an AddressGroup with source tracking
type AddressGroupReference struct {
	Ref    netguardv1beta1.NamespacedObjectReference
	Source AddressGroupRegistrationSource
}

// AddressGroupRegistrationSource indicates how an address group was registered
type AddressGroupRegistrationSource string

const (
	// AddressGroupSourceSpec indicates the address group was registered via Service.spec.addressGroups
	AddressGroupSourceSpec AddressGroupRegistrationSource = "spec"
	// AddressGroupSourceBinding indicates the address group was registered via AddressGroupBinding
	AddressGroupSourceBinding AddressGroupRegistrationSource = "binding"
)
