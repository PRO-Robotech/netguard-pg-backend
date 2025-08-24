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
	Meta          Meta
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
