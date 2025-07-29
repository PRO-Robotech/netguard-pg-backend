package models

import (
	"fmt"

	"netguard-pg-backend/internal/sync/types"
)

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

// SyncableEntity interface implementation for IEAgAgRule

// GetSyncSubjectType returns the sync subject type for IEAgAgRule
func (r *IEAgAgRule) GetSyncSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeIEAgAgRules
}

// GetSyncKey returns a unique key for the IEAgAgRule
func (r *IEAgAgRule) GetSyncKey() string {
	if r.Namespace != "" {
		return fmt.Sprintf("ieagagrule-%s/%s", r.Namespace, r.Name)
	}
	return fmt.Sprintf("ieagagrule-%s", r.Name)
}

// ToSGroupsProto converts the IEAgAgRule to sgroups protobuf format
func (r *IEAgAgRule) ToSGroupsProto() (interface{}, error) {
	// For IEAgAgRule, we return the rule itself as it's already in the format
	// expected by sgroups. The actual protobuf conversion will be handled by the syncer.
	return r, nil
}
