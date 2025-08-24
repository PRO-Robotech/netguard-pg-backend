package base

import (
	"fmt"
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// StandardErrorConverter provides standard backend error → K8s error conversions
// for better kubectl compatibility and consistent error handling across all resources.
type StandardErrorConverter struct {
	groupResource schema.GroupResource
}

// NewStandardErrorConverter creates a new StandardErrorConverter for a specific resource type
func NewStandardErrorConverter(group, resource string) *StandardErrorConverter {
	return &StandardErrorConverter{
		groupResource: schema.GroupResource{
			Group:    group,
			Resource: resource,
		},
	}
}

// ConvertBackendError converts backend errors to appropriate Kubernetes API errors
// This ensures consistent error handling and proper HTTP status codes for kubectl compatibility
func (c *StandardErrorConverter) ConvertBackendError(err error, resourceName string) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	// Convert common backend error patterns to Kubernetes API errors
	switch {
	case strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist"):
		return apierrors.NewNotFound(c.groupResource, resourceName)

	case strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "duplicate"):
		return apierrors.NewAlreadyExists(c.groupResource, resourceName)

	case strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "validation failed"):
		return apierrors.NewBadRequest(fmt.Sprintf("Invalid %s %s: %s", c.groupResource.Resource, resourceName, errMsg))

	case strings.Contains(errMsg, "forbidden") || strings.Contains(errMsg, "access denied"):
		return apierrors.NewForbidden(c.groupResource, resourceName, err)

	case strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "authentication"):
		return apierrors.NewUnauthorized(errMsg)

	case strings.Contains(errMsg, "conflict") || strings.Contains(errMsg, "resource version"):
		return apierrors.NewConflict(c.groupResource, resourceName, err)

	case strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded"):
		return apierrors.NewTimeoutError(errMsg, 0)

	case strings.Contains(errMsg, "service unavailable") || strings.Contains(errMsg, "backend unavailable"):
		return apierrors.NewServiceUnavailable(errMsg)

	case strings.Contains(errMsg, "too many requests") || strings.Contains(errMsg, "rate limit"):
		return apierrors.NewTooManyRequestsError(errMsg)

	case strings.Contains(errMsg, "method not allowed"):
		return apierrors.NewMethodNotSupported(c.groupResource, "")

	default:
		// Generic internal server error for unknown backend errors
		return apierrors.NewInternalError(err)
	}
}

// ConvertValidationError converts domain validation errors to Kubernetes validation errors
func (c *StandardErrorConverter) ConvertValidationError(err error, resourceName string, field string, badValue interface{}) error {
	if err == nil {
		return nil
	}

	return apierrors.NewBadRequest(fmt.Sprintf("Invalid %s %s field %s: %s (value: %v)", c.groupResource.Resource, resourceName, field, err.Error(), badValue))
}

// ConvertRequiredError converts missing required field errors to Kubernetes required field errors
func (c *StandardErrorConverter) ConvertRequiredError(resourceName string, field string) error {
	return apierrors.NewBadRequest(fmt.Sprintf("Required field %s is missing for %s %s", field, c.groupResource.Resource, resourceName))
}

// ConvertDuplicateError converts duplicate field errors to Kubernetes duplicate field errors
func (c *StandardErrorConverter) ConvertDuplicateError(resourceName string, field string, value interface{}) error {
	return apierrors.NewBadRequest(fmt.Sprintf("Duplicate value %v for field %s in %s %s", value, field, c.groupResource.Resource, resourceName))
}

// WrapBackendError wraps backend errors with additional context for debugging
// while preserving the original Kubernetes error classification
func (c *StandardErrorConverter) WrapBackendError(err error, resourceName, operation string) error {
	if err == nil {
		return nil
	}

	// If it's already a Kubernetes API error, preserve it but add context
	if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.ErrStatus.Code < 500 {
		return fmt.Errorf("operation %s on %s %s failed: %w", operation, c.groupResource.Resource, resourceName, err)
	}

	// Convert backend error and add context
	k8sError := c.ConvertBackendError(err, resourceName)
	return fmt.Errorf("operation %s on %s %s failed: %w", operation, c.groupResource.Resource, resourceName, k8sError)
}

