package models

// IEAgAgRule представляет правило между двумя группами адресов
type IEAgAgRule struct {
	SelfRef
	Transport         TransportProtocol
	Traffic           Traffic
	AddressGroupLocal AddressGroupRef
	AddressGroup      AddressGroupRef
	Ports             []PortSpec
	Action            RuleAction
	Logs              bool
	Priority          int32
	Meta              Meta
}

// PortSpec определяет спецификацию портов
type PortSpec struct {
	Source      string // Опционально, порт источника
	Destination string // Порт назначения
}

// RuleAction представляет действие правила
type RuleAction string

const (
	ActionAccept RuleAction = "ACCEPT"
	ActionDrop   RuleAction = "DROP"
)

// IEAgAgRuleRef представляет ссылку на IEAgAgRule
type IEAgAgRuleRef struct {
	ResourceIdentifier
}

// NewIEAgAgRuleRef создает новую ссылку на IEAgAgRule
func NewIEAgAgRuleRef(name string, opts ...ResourceIdentifierOption) IEAgAgRuleRef {
	return IEAgAgRuleRef{ResourceIdentifier: NewResourceIdentifier(name, opts...)}
}
