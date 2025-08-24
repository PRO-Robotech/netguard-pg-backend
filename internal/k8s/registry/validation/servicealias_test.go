package validation

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestServiceAliasValidator_ValidateCreate(t *testing.T) {
	validator := NewServiceAliasValidator()
	ctx := context.Background()

	tests := []struct {
		name         string
		serviceAlias *v1beta1.ServiceAlias
		expectError  bool
		errorMsg     string
	}{
		{
			name: "valid ServiceAlias",
			serviceAlias: &v1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-alias",
					Namespace: "default",
				},
				Spec: v1beta1.ServiceAliasSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
				},
			},
			expectError: false,
		},
		{
			name:         "nil ServiceAlias object",
			serviceAlias: nil,
			expectError:  true,
			errorMsg:     "servicealias object cannot be nil",
		},
		{
			name: "missing name in metadata",
			serviceAlias: &v1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: v1beta1.ServiceAliasSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
				},
			},
			expectError: true,
			errorMsg:    "name or generateName is required",
		},
		{
			name: "invalid name format",
			serviceAlias: &v1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_ServiceAlias_Name",
					Namespace: "default",
				},
				Spec: v1beta1.ServiceAliasSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "missing serviceRef fields",
			serviceAlias: &v1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-service-ref",
					Namespace: "default",
				},
				Spec: v1beta1.ServiceAliasSpec{
					ServiceRef: v1beta1.ObjectReference{
						// Missing required fields
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "wrong API version in serviceRef",
			serviceAlias: &v1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-api-version",
					Namespace: "default",
				},
				Spec: v1beta1.ServiceAliasSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "v1", // Wrong API version
						Kind:       "Service",
						Name:       "my-service",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion must be 'netguard.sgroups.io/v1beta1'",
		},
		{
			name: "wrong kind in serviceRef",
			serviceAlias: &v1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-kind",
					Namespace: "default",
				},
				Spec: v1beta1.ServiceAliasSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup", // Should be Service
						Name:       "my-service",
					},
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'Service'",
		},
		{
			name: "missing name in serviceRef",
			serviceAlias: &v1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-service-name",
					Namespace: "default",
				},
				Spec: v1beta1.ServiceAliasSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						// Missing name
					},
				},
			},
			expectError: true,
			errorMsg:    "name is required",
		},
		{
			name: "invalid name format in serviceRef",
			serviceAlias: &v1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-service-name",
					Namespace: "default",
				},
				Spec: v1beta1.ServiceAliasSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "Invalid_Service_Name",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "valid generateName instead of name",
			serviceAlias: &v1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "auto-alias",
					Namespace:    "default",
				},
				Spec: v1beta1.ServiceAliasSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid generateName format",
			serviceAlias: &v1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "Invalid_GenerateName",
					Namespace:    "default",
				},
				Spec: v1beta1.ServiceAliasSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "valid labels and annotations",
			serviceAlias: &v1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labeled-alias",
					Namespace: "default",
					Labels: map[string]string{
						"app":                      "web",
						"version":                  "v1.0.0",
						"kubernetes.io/managed-by": "netguard",
					},
					Annotations: map[string]string{
						"description": "Service alias for web application",
						"owner":       "platform-team",
					},
				},
				Spec: v1beta1.ServiceAliasSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "web-service",
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateCreate(ctx, tt.serviceAlias)

			if tt.expectError {
				if len(errors) == 0 {
					t.Errorf("expected validation error but got none")
					return
				}

				// Check if the expected error message is found
				found := false
				fullErrorText := errors.ToAggregate().Error()
				if containsString(fullErrorText, tt.errorMsg) {
					found = true
				}
				if !found {
					t.Errorf("expected error message containing '%s', but got: %v", tt.errorMsg, errors)
				}
			} else {
				if len(errors) > 0 {
					t.Errorf("expected no validation errors but got: %v", errors)
				}
			}
		})
	}
}

