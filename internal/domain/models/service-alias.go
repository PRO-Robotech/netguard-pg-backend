package models

import (
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// ServiceAlias represents an alias for a service
type ServiceAlias struct {
	SelfRef
	ServiceRef v1beta1.NamespacedObjectReference // Full object reference with apiVersion, kind and namespace
	Meta       Meta
}

// ServiceRefKey returns the key for the ServiceRef (namespace/name)
func (a *ServiceAlias) ServiceRefKey() string {
	// ServiceRef has its own namespace now
	if a.ServiceRef.Namespace == "" {
		return a.ServiceRef.Name
	}
	return a.ServiceRef.Namespace + "/" + a.ServiceRef.Name
}

type ServiceAliasRef struct {
	ResourceIdentifier
}

// NewServiceAliasRef creates a new ServiceAliasRef
func NewServiceAliasRef(name string, opts ...ResourceIdentifierOption) ServiceAliasRef {
	return ServiceAliasRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}
