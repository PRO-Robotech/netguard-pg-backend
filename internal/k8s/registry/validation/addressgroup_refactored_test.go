package validation

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestAddressGroupValidatorRefactored(t *testing.T) {
	validator := NewAddressGroupValidator()

	tests := []struct {
		name         string
		addressGroup *netguardv1beta1.AddressGroup
		wantErrs     int
	}{
		{
			name: "valid address group with standard validation",
			addressGroup: &netguardv1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: netguardv1beta1.AddressGroupSpec{
					DefaultAction: netguardv1beta1.ActionAccept,
					Logs:          true,
					Trace:         false,
				},
			},
			wantErrs: 0,
		},
		{
			name: "valid address group with DROP action",
			addressGroup: &netguardv1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag-drop",
					Namespace: "default",
				},
				Spec: netguardv1beta1.AddressGroupSpec{
					DefaultAction: netguardv1beta1.ActionDrop,
					Logs:          false,
					Trace:         true,
				},
			},
			wantErrs: 0,
		},
		{
			name: "invalid address group with bad metadata",
			addressGroup: &netguardv1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					// Missing name and generateName
					Namespace: "default",
				},
				Spec: netguardv1beta1.AddressGroupSpec{
					DefaultAction: netguardv1beta1.ActionAccept,
				},
			},
			wantErrs: 1, // Should fail standard metadata validation
		},
		{
			name: "invalid address group with missing default action",
			addressGroup: &netguardv1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: netguardv1beta1.AddressGroupSpec{
					// DefaultAction is empty (required field)
					Logs:  true,
					Trace: false,
				},
			},
			wantErrs: 1, // Should fail required validation
		},
		{
			name: "invalid address group with invalid default action",
			addressGroup: &netguardv1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: netguardv1beta1.AddressGroupSpec{
					DefaultAction: "INVALID", // Invalid action
					Logs:          true,
					Trace:         false,
				},
			},
			wantErrs: 1, // Should fail standard RuleAction validation
		},
		{
			name: "invalid address group with bad name format",
			addressGroup: &netguardv1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "INVALID_NAME", // Invalid DNS format
					Namespace: "default",
				},
				Spec: netguardv1beta1.AddressGroupSpec{
					DefaultAction: netguardv1beta1.ActionAccept,
				},
			},
			wantErrs: 1, // Should fail standard metadata validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			errs := validator.ValidateCreate(ctx, tt.addressGroup)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateCreate() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestAddressGroupValidatorUpdate(t *testing.T) {
	validator := NewAddressGroupValidator()
	ctx := context.Background()

	oldAG := &netguardv1beta1.AddressGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ag",
			Namespace: "default",
		},
		Spec: netguardv1beta1.AddressGroupSpec{
			DefaultAction: netguardv1beta1.ActionAccept,
			Logs:          false,
			Trace:         false,
		},
	}

	newAG := &netguardv1beta1.AddressGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ag",
			Namespace: "default",
		},
		Spec: netguardv1beta1.AddressGroupSpec{
			DefaultAction: netguardv1beta1.ActionDrop, // Changed action
			Logs:          true,                       // Changed logs
			Trace:         true,                       // Changed trace
		},
	}

	errs := validator.ValidateUpdate(ctx, newAG, oldAG)
	if len(errs) != 0 {
		t.Errorf("ValidateUpdate() got %d errors, want 0 errors: %v", len(errs), errs)
	}
}

func TestAddressGroupValidatorDelete(t *testing.T) {
	validator := NewAddressGroupValidator()
	ctx := context.Background()

	ag := &netguardv1beta1.AddressGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ag",
			Namespace: "default",
		},
		Spec: netguardv1beta1.AddressGroupSpec{
			DefaultAction: netguardv1beta1.ActionAccept,
		},
	}

	errs := validator.ValidateDelete(ctx, ag)
	if len(errs) != 0 {
		t.Errorf("ValidateDelete() got %d errors, want 0 errors: %v", len(errs), errs)
	}
}
