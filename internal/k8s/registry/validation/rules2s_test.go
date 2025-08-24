package validation

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestRuleS2SValidator_ValidateCreate(t *testing.T) {
	validator := NewRuleS2SValidator()
	ctx := context.Background()

	tests := []struct {
		name        string
		rule        *v1beta1.RuleS2S
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid RuleS2S with INGRESS traffic",
			rule: &v1beta1.RuleS2S{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-rule",
					Namespace: "default",
				},
				Spec: v1beta1.RuleS2SSpec{
					Traffic: v1beta1.INGRESS,
					ServiceLocalRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "local-service",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "remote-service",
						},
						Namespace: "remote",
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid RuleS2S with EGRESS traffic",
			rule: &v1beta1.RuleS2S{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "egress-rule",
					Namespace: "default",
				},
				Spec: v1beta1.RuleS2SSpec{
					Traffic: v1beta1.EGRESS,
					ServiceLocalRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "local-service",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "remote-service",
						},
						Namespace: "remote",
					},
				},
			},
			expectError: false,
		},
		{
			name:        "nil RuleS2S object",
			rule:        nil,
			expectError: true,
			errorMsg:    "rules2s object cannot be nil",
		},
		{
			name: "missing name in metadata",
			rule: &v1beta1.RuleS2S{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: v1beta1.RuleS2SSpec{
					Traffic: v1beta1.INGRESS,
				},
			},
			expectError: true,
			errorMsg:    "name or generateName is required",
		},
		{
			name: "invalid name format",
			rule: &v1beta1.RuleS2S{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Name_With_Underscores",
					Namespace: "default",
				},
				Spec: v1beta1.RuleS2SSpec{
					Traffic: v1beta1.INGRESS,
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "missing traffic",
			rule: &v1beta1.RuleS2S{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-traffic",
					Namespace: "default",
				},
				Spec: v1beta1.RuleS2SSpec{
					ServiceLocalRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "local-service",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "remote-service",
						},
						Namespace: "remote",
					},
				},
			},
			expectError: true,
			errorMsg:    "traffic is required",
		},
		{
			name: "invalid traffic value",
			rule: &v1beta1.RuleS2S{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-traffic",
					Namespace: "default",
				},
				Spec: v1beta1.RuleS2SSpec{
					Traffic: v1beta1.Traffic("INVALID"),
					ServiceLocalRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "local-service",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "remote-service",
						},
						Namespace: "remote",
					},
				},
			},
			expectError: true,
			errorMsg:    "supported values: \"INGRESS\", \"EGRESS\"",
		},
		{
			name: "missing serviceLocalRef fields",
			rule: &v1beta1.RuleS2S{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-local-ref",
					Namespace: "default",
				},
				Spec: v1beta1.RuleS2SSpec{
					Traffic:         v1beta1.INGRESS,
					ServiceLocalRef: v1beta1.NamespacedObjectReference{
						// Missing required fields
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "remote-service",
						},
						Namespace: "remote",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "wrong kind in serviceLocalRef",
			rule: &v1beta1.RuleS2S{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-kind-local",
					Namespace: "default",
				},
				Spec: v1beta1.RuleS2SSpec{
					Traffic: v1beta1.INGRESS,
					ServiceLocalRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service", // Should be ServiceAlias
							Name:       "local-service",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "remote-service",
						},
						Namespace: "remote",
					},
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'ServiceAlias'",
		},
		{
			name: "wrong kind in serviceRef",
			rule: &v1beta1.RuleS2S{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-kind-remote",
					Namespace: "default",
				},
				Spec: v1beta1.RuleS2SSpec{
					Traffic: v1beta1.INGRESS,
					ServiceLocalRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "local-service",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup", // Should be ServiceAlias
							Name:       "remote-service",
						},
						Namespace: "remote",
					},
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'ServiceAlias'",
		},
		{
			name: "missing namespace in serviceLocalRef",
			rule: &v1beta1.RuleS2S{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-local-ns",
					Namespace: "default",
				},
				Spec: v1beta1.RuleS2SSpec{
					Traffic: v1beta1.INGRESS,
					ServiceLocalRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "local-service",
						},
						// Missing namespace
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "remote-service",
						},
						Namespace: "remote",
					},
				},
			},
			expectError: true,
			errorMsg:    "namespace is required",
		},
		{
			name: "invalid name format in serviceRef",
			rule: &v1beta1.RuleS2S{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-ref-name",
					Namespace: "default",
				},
				Spec: v1beta1.RuleS2SSpec{
					Traffic: v1beta1.INGRESS,
					ServiceLocalRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "local-service",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "Invalid_Service_Name",
						},
						Namespace: "remote",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateCreate(ctx, tt.rule)

			if tt.expectError {
				if len(errors) == 0 {
					t.Errorf("expected validation error but got none")
					return
				}

				// Check if the expected error message is found
				found := false
				for _, err := range errors {
					if err.Detail != "" && containsString(err.Detail, tt.errorMsg) {
						found = true
						break
					}
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

func TestRuleS2SValidator_ValidateUpdate(t *testing.T) {
	validator := NewRuleS2SValidator()
	ctx := context.Background()

	baseRule := &v1beta1.RuleS2S{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rule",
			Namespace: "default",
		},
		Spec: v1beta1.RuleS2SSpec{
			Traffic: v1beta1.INGRESS,
			ServiceLocalRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "ServiceAlias",
					Name:       "local-service",
				},
				Namespace: "default",
			},
			ServiceRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "ServiceAlias",
					Name:       "remote-service",
				},
				Namespace: "remote",
			},
		},
	}

	tests := []struct {
		name        string
		newRule     *v1beta1.RuleS2S
		oldRule     *v1beta1.RuleS2S
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid update (no immutable fields changed)",
			newRule:     baseRule.DeepCopy(),
			oldRule:     baseRule,
			expectError: false,
		},
		{
			name: "attempt to change traffic (immutable)",
			newRule: func() *v1beta1.RuleS2S {
				rule := baseRule.DeepCopy()
				rule.Spec.Traffic = v1beta1.EGRESS
				return rule
			}(),
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "traffic is immutable and cannot be changed",
		},
		{
			name: "attempt to change serviceLocalRef (immutable)",
			newRule: func() *v1beta1.RuleS2S {
				rule := baseRule.DeepCopy()
				rule.Spec.ServiceLocalRef.Name = "different-local-service"
				return rule
			}(),
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "serviceLocalRef is immutable and cannot be changed",
		},
		{
			name: "attempt to change serviceRef (immutable)",
			newRule: func() *v1beta1.RuleS2S {
				rule := baseRule.DeepCopy()
				rule.Spec.ServiceRef.Name = "different-remote-service"
				return rule
			}(),
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "serviceRef is immutable and cannot be changed",
		},
		{
			name: "update with invalid new data",
			newRule: &v1beta1.RuleS2S{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Name",
					Namespace: "default",
				},
				Spec: v1beta1.RuleS2SSpec{
					Traffic: v1beta1.INGRESS,
					ServiceLocalRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "local-service",
						},
						Namespace: "default",
					},
					ServiceRef: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "ServiceAlias",
							Name:       "remote-service",
						},
						Namespace: "remote",
					},
				},
			},
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateUpdate(ctx, tt.newRule, tt.oldRule)

			if tt.expectError {
				if len(errors) == 0 {
					t.Errorf("expected validation error but got none")
					return
				}

				// Check if the expected error message is found
				found := false
				for _, err := range errors {
					if err.Detail != "" && containsString(err.Detail, tt.errorMsg) {
						found = true
						break
					}
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

func TestRuleS2SValidator_ValidateDelete(t *testing.T) {
	validator := NewRuleS2SValidator()
	ctx := context.Background()

	rule := &v1beta1.RuleS2S{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rule",
			Namespace: "default",
		},
		Spec: v1beta1.RuleS2SSpec{
			Traffic: v1beta1.INGRESS,
		},
	}

	errors := validator.ValidateDelete(ctx, rule)
	if len(errors) > 0 {
		t.Errorf("expected no validation errors for delete but got: %v", errors)
	}
}
