package models

// RuleS2S represents a rule between two services
type RuleS2S struct {
	SelfRef
	Traffic         Traffic
	ServiceLocalRef ServiceAliasRef
	ServiceRef      ServiceAliasRef
	IEAgAgRuleRefs  []ResourceIdentifier // Ссылки на созданные IEAGAG правила
}

// RuleS2SRef represents a reference to a RuleS2S
type RuleS2SRef struct {
	ResourceIdentifier
}

// NewRuleS2SRef creates a new RuleS2SRef
func NewRuleS2SRef(name string, opts ...ResourceIdentifierOption) RuleS2SRef {
	return RuleS2SRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}
