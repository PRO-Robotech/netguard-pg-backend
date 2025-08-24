package models

import (
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// RuleS2S represents a rule between two services
type RuleS2S struct {
	SelfRef
	Traffic         Traffic
	ServiceLocalRef v1beta1.NamespacedObjectReference   // Full object reference with apiVersion, kind, name, namespace
	ServiceRef      v1beta1.NamespacedObjectReference   // Full object reference with apiVersion, kind, name, namespace
	IEAgAgRuleRefs  []v1beta1.NamespacedObjectReference // Full object references for created IEAGAG rules
	Trace           bool                                // Whether to enable trace
	Meta            Meta
}

// ServiceLocalRefKey returns the key for the ServiceLocalRef (namespace/name)
func (r *RuleS2S) ServiceLocalRefKey() string {
	if r.ServiceLocalRef.Namespace == "" {
		return r.ServiceLocalRef.Name
	}
	return r.ServiceLocalRef.Namespace + "/" + r.ServiceLocalRef.Name
}

// ServiceRefKey returns the key for the ServiceRef (namespace/name)
func (r *RuleS2S) ServiceRefKey() string {
	if r.ServiceRef.Namespace == "" {
		return r.ServiceRef.Name
	}
	return r.ServiceRef.Namespace + "/" + r.ServiceRef.Name
}

// RuleS2SRef represents a reference to a RuleS2S
type RuleS2SRef struct {
	ResourceIdentifier
}

// NewRuleS2SRef creates a new RuleS2SRef
func NewRuleS2SRef(name string, opts ...ResourceIdentifierOption) RuleS2SRef {
	return RuleS2SRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}
