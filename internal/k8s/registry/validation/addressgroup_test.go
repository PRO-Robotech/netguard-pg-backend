package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestAddressGroupValidator_ValidateCreate(t *testing.T) {
	ctx := context.Background()
	validator := NewAddressGroupValidator()

	testCases := []struct {
		name           string
		addressGroup   *v1beta1.AddressGroup
		expectedErrors int
	}{
		{
			name: "valid minimal addressgroup",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupSpec{
					DefaultAction: v1beta1.ActionAccept,
				},
			},
			expectedErrors: 0,
		},
		{
			name: "valid full addressgroup",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-address-group",
					Namespace: "test-namespace",
				},
				Spec: v1beta1.AddressGroupSpec{
					DefaultAction: v1beta1.ActionDrop,
					Logs:          true,
					Trace:         true,
				},
			},
			expectedErrors: 0,
		},
		{
			name:           "nil addressgroup",
			addressGroup:   nil,
			expectedErrors: 1,
		},
		{
			name: "missing name",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupSpec{
					DefaultAction: v1beta1.ActionAccept,
				},
			},
			expectedErrors: 1,
		},
		{
			name: "invalid name format",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Test_Invalid_Name!",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupSpec{
					DefaultAction: v1beta1.ActionAccept,
				},
			},
			expectedErrors: 1,
		},
		{
			name: "invalid namespace format",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "Test_Invalid_Namespace!",
				},
				Spec: v1beta1.AddressGroupSpec{
					DefaultAction: v1beta1.ActionAccept,
				},
			},
			expectedErrors: 1,
		},
		{
			name: "missing default action",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupSpec{
					// DefaultAction missing
				},
			},
			expectedErrors: 1,
		},
		{
			name: "invalid default action",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupSpec{
					DefaultAction: "INVALID",
				},
			},
			expectedErrors: 1,
		},
		{
			name: "multiple validation errors",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Test_Invalid_Name!",
					Namespace: "Test_Invalid_Namespace!",
				},
				Spec: v1beta1.AddressGroupSpec{
					DefaultAction: "INVALID",
				},
			},
			expectedErrors: 3, // invalid name + invalid namespace + invalid action
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validator.ValidateCreate(ctx, tc.addressGroup)
			assert.Len(t, errs, tc.expectedErrors,
				"Expected %d errors, got %d: %v", tc.expectedErrors, len(errs), errs)
		})
	}
}

func TestAddressGroupValidator_ValidateUpdate(t *testing.T) {
	ctx := context.Background()
	validator := NewAddressGroupValidator()

	validOld := &v1beta1.AddressGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ag",
			Namespace: "default",
		},
		Spec: v1beta1.AddressGroupSpec{
			DefaultAction: v1beta1.ActionAccept,
		},
	}

	testCases := []struct {
		name            string
		addressGroup    *v1beta1.AddressGroup
		oldAddressGroup *v1beta1.AddressGroup
		expectedErrors  int
	}{
		{
			name: "valid update",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupSpec{
					DefaultAction: v1beta1.ActionDrop, // Changed from ACCEPT to DROP
					Logs:          true,               // Added logs
				},
			},
			oldAddressGroup: validOld,
			expectedErrors:  0,
		},
		{
			name: "invalid new object",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupSpec{
					DefaultAction: "INVALID",
				},
			},
			oldAddressGroup: validOld,
			expectedErrors:  1,
		},
		{
			name: "update with nil old object",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupSpec{
					DefaultAction: v1beta1.ActionAccept,
				},
			},
			oldAddressGroup: nil,
			expectedErrors:  0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validator.ValidateUpdate(ctx, tc.addressGroup, tc.oldAddressGroup)
			assert.Len(t, errs, tc.expectedErrors,
				"Expected %d errors, got %d: %v", tc.expectedErrors, len(errs), errs)
		})
	}
}

func TestAddressGroupValidator_ValidateDelete(t *testing.T) {
	ctx := context.Background()
	validator := NewAddressGroupValidator()

	testCases := []struct {
		name           string
		addressGroup   *v1beta1.AddressGroup
		expectedErrors int
	}{
		{
			name: "valid delete",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupSpec{
					DefaultAction: v1beta1.ActionAccept,
				},
			},
			expectedErrors: 0, // Delete always allowed for now
		},
		{
			name:           "delete nil object",
			addressGroup:   nil,
			expectedErrors: 0, // Delete always allowed for now
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validator.ValidateDelete(ctx, tc.addressGroup)
			assert.Len(t, errs, tc.expectedErrors,
				"Expected %d errors, got %d: %v", tc.expectedErrors, len(errs), errs)
		})
	}
}

