package validation

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestSvcSvcRuleValidator_ValidateCreate(t *testing.T) {
	validator := NewSvcSvcRuleValidator()
	ctx := context.Background()

	tests := []struct {
		name        string
		rule        *v1beta1.SvcSvcRule
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid SvcSvcRule with ACCEPT action",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-rule",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-to",
						},
						Namespace: "default",
					},
					Action:   v1beta1.ActionAccept,
					Priority: 100,
				},
			},
			expectError: false,
		},
		{
			name: "valid SvcSvcRule with DROP action",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "drop-rule",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-to",
						},
						Namespace: "remote",
					},
					Action:   v1beta1.ActionDrop,
					Priority: 500,
				},
			},
			expectError: false,
		},
		{
			name: "valid SvcSvcRule with priority 0",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "priority-zero",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-to",
						},
						Namespace: "default",
					},
					Action:   v1beta1.ActionAccept,
					Priority: 0,
				},
			},
			expectError: false,
		},
		{
			name: "valid SvcSvcRule with priority 1000",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "priority-max",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-to",
						},
						Namespace: "default",
					},
					Action:   v1beta1.ActionAccept,
					Priority: 1000,
				},
			},
			expectError: false,
		},
		{
			name:        "nil SvcSvcRule object",
			rule:        nil,
			expectError: true,
			errorMsg:    "object cannot be nil",
		},
		{
			name: "missing name in metadata",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					Action: v1beta1.ActionAccept,
				},
			},
			expectError: true,
			errorMsg:    "name or generateName is required",
		},
		{
			name: "invalid name format",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Name_With_Underscores",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					Action: v1beta1.ActionAccept,
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "missing serviceFrom fields",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-service-from",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						// Missing required fields
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-to",
						},
						Namespace: "default",
					},
					Action:   v1beta1.ActionAccept,
					Priority: 100,
				},
			},
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "wrong kind in serviceFrom",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-kind-from",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup", // Should be Service
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-to",
						},
						Namespace: "default",
					},
					Action:   v1beta1.ActionAccept,
					Priority: 100,
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'Service'",
		},
		{
			name: "wrong kind in serviceTo",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-kind-to",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Host", // Should be Service
							Name:       "service-to",
						},
						Namespace: "default",
					},
					Action:   v1beta1.ActionAccept,
					Priority: 100,
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'Service'",
		},
		{
			name: "missing namespace in serviceFrom",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-from-ns",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						// Missing namespace
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-to",
						},
						Namespace: "default",
					},
					Action:   v1beta1.ActionAccept,
					Priority: 100,
				},
			},
			expectError: true,
			errorMsg:    "namespace is required",
		},
		{
			name: "missing serviceTo fields",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-service-to",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						// Missing required fields
					},
					Action:   v1beta1.ActionAccept,
					Priority: 100,
				},
			},
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "invalid name format in serviceTo",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-to-name",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "Invalid_Service_Name",
						},
						Namespace: "default",
					},
					Action:   v1beta1.ActionAccept,
					Priority: 100,
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "missing action",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-action",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-to",
						},
						Namespace: "default",
					},
					Priority: 100,
				},
			},
			expectError: true,
			errorMsg:    "action is required",
		},
		{
			name: "invalid action value",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-action",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-to",
						},
						Namespace: "default",
					},
					Action:   v1beta1.RuleAction("INVALID"),
					Priority: 100,
				},
			},
			expectError: true,
			errorMsg:    "supported values: \"ACCEPT\", \"DROP\"",
		},
		{
			name: "negative priority",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "negative-priority",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-to",
						},
						Namespace: "default",
					},
					Action:   v1beta1.ActionAccept,
					Priority: -1,
				},
			},
			expectError: true,
			errorMsg:    "priority must be between 0 and 1000",
		},
		{
			name: "priority too high",
			rule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "priority-too-high",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-to",
						},
						Namespace: "default",
					},
					Action:   v1beta1.ActionAccept,
					Priority: 1001,
				},
			},
			expectError: true,
			errorMsg:    "priority must be between 0 and 1000",
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

