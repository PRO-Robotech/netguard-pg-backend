package conditions

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This file contains common helper functions shared across entity-specific condition processing.
// These functions can be used to reduce code duplication in entity files.
//
// Future enhancements:
// - extractResourceIdentifier(resource) - extract namespace/name from any resource
// - commonSavePattern(ctx, cm, saveFunc) - common save pattern wrapper
// - validateReferenceExists(ctx, reader, ref) - common reference validation

// formatConditions converts conditions slice to readable string for logging
// Returns format: "Type1=Status1(Reason1), Type2=Status2(Reason2), ..."
// Empty slice returns "EMPTY"
func formatConditions(conditions []metav1.Condition) string {
	if len(conditions) == 0 {
		return "EMPTY"
	}
	parts := make([]string, 0, len(conditions))
	for _, c := range conditions {
		parts = append(parts, fmt.Sprintf("%s=%s(%s)", c.Type, c.Status, c.Reason))
	}
	return strings.Join(parts, ", ")
}

// getConditionStatus safely extracts status from condition
// Returns "NIL" if condition is nil
func getConditionStatus(cond *metav1.Condition) string {
	if cond == nil {
		return "NIL"
	}
	return string(cond.Status)
}

// getConditionReason safely extracts reason from condition
// Returns "NIL" if condition is nil
func getConditionReason(cond *metav1.Condition) string {
	if cond == nil {
		return "NIL"
	}
	return cond.Reason
}
