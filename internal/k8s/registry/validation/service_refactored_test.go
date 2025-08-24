package validation

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestServiceValidatorRefactored(t *testing.T) {
	validator := NewServiceValidator()

	tests := []struct {
		name     string
		service  *netguardv1beta1.Service
		wantErrs int
	}{
		{
			name: "valid service with standard validation",
			service: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Test service",
					IngressPorts: []netguardv1beta1.IngressPort{
						{
							Protocol:    netguardv1beta1.ProtocolTCP,
							Port:        "80",
							Description: "HTTP port",
						},
					},
				},
			},
			wantErrs: 0,
		},
		{
			name: "invalid service with bad metadata",
			service: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					// Missing name and generateName
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Test service",
				},
			},
			wantErrs: 1, // Should fail standard metadata validation
		},
		{
			name: "invalid service with bad ingress ports",
			service: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Test service",
					IngressPorts: []netguardv1beta1.IngressPort{
						{
							Protocol: "INVALID", // Invalid protocol
							Port:     "80",
						},
					},
				},
			},
			wantErrs: 1, // Should fail standard ingress port validation
		},
		{
			name: "service with duplicate ports",
			service: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					IngressPorts: []netguardv1beta1.IngressPort{
						{
							Protocol: netguardv1beta1.ProtocolTCP,
							Port:     "80",
						},
						{
							Protocol: netguardv1beta1.ProtocolTCP,
							Port:     "80", // Duplicate
						},
					},
				},
			},
			wantErrs: 1, // Should fail duplicate validation
		},
		{
			name: "service with long description",
			service: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: string(make([]byte, 600)), // Longer than 512 chars
				},
			},
			wantErrs: 1, // Should fail description length validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			errs := validator.ValidateCreate(ctx, tt.service)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateCreate() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestServiceValidatorUpdate(t *testing.T) {
	validator := NewServiceValidator()
	ctx := context.Background()

	oldService := &netguardv1beta1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: netguardv1beta1.ServiceSpec{
			Description: "Original service",
		},
	}

	newService := &netguardv1beta1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: netguardv1beta1.ServiceSpec{
			Description: "Updated service",
			IngressPorts: []netguardv1beta1.IngressPort{
				{
					Protocol: netguardv1beta1.ProtocolTCP,
					Port:     "8080",
				},
			},
		},
	}

	errs := validator.ValidateUpdate(ctx, newService, oldService)
	if len(errs) != 0 {
		t.Errorf("ValidateUpdate() got %d errors, want 0 errors: %v", len(errs), errs)
	}
}

func TestServiceValidatorDelete(t *testing.T) {
	validator := NewServiceValidator()
	ctx := context.Background()

	service := &netguardv1beta1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
	}

	errs := validator.ValidateDelete(ctx, service)
	if len(errs) != 0 {
		t.Errorf("ValidateDelete() got %d errors, want 0 errors: %v", len(errs), errs)
	}
}
