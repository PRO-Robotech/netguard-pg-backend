package models

// ServiceAlias represents an alias for a service
type ServiceAlias struct {
	SelfRef
	ServiceRef ServiceRef
	Meta       Meta
}

type ServiceAliasRef struct {
	ResourceIdentifier
}

// NewServiceAliasRef creates a new ServiceAliasRef
func NewServiceAliasRef(name string, opts ...ResourceIdentifierOption) ServiceAliasRef {
	return ServiceAliasRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}