func TestServiceAliasValidator_ValidateUpdate(t *testing.T) {
	validator := NewServiceAliasValidator()
	ctx := context.Background()

	baseServiceAlias := &v1beta1.ServiceAlias{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-alias",
			Namespace: "default",
		},
		Spec: v1beta1.ServiceAliasSpec{
			ServiceRef: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "my-service",
			},
		},
	}

	tests := []struct {
		name            string
		newServiceAlias *v1beta1.ServiceAlias
		oldServiceAlias *v1beta1.ServiceAlias
		expectError     bool
		errorMsg        string
	}{
		{
			name:            "valid update (no serviceRef change)",
			newServiceAlias: baseServiceAlias.DeepCopy(),
			oldServiceAlias: baseServiceAlias,
			expectError:     false,
		},
		{
			name: "attempt to change serviceRef (immutable)",
			newServiceAlias: func() *v1beta1.ServiceAlias {
				alias := baseServiceAlias.DeepCopy()
				alias.Spec.ServiceRef.Name = "different-service"
				return alias
			}(),
			oldServiceAlias: baseServiceAlias,
			expectError:     true,
			errorMsg:        "serviceRef is immutable and cannot be changed",
		},
		{
			name: "attempt to change serviceRef kind (immutable)",
			newServiceAlias: func() *v1beta1.ServiceAlias {
				alias := baseServiceAlias.DeepCopy()
				alias.Spec.ServiceRef.Kind = "AddressGroup"
				return alias
			}(),
			oldServiceAlias: baseServiceAlias,
			expectError:     true,
			errorMsg:        "serviceRef is immutable and cannot be changed",
		},
		{
			name: "attempt to change serviceRef API version (immutable)",
			newServiceAlias: func() *v1beta1.ServiceAlias {
				alias := baseServiceAlias.DeepCopy()
				alias.Spec.ServiceRef.APIVersion = "v1"
				return alias
			}(),
			oldServiceAlias: baseServiceAlias,
			expectError:     true,
			errorMsg:        "serviceRef is immutable and cannot be changed",
		},
		{
			name: "valid metadata update (labels, annotations)",
			newServiceAlias: func() *v1beta1.ServiceAlias {
				alias := baseServiceAlias.DeepCopy()
				alias.Labels = map[string]string{"new-label": "new-value"}
				alias.Annotations = map[string]string{"new-annotation": "new-value"}
				return alias
			}(),
			oldServiceAlias: baseServiceAlias,
			expectError:     false,
		},
		{
			name: "update with invalid new data",
			newServiceAlias: &v1beta1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Name",
					Namespace: "default",
				},
				Spec: v1beta1.ServiceAliasSpec{
					ServiceRef: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service",
						Name:       "my-service",
					},
				},
			},
			oldServiceAlias: baseServiceAlias,
			expectError:     true,
			errorMsg:        "RFC 1123 subdomain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateUpdate(ctx, tt.newServiceAlias, tt.oldServiceAlias)

			if tt.expectError {
				if len(errors) == 0 {
					t.Errorf("expected validation error but got none")
					return
				}

				// Check if the expected error message is found
				found := false
				fullErrorText := errors.ToAggregate().Error()
				if containsString(fullErrorText, tt.errorMsg) {
					found = true
				}
				if !found {
					t.Errorf("expected error message containing '%s', but got: %v", tt.errorMsg, errors)
				}
			} else {
				if len(errors) > 0 {
					t.Errorf("expected no validation errors but got: %v", errors)
				}
			}
		})
	}
}

func TestServiceAliasValidator_ValidateDelete(t *testing.T) {
	validator := NewServiceAliasValidator()
	ctx := context.Background()

	serviceAlias := &v1beta1.ServiceAlias{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-alias",
			Namespace: "default",
		},
		Spec: v1beta1.ServiceAliasSpec{
			ServiceRef: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "my-service",
			},
		},
	}

	errors := validator.ValidateDelete(ctx, serviceAlias)
	if len(errors) > 0 {
		t.Errorf("expected no validation errors for delete but got: %v", errors)
	}
}
