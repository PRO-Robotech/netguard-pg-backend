package validation

import (
	"fmt"
	"strings"

	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// SelectorValidator provides validation for field selectors and label selectors
type SelectorValidator struct{}

// NewSelectorValidator creates a new SelectorValidator
func NewSelectorValidator() *SelectorValidator {
	return &SelectorValidator{}
}

// ValidateFieldSelectors validates a list of field selectors for a given resource type
func (v *SelectorValidator) ValidateFieldSelectors(resourceType string, selectors []*netguardpb.FieldSelector) error {
	if len(selectors) == 0 {
		return nil
	}

	// Limit the number of field selectors to prevent performance issues
	const maxFieldSelectors = 10
	if len(selectors) > maxFieldSelectors {
		return NewValidationError(fmt.Sprintf("too many field selectors: maximum %d allowed, got %d", maxFieldSelectors, len(selectors)))
	}

	for i, selector := range selectors {
		if err := v.validateFieldSelector(resourceType, selector, i); err != nil {
			return err
		}
	}

	return nil
}

// validateFieldSelector validates a single field selector
func (v *SelectorValidator) validateFieldSelector(resourceType string, selector *netguardpb.FieldSelector, index int) error {
	if selector == nil {
		return NewValidationError(fmt.Sprintf("field selector at index %d is nil", index))
	}

	// Validate field path
	if selector.Field == "" {
		return NewValidationError(fmt.Sprintf("field selector at index %d: field is required", index))
	}

	// Validate field path format (should be dot-separated, e.g., "metadata.name")
	if !isValidFieldPath(selector.Field) {
		return NewValidationError(fmt.Sprintf("field selector at index %d: invalid field path %q (must be dot-separated, e.g., 'metadata.name')", index, selector.Field))
	}

	// Validate operator
	if selector.Operator == netguardpb.FieldOperator_FIELD_OPERATOR_UNSPECIFIED {
		return NewValidationError(fmt.Sprintf("field selector at index %d: operator is required", index))
	}

	// Validate value based on operator
	if err := v.validateFieldSelectorValue(selector, index); err != nil {
		return err
	}

	return nil
}

// validateFieldSelectorValue validates the value based on the operator
func (v *SelectorValidator) validateFieldSelectorValue(selector *netguardpb.FieldSelector, index int) error {
	switch selector.Operator {
	case netguardpb.FieldOperator_FIELD_OPERATOR_EXISTS,
		netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EXISTS:
		// EXISTS and NOT_EXISTS don't need a value
		return nil

	case netguardpb.FieldOperator_FIELD_OPERATOR_IN,
		netguardpb.FieldOperator_FIELD_OPERATOR_NOT_IN:
		// IN and NOT_IN require at least one value in comma-separated list
		if selector.Value == "" {
			return NewValidationError(fmt.Sprintf("field selector at index %d: value is required for %s operator", index, selector.Operator))
		}
		values := strings.Split(selector.Value, ",")
		if len(values) == 0 || (len(values) == 1 && strings.TrimSpace(values[0]) == "") {
			return NewValidationError(fmt.Sprintf("field selector at index %d: at least one value is required for %s operator", index, selector.Operator))
		}
		// Limit number of values in IN/NOT_IN
		const maxInValues = 20
		if len(values) > maxInValues {
			return NewValidationError(fmt.Sprintf("field selector at index %d: too many values in %s operator (maximum %d, got %d)", index, selector.Operator, maxInValues, len(values)))
		}

	default:
		// All other operators require a value
		if selector.Value == "" {
			return NewValidationError(fmt.Sprintf("field selector at index %d: value is required for %s operator", index, selector.Operator))
		}
	}

	return nil
}

// ValidateLabelSelectors validates a list of label selectors
func (v *SelectorValidator) ValidateLabelSelectors(selectors []*netguardpb.LabelSelector) error {
	if len(selectors) == 0 {
		return nil
	}

	// Limit the number of label selectors to prevent performance issues
	const maxLabelSelectors = 10
	if len(selectors) > maxLabelSelectors {
		return NewValidationError(fmt.Sprintf("too many label selectors: maximum %d allowed, got %d", maxLabelSelectors, len(selectors)))
	}

	for i, selector := range selectors {
		if err := v.validateLabelSelector(selector, i); err != nil {
			return err
		}
	}

	return nil
}

// validateLabelSelector validates a single label selector
func (v *SelectorValidator) validateLabelSelector(selector *netguardpb.LabelSelector, index int) error {
	if selector == nil {
		return NewValidationError(fmt.Sprintf("label selector at index %d is nil", index))
	}

	// Validate key
	if selector.Key == "" {
		return NewValidationError(fmt.Sprintf("label selector at index %d: key is required", index))
	}

	// Validate key format (Kubernetes label key rules)
	if err := validateLabelKey(selector.Key); err != nil {
		return NewValidationError(fmt.Sprintf("label selector at index %d: %v", index, err))
	}

	// Validate operator
	if selector.Operator == netguardpb.LabelOperator_LABEL_OPERATOR_UNSPECIFIED {
		return NewValidationError(fmt.Sprintf("label selector at index %d: operator is required", index))
	}

	// Validate values based on operator
	if err := v.validateLabelSelectorValues(selector, index); err != nil {
		return err
	}

	return nil
}

// validateLabelSelectorValues validates the values based on the operator
func (v *SelectorValidator) validateLabelSelectorValues(selector *netguardpb.LabelSelector, index int) error {
	switch selector.Operator {
	case netguardpb.LabelOperator_LABEL_OPERATOR_EXISTS,
		netguardpb.LabelOperator_LABEL_OPERATOR_NOT_EXISTS:
		// EXISTS and NOT_EXISTS don't need values
		return nil

	case netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS,
		netguardpb.LabelOperator_LABEL_OPERATOR_NOT_EQUALS:
		// EQUALS and NOT_EQUALS require exactly one value
		if len(selector.Values) == 0 {
			return NewValidationError(fmt.Sprintf("label selector at index %d: value is required for %s operator", index, selector.Operator))
		}
		if len(selector.Values) > 1 {
			return NewValidationError(fmt.Sprintf("label selector at index %d: %s operator accepts only one value, got %d", index, selector.Operator, len(selector.Values)))
		}
		// Validate label value
		if err := validateLabelValue(selector.Values[0]); err != nil {
			return NewValidationError(fmt.Sprintf("label selector at index %d: %v", index, err))
		}

	case netguardpb.LabelOperator_LABEL_OPERATOR_IN,
		netguardpb.LabelOperator_LABEL_OPERATOR_NOT_IN:
		// IN and NOT_IN require at least one value
		if len(selector.Values) == 0 {
			return NewValidationError(fmt.Sprintf("label selector at index %d: at least one value is required for %s operator", index, selector.Operator))
		}
		// Limit number of values
		const maxInValues = 20
		if len(selector.Values) > maxInValues {
			return NewValidationError(fmt.Sprintf("label selector at index %d: too many values in %s operator (maximum %d, got %d)", index, selector.Operator, maxInValues, len(selector.Values)))
		}
		// Validate each label value
		for j, value := range selector.Values {
			if err := validateLabelValue(value); err != nil {
				return NewValidationError(fmt.Sprintf("label selector at index %d, value at index %d: %v", index, j, err))
			}
		}

	default:
		return NewValidationError(fmt.Sprintf("label selector at index %d: unsupported operator %s", index, selector.Operator))
	}

	return nil
}

// isValidFieldPath checks if the field path is valid (dot-separated)
func isValidFieldPath(path string) bool {
	if path == "" {
		return false
	}
	parts := strings.Split(path, ".")
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	return true
}

// validateLabelKey validates a Kubernetes label key
// Label keys must be 63 characters or less and consist of alphanumeric characters, '-', '_' or '.'
// and must start and end with an alphanumeric character
func validateLabelKey(key string) error {
	if key == "" {
		return fmt.Errorf("label key cannot be empty")
	}

	// Check length
	const maxLabelKeyLength = 63
	if len(key) > maxLabelKeyLength {
		return fmt.Errorf("label key %q is too long (maximum %d characters, got %d)", key, maxLabelKeyLength, len(key))
	}

	// Check format (simplified - full Kubernetes validation is more complex with prefixes)
	if !isValidLabelKeyOrValue(key) {
		return fmt.Errorf("label key %q contains invalid characters (must be alphanumeric, '-', '_', or '.')", key)
	}

	return nil
}

// validateLabelValue validates a Kubernetes label value
// Label values must be 63 characters or less and consist of alphanumeric characters, '-', '_' or '.'
func validateLabelValue(value string) error {
	// Empty values are allowed in Kubernetes
	if value == "" {
		return nil
	}

	// Check length
	const maxLabelValueLength = 63
	if len(value) > maxLabelValueLength {
		return fmt.Errorf("label value %q is too long (maximum %d characters, got %d)", value, maxLabelValueLength, len(value))
	}

	// Check format
	if !isValidLabelKeyOrValue(value) {
		return fmt.Errorf("label value %q contains invalid characters (must be alphanumeric, '-', '_', or '.')", value)
	}

	return nil
}

// isValidLabelKeyOrValue checks if a string is valid for label key or value
func isValidLabelKeyOrValue(s string) bool {
	if s == "" {
		return true
	}
	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_' || ch == '.') {
			return false
		}
	}
	return true
}
