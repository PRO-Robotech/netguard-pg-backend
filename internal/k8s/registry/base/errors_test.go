package base

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestStandardErrorConverter_ConvertBackendError(t *testing.T) {
	converter := NewStandardErrorConverter("netguard.sgroups.io", "services")
	resourceName := "test-service"

	testCases := []struct {
		name           string
		backendError   error
		expectedType   func(error) bool
		expectedStatus int
	}{
		{
			name:           "not_found_error",
			backendError:   errors.New("service not found"),
			expectedType:   apierrors.IsNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "already_exists_error",
			backendError:   errors.New("service already exists"),
			expectedType:   apierrors.IsAlreadyExists,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "validation_error",
			backendError:   errors.New("invalid CIDR format"),
			expectedType:   apierrors.IsBadRequest,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "forbidden_error",
			backendError:   errors.New("access denied to resource"),
			expectedType:   apierrors.IsForbidden,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "unauthorized_error",
			backendError:   errors.New("authentication failed"),
			expectedType:   apierrors.IsUnauthorized,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "conflict_error",
			backendError:   errors.New("resource version conflict"),
			expectedType:   apierrors.IsConflict,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "timeout_error",
			backendError:   errors.New("request timeout"),
			expectedType:   apierrors.IsTimeout,
			expectedStatus: http.StatusGatewayTimeout,
		},
		{
			name:           "service_unavailable_error",
			backendError:   errors.New("backend unavailable"),
			expectedType:   apierrors.IsServiceUnavailable,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:           "rate_limit_error",
			backendError:   errors.New("too many requests"),
			expectedType:   apierrors.IsTooManyRequests,
			expectedStatus: http.StatusTooManyRequests,
		},
		{
			name:           "method_not_allowed_error",
			backendError:   errors.New("method not allowed"),
			expectedType:   apierrors.IsMethodNotSupported,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "unknown_error",
			backendError:   errors.New("unexpected database error"),
			expectedType:   apierrors.IsInternalError,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "nil_error",
			backendError:   nil,
			expectedType:   nil,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := converter.ConvertBackendError(tc.backendError, resourceName)

			if tc.expectedType == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.True(t, tc.expectedType(result), "Error type mismatch for %s", tc.name)
				assert.Equal(t, tc.expectedStatus, converter.GetHTTPStatusCode(result))
			}
		})
	}
}

