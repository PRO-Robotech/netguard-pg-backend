package base

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"

	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// ParseFieldSelectors converts Kubernetes field selector string to protobuf FieldSelector messages.
// Input format: "metadata.name=test,status.phase!=Running"
// Output: []*netguardpb.FieldSelector
func ParseFieldSelectors(selectorStr string) ([]*netguardpb.FieldSelector, error) {
	if selectorStr == "" {
		return nil, nil
	}

	// Parse using K8s fields.Selector
	selector, err := fields.ParseSelector(selectorStr)
	if err != nil {
		return nil, fmt.Errorf("invalid field selector: %w", err)
	}

	// Convert K8s selector to our protobuf format
	requirements := selector.Requirements()
	result := make([]*netguardpb.FieldSelector, 0, len(requirements))

	for _, req := range requirements {
		fieldSelector := &netguardpb.FieldSelector{
			Field: req.Field,
			Value: req.Value,
		}

		// Map K8s operator to our operator
		// K8s fields.Selector uses string operators: "=", "==", "!="
		switch req.Operator {
		case "=", "==":
			fieldSelector.Operator = netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS
		case "!=":
			fieldSelector.Operator = netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EQUALS
		default:
			// K8s only supports = and !=
			return nil, fmt.Errorf("unsupported field operator: %v", req.Operator)
		}

		result = append(result, fieldSelector)
	}

	return result, nil
}

// ParseLabelSelectors converts Kubernetes label selector to protobuf LabelSelector messages.
// Supports both equality-based and set-based selectors:
// - Equality: "env=prod,tier=frontend"
// - Set-based: "env in (prod,staging),tier notin (cache)"
// - Exists: "version"
// - Not exists: "!deprecated"
func ParseLabelSelectors(selectorStr string) ([]*netguardpb.LabelSelector, error) {
	if selectorStr == "" {
		return nil, nil
	}

	// Parse using K8s labels.Selector
	selector, err := labels.Parse(selectorStr)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %w", err)
	}

	// Convert K8s selector to our protobuf format
	requirements, _ := selector.Requirements()
	result := make([]*netguardpb.LabelSelector, 0, len(requirements))

	for _, req := range requirements {
		labelSelector := &netguardpb.LabelSelector{
			Key: req.Key(),
		}

		// Map K8s operator to our operator
		// Use string comparison as K8s labels.Requirement.Operator() returns selection.Operator type
		op := req.Operator()
		opStr := string(op)

		switch opStr {
		case "=", "==":
			labelSelector.Operator = netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS
			values := req.Values()
			if values.Len() > 0 {
				labelSelector.Values = values.List()
			}

		case "!=":
			labelSelector.Operator = netguardpb.LabelOperator_LABEL_OPERATOR_NOT_EQUALS
			values := req.Values()
			if values.Len() > 0 {
				labelSelector.Values = values.List()
			}

		case "in":
			labelSelector.Operator = netguardpb.LabelOperator_LABEL_OPERATOR_IN
			values := req.Values()
			if values.Len() > 0 {
				labelSelector.Values = values.List()
			}

		case "notin":
			labelSelector.Operator = netguardpb.LabelOperator_LABEL_OPERATOR_NOT_IN
			values := req.Values()
			if values.Len() > 0 {
				labelSelector.Values = values.List()
			}

		case "exists":
			labelSelector.Operator = netguardpb.LabelOperator_LABEL_OPERATOR_EXISTS
			labelSelector.Values = []string{} // Empty for exists

		case "!":
			labelSelector.Operator = netguardpb.LabelOperator_LABEL_OPERATOR_NOT_EXISTS
			labelSelector.Values = []string{} // Empty for not exists

		default:
			return nil, fmt.Errorf("unsupported label operator: %v", req.Operator())
		}

		result = append(result, labelSelector)
	}

	return result, nil
}

