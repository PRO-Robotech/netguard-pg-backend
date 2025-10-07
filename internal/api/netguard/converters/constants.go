package converters

import "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

// API Version constants
const (
	// NetguardAPIVersion represents the current API version for netguard resources
	NetguardAPIVersion = "netguard.sgroups.io/v1beta1"

	// APIGroup represents the API group for netguard resources
	APIGroup = "netguard.sgroups.io"

	// APIVersion represents the current version
	APIVersion = "v1beta1"
)

// Resource Kind constants
const (
	// KindAddressGroup represents AddressGroup resource kind
	KindAddressGroup = "AddressGroup"

	// KindNetwork represents Network resource kind
	KindNetwork = "Network"

	// KindService represents Service resource kind
	KindService = "Service"

	// KindIEAgAgRule represents IEAgAgRule resource kind
	KindIEAgAgRule = "IEAgAgRule"

	// KindHost represents Host resource kind
	KindHost = "Host"

	// KindNetworkBinding represents NetworkBinding resource kind
	KindNetworkBinding = "NetworkBinding"

	// KindAddressGroupBinding represents AddressGroupBinding resource kind
	KindAddressGroupBinding = "AddressGroupBinding"
)

// Helper functions for creating ObjectReference

// NewObjectReference creates a new ObjectReference with the standard API version
func NewObjectReference(kind, name string) v1beta1.ObjectReference {
	return v1beta1.ObjectReference{
		APIVersion: NetguardAPIVersion,
		Kind:       kind,
		Name:       name,
	}
}

// NewNamespacedObjectReference creates a new NamespacedObjectReference with the standard API version
func NewNamespacedObjectReference(kind, name, namespace string) v1beta1.NamespacedObjectReference {
	return v1beta1.NamespacedObjectReference{
		ObjectReference: v1beta1.ObjectReference{
			APIVersion: NetguardAPIVersion,
			Kind:       kind,
			Name:       name,
		},
		Namespace: namespace,
	}
}