func TestStandardErrorConverter_ConvertValidationError(t *testing.T) {
	converter := NewStandardErrorConverter("netguard.sgroups.io", "services")
	resourceName := "test-service"

	testCases := []struct {
		name        string
		inputError  error
		field       string
		badValue    interface{}
		expectError bool
	}{
		{
			name:        "validation_error",
			inputError:  errors.New("Invalid port number"),
			field:       "spec.ports[0].port",
			badValue:    "invalid-port",
			expectError: true,
		},
		{
			name:        "nil_error",
			inputError:  nil,
			field:       "spec.field",
			badValue:    "value",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := converter.ConvertValidationError(tc.inputError, resourceName, tc.field, tc.badValue)

			if tc.expectError {
				require.NotNil(t, result)
				assert.True(t, apierrors.IsBadRequest(result))

				// Since we simplified to use BadRequest, just check the message contains expected content
				assert.Contains(t, result.Error(), tc.field)
				assert.Contains(t, result.Error(), "Invalid")
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func TestStandardErrorConverter_ConvertRequiredError(t *testing.T) {
	converter := NewStandardErrorConverter("netguard.sgroups.io", "services")
	resourceName := "test-service"
	field := "spec.cidr"

	result := converter.ConvertRequiredError(resourceName, field)

	require.NotNil(t, result)
	assert.True(t, apierrors.IsBadRequest(result))

	// Check error message contains expected content
	assert.Contains(t, result.Error(), field)
	assert.Contains(t, result.Error(), "Required field")
}

func TestStandardErrorConverter_ConvertDuplicateError(t *testing.T) {
	converter := NewStandardErrorConverter("netguard.sgroups.io", "services")
	resourceName := "test-service"
	field := "spec.name"
	value := "duplicate-name"

	result := converter.ConvertDuplicateError(resourceName, field, value)

	require.NotNil(t, result)
	assert.True(t, apierrors.IsBadRequest(result))

	// Check error message contains expected content
	assert.Contains(t, result.Error(), field)
	assert.Contains(t, result.Error(), "Duplicate")
	assert.Contains(t, result.Error(), "duplicate-name")
}

func TestStandardErrorConverter_IsRetryableError(t *testing.T) {
	converter := NewStandardErrorConverter("netguard.sgroups.io", "services")

	testCases := []struct {
		name      string
		error     error
		retryable bool
	}{
		{
			name:      "timeout_error",
			error:     apierrors.NewTimeoutError("timeout", 0),
			retryable: true,
		},
		{
			name:      "service_unavailable_error",
			error:     apierrors.NewServiceUnavailable("service unavailable"),
			retryable: true,
		},
		{
			name:      "too_many_requests_error",
			error:     apierrors.NewTooManyRequestsError("rate limit exceeded"),
			retryable: true,
		},
		{
			name:      "internal_error",
			error:     apierrors.NewInternalError(errors.New("internal error")),
			retryable: true,
		},
		{
			name:      "connection_refused_error",
			error:     errors.New("connection refused"),
			retryable: true,
		},
		{
			name:      "not_found_error",
			error:     apierrors.NewNotFound(schema.GroupResource{}, "test"),
			retryable: false,
		},
		{
			name:      "validation_error",
			error:     apierrors.NewBadRequest("bad request"),
			retryable: false,
		},
		{
			name:      "nil_error",
			error:     nil,
			retryable: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := converter.IsRetryableError(tc.error)
			assert.Equal(t, tc.retryable, result)
		})
	}
}

func TestStandardErrorConverter_GetHTTPStatusCode(t *testing.T) {
	converter := NewStandardErrorConverter("netguard.sgroups.io", "services")

	testCases := []struct {
		name           string
		error          error
		expectedStatus int
	}{
		{
			name:           "not_found",
			error:          apierrors.NewNotFound(schema.GroupResource{}, "test"),
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "bad_request",
			error:          apierrors.NewBadRequest("bad request"),
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "forbidden",
			error:          apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")),
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "nil_error",
			error:          nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "unknown_error",
			error:          errors.New("unknown error"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := converter.GetHTTPStatusCode(tc.error)
			assert.Equal(t, tc.expectedStatus, result)
		})
	}
}

func TestStandardErrorConverter_SummarizeError(t *testing.T) {
	converter := NewStandardErrorConverter("netguard.sgroups.io", "services")
	resourceName := "test-service"

	testCases := []struct {
		name         string
		error        error
		expectedType string
		expectedCode int
	}{
		{
			name:         "not_found",
			error:        apierrors.NewNotFound(schema.GroupResource{}, resourceName),
			expectedType: "NotFound",
			expectedCode: http.StatusNotFound,
		},
		{
			name:         "already_exists",
			error:        apierrors.NewAlreadyExists(schema.GroupResource{}, resourceName),
			expectedType: "AlreadyExists",
			expectedCode: http.StatusConflict,
		},
		{
			name:         "forbidden",
			error:        apierrors.NewForbidden(schema.GroupResource{}, resourceName, errors.New("forbidden")),
			expectedType: "Forbidden",
			expectedCode: http.StatusForbidden,
		},
		{
			name:         "nil_error",
			error:        nil,
			expectedType: "none",
			expectedCode: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			summary := converter.SummarizeError(tc.error, resourceName)

			assert.Equal(t, tc.expectedType, summary.Type)
			assert.Equal(t, tc.expectedCode, summary.Code)
			assert.Equal(t, resourceName, summary.Resource)

			if tc.error == nil {
				assert.Empty(t, summary.Message)
				assert.False(t, summary.Retryable)
			} else {
				assert.NotEmpty(t, summary.Message)
			}
		})
	}
}

func TestStandardErrorConverter_WrapBackendError(t *testing.T) {
	converter := NewStandardErrorConverter("netguard.sgroups.io", "services")
	resourceName := "test-service"
	operation := "create"

	testCases := []struct {
		name           string
		inputError     error
		expectWrapped  bool
		expectContains []string
	}{
		{
			name:           "backend_error",
			inputError:     errors.New("database connection failed"),
			expectWrapped:  true,
			expectContains: []string{"operation create", "services test-service", "failed"},
		},
		{
			name:           "kubernetes_error",
			inputError:     apierrors.NewNotFound(schema.GroupResource{}, resourceName),
			expectWrapped:  true,
			expectContains: []string{"operation create", "services test-service", "failed"},
		},
		{
			name:          "nil_error",
			inputError:    nil,
			expectWrapped: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := converter.WrapBackendError(tc.inputError, resourceName, operation)

			if tc.expectWrapped {
				require.NotNil(t, result)
				resultMsg := result.Error()
				for _, expected := range tc.expectContains {
					assert.Contains(t, resultMsg, expected)
				}
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func TestStandardErrorConverters_Constants(t *testing.T) {
	// Test that all pre-configured error converters are properly initialized
	converters := map[string]*StandardErrorConverter{
		"ServiceErrorConverter":                   ServiceErrorConverter,
		"AddressGroupErrorConverter":              AddressGroupErrorConverter,
		"AddressGroupBindingErrorConverter":       AddressGroupBindingErrorConverter,
		"AddressGroupPortMappingErrorConverter":   AddressGroupPortMappingErrorConverter,
		"RuleS2SErrorConverter":                   RuleS2SErrorConverter,
		"ServiceAliasErrorConverter":              ServiceAliasErrorConverter,
		"AddressGroupBindingPolicyErrorConverter": AddressGroupBindingPolicyErrorConverter,
		"IEAgAgRuleErrorConverter":                IEAgAgRuleErrorConverter,
		"NetworkErrorConverter":                   NetworkErrorConverter,
		"NetworkBindingErrorConverter":            NetworkBindingErrorConverter,
	}

	for name, converter := range converters {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, converter, "Converter %s should not be nil", name)
			assert.Equal(t, "netguard.sgroups.io", converter.groupResource.Group)
			assert.NotEmpty(t, converter.groupResource.Resource)

			// Test that error conversion works
			testError := errors.New("test backend error")
			result := converter.ConvertBackendError(testError, "test-resource")
			require.NotNil(t, result)
			assert.True(t, apierrors.IsInternalError(result))
		})
	}
}