// ConvertListOptionsToProto converts Kubernetes ListOptions to protobuf ListOptions.
// This is the main entry point for converting K8s API requests to backend gRPC calls.
func ConvertListOptionsToProto(k8sOptions interface{}) (*netguardpb.ListOptions, error) {
	if k8sOptions == nil {
		return nil, nil
	}

	// Type assertion to get field selector and label selector strings
	// Note: This assumes internalversion.ListOptions or metav1.ListOptions
	var fieldSelectorStr, labelSelectorStr string
	var limit int64
	var continueToken string

	// Try to extract using reflection or type switch
	// This is a simplified version - you may need to adjust based on actual K8s types
	switch opts := k8sOptions.(type) {
	case interface{ GetFieldSelector() string }:
		fieldSelectorStr = opts.GetFieldSelector()
	}

	switch opts := k8sOptions.(type) {
	case interface{ GetLabelSelector() string }:
		labelSelectorStr = opts.GetLabelSelector()
	}

	switch opts := k8sOptions.(type) {
	case interface{ GetLimit() int64 }:
		limit = opts.GetLimit()
	}

	switch opts := k8sOptions.(type) {
	case interface{ GetContinue() string }:
		continueToken = opts.GetContinue()
	}

	// Parse field selectors
	fieldSelectors, err := ParseFieldSelectors(fieldSelectorStr)
	if err != nil {
		return nil, err
	}

	// Parse label selectors
	labelSelectors, err := ParseLabelSelectors(labelSelectorStr)
	if err != nil {
		return nil, err
	}

	// Build protobuf ListOptions
	protoOptions := &netguardpb.ListOptions{
		FieldSelectors: fieldSelectors,
		LabelSelectors: labelSelectors,
		Limit:          int32(limit),
		ContinueToken:  continueToken,
	}

	return protoOptions, nil
}

// ExtendedFieldSelectorParser provides extended operators beyond K8s standard.
// Use this for custom field selector syntax that supports CONTAINS, STARTS_WITH, etc.
type ExtendedFieldSelectorParser struct{}

// ParseExtendedFieldSelector parses custom field selector syntax with extended operators.
// Syntax examples:
//   - metadata.name=test (EQUALS)
//   - metadata.name!=test (NOT_EQUALS)
//   - metadata.name~=test (CONTAINS)
//   - metadata.name^=test (STARTS_WITH)
//   - metadata.name$=test (ENDS_WITH)
//   - metadata.name:exists (EXISTS)
//   - metadata.name:!exists (NOT_EXISTS)
//   - metadata.namespace@=prod,staging (IN)
//   - metadata.namespace@!=test,dev (NOT_IN)
func (p *ExtendedFieldSelectorParser) Parse(selectorStr string) ([]*netguardpb.FieldSelector, error) {
	if selectorStr == "" {
		return nil, nil
	}

	// Split by comma for multiple selectors
	parts := strings.Split(selectorStr, ",")
	result := make([]*netguardpb.FieldSelector, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		fieldSelector, err := p.parseSingleSelector(part)
		if err != nil {
			return nil, err
		}
		result = append(result, fieldSelector)
	}

	return result, nil
}

func (p *ExtendedFieldSelectorParser) parseSingleSelector(selector string) (*netguardpb.FieldSelector, error) {
	// Try different operators in order of specificity
	operators := []struct {
		pattern  string
		operator netguardpb.FieldOperator
	}{
		{"@!=", netguardpb.FieldOperator_FIELD_OPERATOR_NOT_IN},
		{"@=", netguardpb.FieldOperator_FIELD_OPERATOR_IN},
		{"!=", netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EQUALS},
		{"~=", netguardpb.FieldOperator_FIELD_OPERATOR_CONTAINS},
		{"^=", netguardpb.FieldOperator_FIELD_OPERATOR_STARTS_WITH},
		{"$=", netguardpb.FieldOperator_FIELD_OPERATOR_ENDS_WITH},
		{"=", netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS},
		{":exists", netguardpb.FieldOperator_FIELD_OPERATOR_EXISTS},
		{":!exists", netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EXISTS},
	}

	for _, op := range operators {
		if idx := strings.Index(selector, op.pattern); idx != -1 {
			field := strings.TrimSpace(selector[:idx])
			value := ""
			if op.operator != netguardpb.FieldOperator_FIELD_OPERATOR_EXISTS &&
				op.operator != netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EXISTS {
				value = strings.TrimSpace(selector[idx+len(op.pattern):])
			}

			return &netguardpb.FieldSelector{
				Field:    field,
				Operator: op.operator,
				Value:    value,
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid field selector syntax: %s", selector)
}
