package validation

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestNetworkValidator_ValidateCreate(t *testing.T) {
	validator := NewNetworkValidator()
	ctx := context.Background()

	tests := []struct {
		name        string
		network     *v1beta1.Network
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid Network with IPv4 CIDR",
			network: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-network",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "192.168.1.0/24",
				},
			},
			expectError: false,
		},
		{
			name: "valid Network with IPv6 CIDR",
			network: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ipv6-network",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "2001:db8::/32",
				},
			},
			expectError: false,
		},
		{
			name: "valid Network with single host CIDR",
			network: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "single-host",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "10.0.0.1/32",
				},
			},
			expectError: false,
		},
		{
			name:        "nil Network object",
			network:     nil,
			expectError: true,
			errorMsg:    "network object cannot be nil",
		},
		{
			name: "missing name in metadata",
			network: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "10.0.0.0/8",
				},
			},
			expectError: true,
			errorMsg:    "name or generateName is required",
		},
		{
			name: "invalid name format",
			network: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Network_Name",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "10.0.0.0/8",
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "missing CIDR",
			network: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-cidr",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkSpec{
					// Missing CIDR
				},
			},
			expectError: true,
			errorMsg:    "CIDR cannot be empty",
		},
		{
			name: "invalid CIDR format",
			network: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-cidr",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "invalid-cidr-format",
				},
			},
			expectError: true,
			errorMsg:    "must be a valid CIDR notation",
		},
		{
			name: "invalid IP address in CIDR",
			network: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-ip",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "300.300.300.300/24",
				},
			},
			expectError: true,
			errorMsg:    "must be a valid CIDR notation",
		},
		{
			name: "invalid subnet mask",
			network: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-mask",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "192.168.1.0/99",
				},
			},
			expectError: true,
			errorMsg:    "must be a valid CIDR notation",
		},
		{
			name: "IP without CIDR notation",
			network: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-cidr-notation",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "192.168.1.1", // Missing /24
				},
			},
			expectError: true,
			errorMsg:    "must be a valid CIDR notation",
		},
		{
			name: "valid generateName instead of name",
			network: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "auto-network",
					Namespace:    "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "172.16.0.0/16",
				},
			},
			expectError: false,
		},
		{
			name: "invalid generateName format",
			network: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "Invalid_Generate_Name",
					Namespace:    "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "172.16.0.0/16",
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateCreate(ctx, tt.network)

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

func TestNetworkValidator_ValidateUpdate(t *testing.T) {
	validator := NewNetworkValidator()
	ctx := context.Background()

	baseNetwork := &v1beta1.Network{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-network",
			Namespace: "default",
		},
		Spec: v1beta1.NetworkSpec{
			CIDR: "10.0.0.0/8",
		},
		Status: v1beta1.NetworkStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: "False", // Not ready yet
				},
			},
		},
	}

	baseNetworkReady := &v1beta1.Network{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ready-network",
			Namespace: "default",
		},
		Spec: v1beta1.NetworkSpec{
			CIDR: "10.0.0.0/8",
		},
		Status: v1beta1.NetworkStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: "True", // Ready
				},
			},
		},
	}

	tests := []struct {
		name        string
		newNetwork  *v1beta1.Network
		oldNetwork  *v1beta1.Network
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid update (no CIDR change, not ready)",
			newNetwork:  baseNetwork.DeepCopy(),
			oldNetwork:  baseNetwork,
			expectError: false,
		},
		{
			name: "valid CIDR change when not ready",
			newNetwork: func() *v1beta1.Network {
				network := baseNetwork.DeepCopy()
				network.Spec.CIDR = "172.16.0.0/16"
				return network
			}(),
			oldNetwork:  baseNetwork,
			expectError: false,
		},
		{
			name:        "valid update when ready (no CIDR change)",
			newNetwork:  baseNetworkReady.DeepCopy(),
			oldNetwork:  baseNetworkReady,
			expectError: false,
		},
		{
			name: "attempt to change CIDR when ready (immutable)",
			newNetwork: func() *v1beta1.Network {
				network := baseNetworkReady.DeepCopy()
				network.Spec.CIDR = "192.168.0.0/16"
				return network
			}(),
			oldNetwork:  baseNetworkReady,
			expectError: true,
			errorMsg:    "cannot change CIDR when Ready condition is true",
		},
		{
			name: "update with invalid new CIDR",
			newNetwork: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-network",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "invalid-cidr",
				},
			},
			oldNetwork:  baseNetwork,
			expectError: true,
			errorMsg:    "must be a valid CIDR notation",
		},
		{
			name: "update with invalid metadata",
			newNetwork: &v1beta1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Name",
					Namespace: "default",
				},
				Spec: v1beta1.NetworkSpec{
					CIDR: "10.0.0.0/8",
				},
			},
			oldNetwork:  baseNetwork,
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateUpdate(ctx, tt.newNetwork, tt.oldNetwork)

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

func TestNetworkValidator_ValidateDelete(t *testing.T) {
	validator := NewNetworkValidator()
	ctx := context.Background()

	network := &v1beta1.Network{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-network",
			Namespace: "default",
		},
		Spec: v1beta1.NetworkSpec{
			CIDR: "10.0.0.0/8",
		},
	}

	errors := validator.ValidateDelete(ctx, network)
	if len(errors) > 0 {
		t.Errorf("expected no validation errors for delete but got: %v", errors)
	}
}
