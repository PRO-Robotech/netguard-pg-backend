package validation

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestIEAgAgRuleValidator_ValidateCreate(t *testing.T) {
	validator := NewIEAgAgRuleValidator()
	ctx := context.Background()

	tests := []struct {
		name        string
		rule        *v1beta1.IEAgAgRule
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid IEAgAgRule with TCP transport",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-rule",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Description: "Test rule for TCP traffic",
					Transport:   v1beta1.ProtocolTCP,
					Traffic:     v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
					Ports: []v1beta1.PortSpec{
						{
							Port: 80,
						},
					},
					Action:   "ACCEPT",
					Priority: 100,
				},
			},
			expectError: false,
		},
		{
			name: "valid IEAgAgRule with UDP transport and port range",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-udp-rule",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Description: "Test rule for UDP traffic",
					Transport:   v1beta1.ProtocolUDP,
					Traffic:     v1beta1.EGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
					Ports: []v1beta1.PortSpec{
						{
							PortRange: &v1beta1.PortRange{
								From: 8000,
								To:   8999,
							},
						},
					},
					Action:   "DROP",
					Priority: 200,
				},
			},
			expectError: false,
		},
		{
			name: "valid IEAgAgRule with no ports (allow all)",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "allow-all-rule",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Description: "Allow all traffic",
					Transport:   v1beta1.ProtocolTCP,
					Traffic:     v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
					Action:   "ACCEPT",
					Priority: 0,
				},
			},
			expectError: false,
		},
		{
			name:        "nil IEAgAgRule object",
			rule:        nil,
			expectError: true,
			errorMsg:    "ieagagrule object cannot be nil",
		},
		{
			name: "missing name in metadata",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "name or generateName is required",
		},
		{
			name: "invalid name format",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Rule_Name",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "description too long",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "long-desc-rule",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Description: strings.Repeat("a", 513), // Too long (>512 chars)
					Transport:   v1beta1.ProtocolTCP,
					Traffic:     v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "Too long",
		},
		{
			name: "missing transport",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-transport",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					// Missing Transport
					Traffic: v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "transport is required",
		},
		{
			name: "invalid transport value",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-transport",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: "INVALID", // Invalid transport
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "Unsupported value: \"INVALID\": supported values: \"TCP\", \"UDP\"",
		},
		{
			name: "missing traffic",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-traffic",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					// Missing Traffic
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "traffic is required",
		},
		{
			name: "invalid traffic value",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-traffic",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   "INVALID", // Invalid traffic
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "Unsupported value: \"INVALID\": supported values: \"INGRESS\", \"EGRESS\"",
		},
		{
			name: "missing AddressGroupLocal fields",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-local-ag",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport:         v1beta1.ProtocolTCP,
					Traffic:           v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						// Missing required fields
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "wrong API version in AddressGroupLocal",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-local-ag-api",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "v1", // Wrong API version
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion must be 'netguard.sgroups.io/v1beta1'",
		},
		{
			name: "wrong kind in AddressGroupLocal",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-local-ag-kind",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service", // Should be AddressGroup
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'AddressGroup'",
		},
		{
			name: "missing AddressGroup fields",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-remote-ag",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						// Missing required fields
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "wrong API version in AddressGroup",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-remote-ag-api",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "networking.k8s.io/v1", // Wrong API version
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion must be 'netguard.sgroups.io/v1beta1'",
		},
		{
			name: "wrong kind in AddressGroup",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-remote-ag-kind",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Service", // Should be AddressGroup
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'AddressGroup'",
		},
		{
			name: "invalid name format in AddressGroupLocal",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-local-ag-name",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "Invalid_Local_AG",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "invalid name format in AddressGroup",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-remote-ag-name",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "Invalid_Remote_AG",
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "invalid action value",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-action",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
					Action: "INVALID", // Invalid action
				},
			},
			expectError: true,
			errorMsg:    "Unsupported value: \"INVALID\": supported values: \"ACCEPT\", \"DROP\"",
		},
		{
			name: "negative priority",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "negative-priority",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
					Priority: -1, // Negative priority
				},
			},
			expectError: true,
			errorMsg:    "priority must be non-negative",
		},
		{
			name: "port spec with neither port nor portRange",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-port-spec",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
					Ports: []v1beta1.PortSpec{
						{
							// Neither Port nor PortRange specified
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "either port or portRange must be specified",
		},
		{
			name: "port spec with both port and portRange",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "both-port-types",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
					Ports: []v1beta1.PortSpec{
						{
							Port: 80, // Both port and portRange specified
							PortRange: &v1beta1.PortRange{
								From: 8000,
								To:   8999,
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "cannot specify both port and portRange",
		},
		{
			name: "invalid port number (too low)",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "port-too-low",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
					Ports: []v1beta1.PortSpec{
						{
							Port: 0, // Port 0 is treated as "not specified", expecting specific error message
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "either port or portRange must be specified",
		},
		{
			name: "invalid port number (too high)",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "port-too-high",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
					Ports: []v1beta1.PortSpec{
						{
							Port: 70000, // Invalid port (too high)
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "port must be between 1 and 65535",
		},
		{
			name: "invalid port range (from too low)",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "range-from-too-low",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
					Ports: []v1beta1.PortSpec{
						{
							PortRange: &v1beta1.PortRange{
								From: 0, // Invalid from port (too low)
								To:   8999,
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "from port must be between 1 and 65535",
		},
		{
			name: "invalid port range (to too high)",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "range-to-too-high",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
					Ports: []v1beta1.PortSpec{
						{
							PortRange: &v1beta1.PortRange{
								From: 8000,
								To:   70000, // Invalid to port (too high)
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "to port must be between 1 and 65535",
		},
		{
			name: "invalid port range (from > to)",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "range-invalid-order",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
					Ports: []v1beta1.PortSpec{
						{
							PortRange: &v1beta1.PortRange{
								From: 9000, // From > To
								To:   8000,
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "from port must be less than or equal to to port",
		},
		{
			name: "valid generateName instead of name",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "auto-rule",
					Namespace:    "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid generateName format",
			rule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "Invalid_GenerateName",
					Namespace:    "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
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

func TestIEAgAgRuleValidator_ValidateUpdate(t *testing.T) {
	validator := NewIEAgAgRuleValidator()
	ctx := context.Background()

	baseRule := &v1beta1.IEAgAgRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rule",
			Namespace: "default",
		},
		Spec: v1beta1.IEAgAgRuleSpec{
			Description: "Test rule",
			Transport:   v1beta1.ProtocolTCP,
			Traffic:     v1beta1.INGRESS,
			AddressGroupLocal: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "local-ag",
			},
			AddressGroup: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "remote-ag",
			},
			Ports: []v1beta1.PortSpec{
				{
					Port: 80,
				},
			},
			Action:   "ACCEPT",
			Priority: 100,
		},
	}

	tests := []struct {
		name        string
		newRule     *v1beta1.IEAgAgRule
		oldRule     *v1beta1.IEAgAgRule
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid update (no immutable field changes)",
			newRule:     baseRule.DeepCopy(),
			oldRule:     baseRule,
			expectError: false,
		},
		{
			name: "attempt to change transport (immutable)",
			newRule: func() *v1beta1.IEAgAgRule {
				rule := baseRule.DeepCopy()
				rule.Spec.Transport = v1beta1.ProtocolUDP
				return rule
			}(),
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "transport is immutable and cannot be changed",
		},
		{
			name: "attempt to change traffic (immutable)",
			newRule: func() *v1beta1.IEAgAgRule {
				rule := baseRule.DeepCopy()
				rule.Spec.Traffic = v1beta1.EGRESS
				return rule
			}(),
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "traffic is immutable and cannot be changed",
		},
		{
			name: "attempt to change addressGroupLocal (immutable)",
			newRule: func() *v1beta1.IEAgAgRule {
				rule := baseRule.DeepCopy()
				rule.Spec.AddressGroupLocal.Name = "different-local-ag"
				return rule
			}(),
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "addressGroupLocal is immutable and cannot be changed",
		},
		{
			name: "attempt to change addressGroup (immutable)",
			newRule: func() *v1beta1.IEAgAgRule {
				rule := baseRule.DeepCopy()
				rule.Spec.AddressGroup.Name = "different-remote-ag"
				return rule
			}(),
			oldRule:     baseRule,
			expectError: true,
			errorMsg:    "addressGroup is immutable and cannot be changed",
		},
		{
			name: "valid change to description",
			newRule: func() *v1beta1.IEAgAgRule {
				rule := baseRule.DeepCopy()
				rule.Spec.Description = "Updated description"
				return rule
			}(),
			oldRule:     baseRule,
			expectError: false,
		},
		{
			name: "valid change to ports",
			newRule: func() *v1beta1.IEAgAgRule {
				rule := baseRule.DeepCopy()
				rule.Spec.Ports = []v1beta1.PortSpec{
					{
						Port: 8080,
					},
				}
				return rule
			}(),
			oldRule:     baseRule,
			expectError: false,
		},
		{
			name: "valid change to action",
			newRule: func() *v1beta1.IEAgAgRule {
				rule := baseRule.DeepCopy()
				rule.Spec.Action = "DROP"
				return rule
			}(),
			oldRule:     baseRule,
			expectError: false,
		},
		{
			name: "valid change to priority",
			newRule: func() *v1beta1.IEAgAgRule {
				rule := baseRule.DeepCopy()
				rule.Spec.Priority = 200
				return rule
			}(),
			oldRule:     baseRule,
			expectError: false,
		},
		{
			name: "valid metadata update (labels, annotations)",
			newRule: func() *v1beta1.IEAgAgRule {
				rule := baseRule.DeepCopy()
				rule.Labels = map[string]string{"new-label": "new-value"}
				rule.Annotations = map[string]string{"new-annotation": "new-value"}
				return rule
			}(),
			oldRule:     baseRule,
			expectError: false,
		},
		{
			name: "update with invalid new data",
			newRule: &v1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Name",
					Namespace: "default",
				},
				Spec: v1beta1.IEAgAgRuleSpec{
					Transport: v1beta1.ProtocolTCP,
					Traffic:   v1beta1.INGRESS,
					AddressGroupLocal: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "local-ag",
					},
					AddressGroup: v1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "remote-ag",
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

func TestIEAgAgRuleValidator_ValidateDelete(t *testing.T) {
	validator := NewIEAgAgRuleValidator()
	ctx := context.Background()

	rule := &v1beta1.IEAgAgRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rule",
			Namespace: "default",
		},
		Spec: v1beta1.IEAgAgRuleSpec{
			Description: "Test rule",
			Transport:   v1beta1.ProtocolTCP,
			Traffic:     v1beta1.INGRESS,
			AddressGroupLocal: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "local-ag",
			},
			AddressGroup: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "remote-ag",
			},
			Ports: []v1beta1.PortSpec{
				{
					Port: 80,
				},
			},
			Action:   "ACCEPT",
			Priority: 100,
		},
	}

	errors := validator.ValidateDelete(ctx, rule)
	if len(errors) > 0 {
		t.Errorf("expected no validation errors for delete but got: %v", errors)
	}
}