// IsRetryableError determines if a backend error should trigger a retry
func (c *StandardErrorConverter) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Kubernetes API errors that should be retried
	if apierrors.IsTimeout(err) || apierrors.IsServiceUnavailable(err) ||
		apierrors.IsTooManyRequests(err) || apierrors.IsInternalError(err) {
		return true
	}

	// Backend-specific retryable error patterns
	errMsg := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"deadline exceeded",
		"service unavailable",
		"temporary failure",
		"rate limit",
		"too many requests",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// GetHTTPStatusCode extracts HTTP status code from Kubernetes API errors
func (c *StandardErrorConverter) GetHTTPStatusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}

	if statusErr, ok := err.(*apierrors.StatusError); ok {
		return int(statusErr.ErrStatus.Code)
	}

	// Default to 500 for unknown errors
	return http.StatusInternalServerError
}

// ErrorSummary provides a structured summary of an error for logging and debugging
type ErrorSummary struct {
	Type      string `json:"type"`
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Resource  string `json:"resource"`
	Field     string `json:"field,omitempty"`
	Retryable bool   `json:"retryable"`
}

// SummarizeError creates a structured summary of an error for logging and monitoring
func (c *StandardErrorConverter) SummarizeError(err error, resourceName string) ErrorSummary {
	if err == nil {
		return ErrorSummary{
			Type:      "none",
			Code:      http.StatusOK,
			Message:   "",
			Resource:  resourceName,
			Retryable: false,
		}
	}

	summary := ErrorSummary{
		Message:   err.Error(),
		Resource:  resourceName,
		Code:      c.GetHTTPStatusCode(err),
		Retryable: c.IsRetryableError(err),
	}

	// Determine error type from Kubernetes API error
	switch {
	case apierrors.IsNotFound(err):
		summary.Type = "NotFound"
	case apierrors.IsAlreadyExists(err):
		summary.Type = "AlreadyExists"
	case apierrors.IsBadRequest(err):
		summary.Type = "BadRequest"
	case apierrors.IsForbidden(err):
		summary.Type = "Forbidden"
	case apierrors.IsUnauthorized(err):
		summary.Type = "Unauthorized"
	case apierrors.IsConflict(err):
		summary.Type = "Conflict"
	case apierrors.IsTimeout(err):
		summary.Type = "Timeout"
	case apierrors.IsServiceUnavailable(err):
		summary.Type = "ServiceUnavailable"
	case apierrors.IsTooManyRequests(err):
		summary.Type = "TooManyRequests"
	case apierrors.IsInternalError(err):
		summary.Type = "InternalError"
	default:
		summary.Type = "Unknown"
	}

	return summary
}

// Standard error converters for all NetGuard resources
var (
	ServiceErrorConverter                   = NewStandardErrorConverter("netguard.sgroups.io", "services")
	AddressGroupErrorConverter              = NewStandardErrorConverter("netguard.sgroups.io", "addressgroups")
	AddressGroupBindingErrorConverter       = NewStandardErrorConverter("netguard.sgroups.io", "addressgroupbindings")
	AddressGroupPortMappingErrorConverter   = NewStandardErrorConverter("netguard.sgroups.io", "addressgroupportmappings")
	RuleS2SErrorConverter                   = NewStandardErrorConverter("netguard.sgroups.io", "rules2s")
	ServiceAliasErrorConverter              = NewStandardErrorConverter("netguard.sgroups.io", "servicealiases")
	AddressGroupBindingPolicyErrorConverter = NewStandardErrorConverter("netguard.sgroups.io", "addressgroupbindingpolicies")
	IEAgAgRuleErrorConverter                = NewStandardErrorConverter("netguard.sgroups.io", "ieagagrules")
	NetworkErrorConverter                   = NewStandardErrorConverter("netguard.sgroups.io", "networks")
	NetworkBindingErrorConverter            = NewStandardErrorConverter("netguard.sgroups.io", "networkbindings")
)