func TestSvcSvcRuleValidator_ValidateUpdate(t *testing.T) {
	validator := NewSvcSvcRuleValidator()
	ctx := context.Background()

	baseRule := &v1beta1.SvcSvcRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rule",
			Namespace: "default",
		},
		Spec: v1beta1.SvcSvcRuleSpec{
			ServiceFrom: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       "service-from",
				},
				Namespace: "default",
			},
			ServiceTo: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
					Name:       "service-to",
				},
				Namespace: "remote",
			},
			Action:   v1beta1.ActionAccept,
			Priority: 100,
		},
	}

	tests := []struct {
		name        string
		newRule     *v1beta1.SvcSvcRule
		oldRule     *v1beta1.SvcSvcRule
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
			name: "valid update (change action)",
			newRule: func() *v1beta1.SvcSvcRule {
				rule := baseRule.DeepCopy()
				rule.Spec.Action = v1beta1.ActionDrop
				return rule
			}(),
			oldRule:     baseRule,
			expectError: false,
		},
		{
			name: "valid update (change priority)",
			newRule: func() *v1beta1.SvcSvcRule {
				rule := baseRule.DeepCopy()
				rule.Spec.Priority = 500
				return rule
			}(),
			oldRule:     baseRule,
			expectError: false,
		},
		{
			name: "attempt to change serviceFrom (immutable)",
			newRule: func() *v1beta1.SvcSvcRule {
				rule := baseRule.DeepCopy()
				rule.Spec.ServiceFrom.Name = "different-service-from"
				return rule
			}(),
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "serviceFrom is immutable and cannot be changed",
		},
		{
			name: "attempt to change serviceTo (immutable)",
			newRule: func() *v1beta1.SvcSvcRule {
				rule := baseRule.DeepCopy()
				rule.Spec.ServiceTo.Name = "different-service-to"
				return rule
			}(),
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "serviceTo is immutable and cannot be changed",
		},
		{
			name: "attempt to change serviceFrom namespace (immutable)",
			newRule: func() *v1beta1.SvcSvcRule {
				rule := baseRule.DeepCopy()
				rule.Spec.ServiceFrom.Namespace = "other-namespace"
				return rule
			}(),
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "serviceFrom is immutable and cannot be changed",
		},
		{
			name: "attempt to change serviceTo namespace (immutable)",
			newRule: func() *v1beta1.SvcSvcRule {
				rule := baseRule.DeepCopy()
				rule.Spec.ServiceTo.Namespace = "other-namespace"
				return rule
			}(),
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "serviceTo is immutable and cannot be changed",
		},
		{
			name: "update with invalid new data",
			newRule: &v1beta1.SvcSvcRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Name",
					Namespace: "default",
				},
				Spec: v1beta1.SvcSvcRuleSpec{
					ServiceFrom: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-from",
						},
						Namespace: "default",
					},
					ServiceTo: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "service-to",
						},
						Namespace: "remote",
					},
					Action:   v1beta1.ActionAccept,
					Priority: 100,
				},
			},
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "update with invalid priority",
			newRule: func() *v1beta1.SvcSvcRule {
				rule := baseRule.DeepCopy()
				rule.Spec.Priority = 2000
				return rule
			}(),
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "priority must be between 0 and 1000",
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

func TestSvcSvcRuleValidator_ValidateDelete(t *testing.T) {
	validator := NewSvcSvcRuleValidator()
	ctx := context.Background()

	rule := &v1beta1.SvcSvcRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rule",
			Namespace: "default",
		},
		Spec: v1beta1.SvcSvcRuleSpec{
			Action: v1beta1.ActionAccept,
		},
	}

	errors := validator.ValidateDelete(ctx, rule)
	if len(errors) > 0 {
		t.Errorf("expected no validation errors for delete but got: %v", errors)
	}
}
