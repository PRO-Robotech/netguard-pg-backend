package validation

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestValidateStandardObjectMeta(t *testing.T) {
	tests := []struct {
		name     string
		meta     *metav1.ObjectMeta
		wantErrs int
	}{
		{
			name:     "nil metadata",
			meta:     nil,
			wantErrs: 1,
		},
		{
			name: "valid metadata with name",
			meta: &metav1.ObjectMeta{
				Name:      "test-resource",
				Namespace: "test-ns",
			},
			wantErrs: 0,
		},
		{
			name: "valid metadata with generateName",
			meta: &metav1.ObjectMeta{
				GenerateName: "test-resource",
				Namespace:    "test-ns",
			},
			wantErrs: 0,
		},
		{
			name: "missing name and generateName",
			meta: &metav1.ObjectMeta{
				Namespace: "test-ns",
			},
			wantErrs: 1,
		},
		{
			name: "invalid name format",
			meta: &metav1.ObjectMeta{
				Name:      "INVALID_NAME",
				Namespace: "test-ns",
			},
			wantErrs: 1,
		},
		{
			name: "invalid namespace format",
			meta: &metav1.ObjectMeta{
				Name:      "test-resource",
				Namespace: "INVALID_NAMESPACE",
			},
			wantErrs: 1,
		},
		{
			name: "valid labels and annotations",
			meta: &metav1.ObjectMeta{
				Name:      "test-resource",
				Namespace: "test-ns",
				Labels: map[string]string{
					"app":     "test",
					"version": "v1",
				},
				Annotations: map[string]string{
					"description": "test resource",
					"version":     "1.0",
				},
			},
			wantErrs: 0,
		},
		{
			name: "invalid label key with space",
			meta: &metav1.ObjectMeta{
				Name:      "test-resource",
				Namespace: "test-ns",
				Labels: map[string]string{
					"invalid key with space": "value",
				},
			},
			wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fldPath := field.NewPath("metadata")
			errs := ValidateStandardObjectMeta(tt.meta, fldPath)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateStandardObjectMeta() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestValidateEnum(t *testing.T) {
	validValues := []string{"TCP", "UDP"}
	fldPath := field.NewPath("protocol")

	tests := []struct {
		name     string
		value    string
		wantErrs int
	}{
		{
			name:     "valid enum value",
			value:    "TCP",
			wantErrs: 0,
		},
		{
			name:     "another valid enum value",
			value:    "UDP",
			wantErrs: 0,
		},
		{
			name:     "empty value",
			value:    "",
			wantErrs: 0, // Empty values are handled by required field validation
		},
		{
			name:     "invalid enum value",
			value:    "HTTP",
			wantErrs: 1,
		},
		{
			name:     "case sensitive invalid",
			value:    "tcp",
			wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateEnum(tt.value, "protocol", validValues, fldPath)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateEnum() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	fldPath := field.NewPath("port")

	tests := []struct {
		name     string
		port     int32
		wantErrs int
	}{
		{
			name:     "valid port 80",
			port:     80,
			wantErrs: 0,
		},
		{
			name:     "valid port 1",
			port:     1,
			wantErrs: 0,
		},
		{
			name:     "valid port 65535",
			port:     65535,
			wantErrs: 0,
		},
		{
			name:     "invalid port 0",
			port:     0,
			wantErrs: 1,
		},
		{
			name:     "invalid port -1",
			port:     -1,
			wantErrs: 1,
		},
		{
			name:     "invalid port 65536",
			port:     65536,
			wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidatePort(tt.port, fldPath)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidatePort() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestValidatePortRange(t *testing.T) {
	fldPath := field.NewPath("portRange")

	tests := []struct {
		name      string
		portRange *netguardv1beta1.PortRange
		wantErrs  int
	}{
		{
			name:      "nil portRange",
			portRange: nil,
			wantErrs:  1,
		},
		{
			name: "valid port range",
			portRange: &netguardv1beta1.PortRange{
				From: 80,
				To:   90,
			},
			wantErrs: 0,
		},
		{
			name: "single port range",
			portRange: &netguardv1beta1.PortRange{
				From: 80,
				To:   80,
			},
			wantErrs: 0,
		},
		{
			name: "invalid from port",
			portRange: &netguardv1beta1.PortRange{
				From: 0,
				To:   80,
			},
			wantErrs: 1,
		},
		{
			name: "invalid to port",
			portRange: &netguardv1beta1.PortRange{
				From: 80,
				To:   65536,
			},
			wantErrs: 1,
		},
		{
			name: "from > to",
			portRange: &netguardv1beta1.PortRange{
				From: 90,
				To:   80,
			},
			wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidatePortRange(tt.portRange, fldPath)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidatePortRange() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestValidateCIDR(t *testing.T) {
	fldPath := field.NewPath("cidr")

	tests := []struct {
		name     string
		cidr     string
		wantErrs int
	}{
		{
			name:     "valid IPv4 CIDR",
			cidr:     "192.168.1.0/24",
			wantErrs: 0,
		},
		{
			name:     "valid IPv6 CIDR",
			cidr:     "2001:db8::/32",
			wantErrs: 0,
		},
		{
			name:     "valid single host IPv4",
			cidr:     "192.168.1.1/32",
			wantErrs: 0,
		},
		{
			name:     "empty CIDR",
			cidr:     "",
			wantErrs: 1,
		},
		{
			name:     "invalid CIDR format",
			cidr:     "192.168.1.0",
			wantErrs: 1,
		},
		{
			name:     "invalid IP address",
			cidr:     "256.256.256.256/24",
			wantErrs: 1,
		},
		{
			name:     "invalid subnet mask",
			cidr:     "192.168.1.0/33",
			wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateCIDR(tt.cidr, fldPath)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateCIDR() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestValidateObjectReference(t *testing.T) {
	fldPath := field.NewPath("ref")

	tests := []struct {
		name     string
		ref      *netguardv1beta1.ObjectReference
		wantErrs int
	}{
		{
			name:     "nil reference",
			ref:      nil,
			wantErrs: 1,
		},
		{
			name: "valid reference",
			ref: &netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "test-service",
			},
			wantErrs: 0,
		},
		{
			name: "missing apiVersion",
			ref: &netguardv1beta1.ObjectReference{
				Kind: "Service",
				Name: "test-service",
			},
			wantErrs: 1,
		},
		{
			name: "missing kind",
			ref: &netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Name:       "test-service",
			},
			wantErrs: 1,
		},
		{
			name: "missing name",
			ref: &netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
			},
			wantErrs: 1,
		},
		{
			name: "invalid name format",
			ref: &netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "INVALID_NAME",
			},
			wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateObjectReference(tt.ref, fldPath)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateObjectReference() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestValidateNamespacedObjectReference(t *testing.T) {
	fldPath := field.NewPath("ref")

	tests := []struct {
		name     string
		ref      *netguardv1beta1.NamespacedObjectReference
		wantErrs int
	}{
		{
			name:     "nil reference",
			ref:      nil,
			wantErrs: 1,
		},
		{
			name: "valid reference",
			ref: &netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "test-ag",
				},
				Namespace: "test-ns",
			},
			wantErrs: 0,
		},
		{
			name: "missing namespace",
			ref: &netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "test-ag",
				},
			},
			wantErrs: 1,
		},
		{
			name: "invalid namespace format",
			ref: &netguardv1beta1.NamespacedObjectReference{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "test-ag",
				},
				Namespace: "INVALID_NAMESPACE",
			},
			wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateNamespacedObjectReference(tt.ref, fldPath)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateNamespacedObjectReference() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestValidateTransportProtocol(t *testing.T) {
	fldPath := field.NewPath("protocol")

	tests := []struct {
		name     string
		protocol netguardv1beta1.TransportProtocol
		wantErrs int
	}{
		{
			name:     "valid TCP",
			protocol: netguardv1beta1.ProtocolTCP,
			wantErrs: 0,
		},
		{
			name:     "valid UDP",
			protocol: netguardv1beta1.ProtocolUDP,
			wantErrs: 0,
		},
		{
			name:     "empty protocol",
			protocol: "",
			wantErrs: 0, // Empty handled by required field validation
		},
		{
			name:     "invalid protocol",
			protocol: "HTTP",
			wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateTransportProtocol(tt.protocol, fldPath)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateTransportProtocol() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestValidateTraffic(t *testing.T) {
	fldPath := field.NewPath("traffic")

	tests := []struct {
		name     string
		traffic  netguardv1beta1.Traffic
		wantErrs int
	}{
		{
			name:     "valid INGRESS",
			traffic:  netguardv1beta1.INGRESS,
			wantErrs: 0,
		},
		{
			name:     "valid EGRESS",
			traffic:  netguardv1beta1.EGRESS,
			wantErrs: 0,
		},
		{
			name:     "empty traffic",
			traffic:  "",
			wantErrs: 0, // Empty handled by required field validation
		},
		{
			name:     "invalid traffic",
			traffic:  "BIDIRECTIONAL",
			wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateTraffic(tt.traffic, fldPath)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateTraffic() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestValidateRuleAction(t *testing.T) {
	fldPath := field.NewPath("action")

	tests := []struct {
		name     string
		action   netguardv1beta1.RuleAction
		wantErrs int
	}{
		{
			name:     "valid ACCEPT",
			action:   netguardv1beta1.ActionAccept,
			wantErrs: 0,
		},
		{
			name:     "valid DROP",
			action:   netguardv1beta1.ActionDrop,
			wantErrs: 0,
		},
		{
			name:     "empty action",
			action:   "",
			wantErrs: 0, // Empty handled by required field validation
		},
		{
			name:     "invalid action",
			action:   "REJECT",
			wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateRuleAction(tt.action, fldPath)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateRuleAction() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestValidationHelpers(t *testing.T) {
	vh := NewValidationHelpers()
	fldPath := field.NewPath("field")

	t.Run("ValidateRequiredString", func(t *testing.T) {
		tests := []struct {
			name     string
			value    string
			wantErrs int
		}{
			{
				name:     "valid string",
				value:    "test-value",
				wantErrs: 0,
			},
			{
				name:     "empty string",
				value:    "",
				wantErrs: 1,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				errs := vh.ValidateRequiredString(tt.value, fldPath, "testField")
				if len(errs) != tt.wantErrs {
					t.Errorf("ValidateRequiredString() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
				}
			})
		}
	})

	t.Run("ValidateOptionalString", func(t *testing.T) {
		validator := func(value string, fldPath *field.Path) field.ErrorList {
			if value == "invalid" {
				return field.ErrorList{field.Invalid(fldPath, value, "test validation error")}
			}
			return field.ErrorList{}
		}

		tests := []struct {
			name     string
			value    string
			wantErrs int
		}{
			{
				name:     "empty string (skipped)",
				value:    "",
				wantErrs: 0,
			},
			{
				name:     "valid string",
				value:    "valid",
				wantErrs: 0,
			},
			{
				name:     "invalid string",
				value:    "invalid",
				wantErrs: 1,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				errs := vh.ValidateOptionalString(tt.value, fldPath, validator)
				if len(errs) != tt.wantErrs {
					t.Errorf("ValidateOptionalString() got %d errors, want %d errors: %v", len(errs), tt.wantErrs, errs)
				}
			})
		}
	})
}