func TestAddressGroupValidator_validateMetadata(t *testing.T) {
	validator := NewAddressGroupValidator()

	testCases := []struct {
		name            string
		addressGroup    *v1beta1.AddressGroup
		expectedErrors  int
		checkErrorPaths []string
	}{
		{
			name: "valid metadata",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
			},
			expectedErrors: 0,
		},
		{
			name: "missing name",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
			},
			expectedErrors:  1,
			checkErrorPaths: []string{"metadata.name"},
		},
		{
			name: "invalid name format",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Name!",
					Namespace: "default",
				},
			},
			expectedErrors:  1,
			checkErrorPaths: []string{"metadata.name"},
		},
		{
			name: "invalid namespace format",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "Invalid_Namespace!",
				},
			},
			expectedErrors:  1,
			checkErrorPaths: []string{"metadata.namespace"},
		},
		{
			name: "empty name and invalid namespace",
			addressGroup: &v1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "",
					Namespace: "Invalid_Namespace!",
				},
			},
			expectedErrors:  2,
			checkErrorPaths: []string{"metadata.name", "metadata.namespace"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validator.validateMetadata(tc.addressGroup)
			assert.Len(t, errs, tc.expectedErrors,
				"Expected %d errors, got %d: %v", tc.expectedErrors, len(errs), errs)

			// Check specific error paths if specified
			for _, expectedPath := range tc.checkErrorPaths {
				found := false
				for _, err := range errs {
					if err.Field == expectedPath {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected error path %s not found in errors: %v", expectedPath, errs)
			}
		})
	}
}

func TestAddressGroupValidator_validateSpec(t *testing.T) {
	validator := NewAddressGroupValidator()

	testCases := []struct {
		name            string
		spec            v1beta1.AddressGroupSpec
		expectedErrors  int
		checkErrorPaths []string
	}{
		{
			name: "valid spec with ACCEPT",
			spec: v1beta1.AddressGroupSpec{
				DefaultAction: v1beta1.ActionAccept,
				Logs:          false,
				Trace:         false,
			},
			expectedErrors: 0,
		},
		{
			name: "valid spec with DROP and flags",
			spec: v1beta1.AddressGroupSpec{
				DefaultAction: v1beta1.ActionDrop,
				Logs:          true,
				Trace:         true,
			},
			expectedErrors: 0,
		},
		{
			name: "missing default action",
			spec: v1beta1.AddressGroupSpec{
				// DefaultAction missing
				Logs:  false,
				Trace: false,
			},
			expectedErrors:  1,
			checkErrorPaths: []string{"spec.defaultAction"},
		},
		{
			name: "invalid default action",
			spec: v1beta1.AddressGroupSpec{
				DefaultAction: "INVALID",
				Logs:          false,
				Trace:         false,
			},
			expectedErrors:  1,
			checkErrorPaths: []string{"spec.defaultAction"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validator.validateSpec(tc.spec, field.NewPath("spec"))
			assert.Len(t, errs, tc.expectedErrors,
				"Expected %d errors, got %d: %v", tc.expectedErrors, len(errs), errs)

			// Check specific error paths if specified
			for _, expectedPath := range tc.checkErrorPaths {
				found := false
				for _, err := range errs {
					if err.Field == expectedPath {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected error path %s not found in errors: %v", expectedPath, errs)
			}
		})
	}
}

func TestAddressGroupValidator_validateDefaultAction(t *testing.T) {
	validator := NewAddressGroupValidator()

	testCases := []struct {
		name           string
		action         v1beta1.RuleAction
		expectedErrors int
		expectedType   field.ErrorType
	}{
		{
			name:           "valid ACCEPT action",
			action:         v1beta1.ActionAccept,
			expectedErrors: 0,
		},
		{
			name:           "valid DROP action",
			action:         v1beta1.ActionDrop,
			expectedErrors: 0,
		},
		{
			name:           "empty action",
			action:         "",
			expectedErrors: 1,
			expectedType:   field.ErrorTypeRequired,
		},
		{
			name:           "invalid action",
			action:         "INVALID",
			expectedErrors: 1,
			expectedType:   field.ErrorTypeNotSupported,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validator.validateDefaultAction(tc.action, field.NewPath("defaultAction"))
			assert.Len(t, errs, tc.expectedErrors,
				"Expected %d errors, got %d: %v", tc.expectedErrors, len(errs), errs)

			if tc.expectedErrors > 0 {
				assert.Equal(t, tc.expectedType, errs[0].Type,
					"Expected error type %s, got %s", tc.expectedType, errs[0].Type)
			}
		})
	}
}
