package ports

import (
	"fmt"
	"strings"

	"netguard-pg-backend/internal/domain/models"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// EmptyScope represents an empty scope
type EmptyScope struct{}

// IsEmpty returns true for EmptyScope
func (EmptyScope) IsEmpty() bool {
	return true
}

// String returns a string representation of EmptyScope
func (EmptyScope) String() string {
	return "empty"
}

// ResourceIdentifierScope represents a scope with resource identifiers
type ResourceIdentifierScope struct {
	Identifiers []models.ResourceIdentifier
}

// IsEmpty returns true if ResourceIdentifierScope is empty
func (s ResourceIdentifierScope) IsEmpty() bool {
	return len(s.Identifiers) == 0
}

// String returns a string representation of ResourceIdentifierScope
func (s ResourceIdentifierScope) String() string {
	if s.IsEmpty() {
		return "empty"
	}

	identifiers := make([]string, 0, len(s.Identifiers))
	for _, id := range s.Identifiers {
		identifiers = append(identifiers, id.Key())
	}

	return fmt.Sprintf("identifiers(%s)", strings.Join(identifiers, ","))
}

// NewResourceIdentifierScope creates a new ResourceIdentifierScope
func NewResourceIdentifierScope(identifiers ...models.ResourceIdentifier) ResourceIdentifierScope {
	return ResourceIdentifierScope{
		Identifiers: identifiers,
	}
}

// FieldSelectorScope represents a scope with resource identifiers, field selectors, and label selectors
type FieldSelectorScope struct {
	Identifiers    []models.ResourceIdentifier
	FieldSelectors []*netguardpb.FieldSelector
	LabelSelectors []*netguardpb.LabelSelector
}

// IsEmpty returns true if Identifiers, FieldSelectors, and LabelSelectors are all empty
func (s FieldSelectorScope) IsEmpty() bool {
	return len(s.Identifiers) == 0 && len(s.FieldSelectors) == 0 && len(s.LabelSelectors) == 0
}

// String returns a string representation of FieldSelectorScope
func (s FieldSelectorScope) String() string {
	if s.IsEmpty() {
		return "empty"
	}

	var parts []string

	if len(s.Identifiers) > 0 {
		identifiers := make([]string, 0, len(s.Identifiers))
		for _, id := range s.Identifiers {
			identifiers = append(identifiers, id.Key())
		}
		parts = append(parts, fmt.Sprintf("identifiers(%s)", strings.Join(identifiers, ",")))
	}

	if len(s.FieldSelectors) > 0 {
		selectors := make([]string, 0, len(s.FieldSelectors))
		for _, fs := range s.FieldSelectors {
			selectors = append(selectors, fmt.Sprintf("%s%s%s", fs.Field, operatorToString(fs.Operator), fs.Value))
		}
		parts = append(parts, fmt.Sprintf("field_selectors(%s)", strings.Join(selectors, ",")))
	}

	if len(s.LabelSelectors) > 0 {
		selectors := make([]string, 0, len(s.LabelSelectors))
		for _, ls := range s.LabelSelectors {
			selectors = append(selectors, fmt.Sprintf("%s%s%v", ls.Key, labelOperatorToString(ls.Operator), ls.Values))
		}
		parts = append(parts, fmt.Sprintf("label_selectors(%s)", strings.Join(selectors, ",")))
	}

	return strings.Join(parts, " AND ")
}

// operatorToString converts FieldOperator to string
func operatorToString(op netguardpb.FieldOperator) string {
	switch op {
	case netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS:
		return "="
	case netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EQUALS:
		return "!="
	default:
		return "="
	}
}

// labelOperatorToString converts LabelOperator to string
func labelOperatorToString(op netguardpb.LabelOperator) string {
	switch op {
	case netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS:
		return "="
	case netguardpb.LabelOperator_LABEL_OPERATOR_NOT_EQUALS:
		return "!="
	case netguardpb.LabelOperator_LABEL_OPERATOR_IN:
		return " in "
	case netguardpb.LabelOperator_LABEL_OPERATOR_NOT_IN:
		return " notin "
	case netguardpb.LabelOperator_LABEL_OPERATOR_EXISTS:
		return " exists"
	case netguardpb.LabelOperator_LABEL_OPERATOR_NOT_EXISTS:
		return " not exists"
	default:
		return "="
	}
}

// NewFieldSelectorScope creates a new FieldSelectorScope
func NewFieldSelectorScope(identifiers []models.ResourceIdentifier, fieldSelectors []*netguardpb.FieldSelector) FieldSelectorScope {
	return FieldSelectorScope{
		Identifiers:    identifiers,
		FieldSelectors: fieldSelectors,
	}
}
