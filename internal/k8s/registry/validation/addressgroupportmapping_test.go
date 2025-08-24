package validation

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestAddressGroupPortMappingValidator_ValidateCreate(t *testing.T) {
	validator := NewAddressGroupPortMappingValidator()
	ctx := context.Background()

	tests := []struct {
		name        string
		mapping     *v1beta1.AddressGroupPortMapping
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid AddressGroupPortMapping with TCP ports",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-mapping",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "Service",
									Name:       "my-service",
								},
								Namespace: "default",
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										Port:        "80",
										Description: "HTTP port",
									},
									{
										Port:        "8080",
										Description: "Alternative HTTP port",
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid AddressGroupPortMapping with UDP ports",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "udp-mapping",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "Service",
									Name:       "dns-service",
								},
								Namespace: "kube-system",
							},
							Ports: v1beta1.ProtocolPorts{
								UDP: []v1beta1.PortConfig{
									{
										Port:        "53",
										Description: "DNS port",
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid AddressGroupPortMapping with both TCP and UDP ports",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mixed-mapping",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "Service",
									Name:       "mixed-service",
								},
								Namespace: "default",
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										Port:        "80",
										Description: "HTTP",
									},
								},
								UDP: []v1beta1.PortConfig{
									{
										Port:        "53",
										Description: "DNS",
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid empty AccessPorts",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-mapping",
					Namespace: "default",
				},
				Spec:        v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{Items: []v1beta1.ServicePortsRef{}},
			},
			expectError: false,
		},
		{
			name:        "nil AddressGroupPortMapping object",
			mapping:     nil,
			expectError: true,
			errorMsg:    "addressgroupportmapping object cannot be nil",
		},
		{
			name: "missing name in metadata",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec:        v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{Items: []v1beta1.ServicePortsRef{}},
			},
			expectError: true,
			errorMsg:    "name or generateName is required",
		},
		{
			name: "invalid name format",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Mapping_Name",
					Namespace: "default",
				},
				Spec:        v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{Items: []v1beta1.ServicePortsRef{}},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "missing service reference fields",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-service-ref",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								// Missing required fields
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										Port:        "80",
										Description: "HTTP",
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "wrong kind in service reference",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-service-kind",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "AddressGroup", // Should be Service
									Name:       "my-service",
								},
								Namespace: "default",
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										Port:        "80",
										Description: "HTTP",
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "kind must be 'Service'",
		},
		{
			name: "wrong API version in service reference",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-service-api",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "v1", // Wrong API version
									Kind:       "Service",
									Name:       "my-service",
								},
								Namespace: "default",
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										Port:        "80",
										Description: "HTTP",
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "apiVersion must be 'netguard.sgroups.io/v1beta1'",
		},
		{
			name: "missing namespace in service reference",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-service-ns",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "Service",
									Name:       "my-service",
								},
								// Missing namespace
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										Port:        "80",
										Description: "HTTP",
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "namespace is required",
		},
		{
			name: "invalid service name format",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-service-name",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "Service",
									Name:       "Invalid_Service_Name",
								},
								Namespace: "default",
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										Port:        "80",
										Description: "HTTP",
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
		{
			name: "missing port in TCP configuration",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-port",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "Service",
									Name:       "my-service",
								},
								Namespace: "default",
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										// Missing port
										Description: "HTTP",
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "port is required",
		},
		{
			name: "port string too long",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "port-too-long",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "Service",
									Name:       "my-service",
								},
								Namespace: "default",
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										Port:        strings.Repeat("8", 40), // Too long (>32 chars)
										Description: "HTTP",
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "Too long",
		},
		{
			name: "description too long",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "desc-too-long",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "Service",
									Name:       "my-service",
								},
								Namespace: "default",
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										Port:        "80",
										Description: strings.Repeat("a", 300), // Too long (>256 chars)
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "Too long",
		},
		{
			name: "multiple service references (valid)",
			mapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-service-mapping",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "Service",
									Name:       "web-service",
								},
								Namespace: "default",
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										Port:        "80",
										Description: "HTTP",
									},
								},
							},
						},
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "Service",
									Name:       "database-service",
								},
								Namespace: "default",
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										Port:        "5432",
										Description: "PostgreSQL",
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateCreate(ctx, tt.mapping)

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

func TestAddressGroupPortMappingValidator_ValidateUpdate(t *testing.T) {
	validator := NewAddressGroupPortMappingValidator()
	ctx := context.Background()

	baseMapping := &v1beta1.AddressGroupPortMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mapping",
			Namespace: "default",
		},
		Spec: v1beta1.AddressGroupPortMappingSpec{},
		AccessPorts: v1beta1.AccessPortsSpec{
			Items: []v1beta1.ServicePortsRef{
				{
					NamespacedObjectReference: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
					Ports: v1beta1.ProtocolPorts{
						TCP: []v1beta1.PortConfig{
							{
								Port:        "80",
								Description: "HTTP port",
							},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name        string
		newMapping  *v1beta1.AddressGroupPortMapping
		oldMapping  *v1beta1.AddressGroupPortMapping
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid update (no changes)",
			newMapping:  baseMapping.DeepCopy(),
			oldMapping:  baseMapping,
			expectError: false,
		},
		{
			name: "valid port configuration change",
			newMapping: func() *v1beta1.AddressGroupPortMapping {
				mapping := baseMapping.DeepCopy()
				mapping.AccessPorts.Items[0].Ports.TCP[0].Port = "8080"
				mapping.AccessPorts.Items[0].Ports.TCP[0].Description = "Alternative HTTP port"
				return mapping
			}(),
			oldMapping:  baseMapping,
			expectError: false,
		},
		{
			name: "valid service reference change",
			newMapping: func() *v1beta1.AddressGroupPortMapping {
				mapping := baseMapping.DeepCopy()
				mapping.AccessPorts.Items[0].NamespacedObjectReference.Name = "different-service"
				return mapping
			}(),
			oldMapping:  baseMapping,
			expectError: false,
		},
		{
			name: "add new UDP ports",
			newMapping: func() *v1beta1.AddressGroupPortMapping {
				mapping := baseMapping.DeepCopy()
				mapping.AccessPorts.Items[0].Ports.UDP = []v1beta1.PortConfig{
					{
						Port:        "53",
						Description: "DNS port",
					},
				}
				return mapping
			}(),
			oldMapping:  baseMapping,
			expectError: false,
		},
		{
			name: "update with invalid new data",
			newMapping: &v1beta1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "Invalid_Name",
					Namespace: "default",
				},
				Spec: v1beta1.AddressGroupPortMappingSpec{},
				AccessPorts: v1beta1.AccessPortsSpec{
					Items: []v1beta1.ServicePortsRef{
						{
							NamespacedObjectReference: v1beta1.NamespacedObjectReference{
								ObjectReference: v1beta1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1beta1",
									Kind:       "Service",
									Name:       "my-service",
								},
								Namespace: "default",
							},
							Ports: v1beta1.ProtocolPorts{
								TCP: []v1beta1.PortConfig{
									{
										Port:        "80",
										Description: "HTTP",
									},
								},
							},
						},
					},
				},
			},
			oldMapping:  baseMapping,
			expectError: true,
			errorMsg:    "RFC 1123 subdomain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateUpdate(ctx, tt.newMapping, tt.oldMapping)

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

func TestAddressGroupPortMappingValidator_ValidateDelete(t *testing.T) {
	validator := NewAddressGroupPortMappingValidator()
	ctx := context.Background()

	mapping := &v1beta1.AddressGroupPortMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mapping",
			Namespace: "default",
		},
		Spec: v1beta1.AddressGroupPortMappingSpec{},
		AccessPorts: v1beta1.AccessPortsSpec{
			Items: []v1beta1.ServicePortsRef{
				{
					NamespacedObjectReference: v1beta1.NamespacedObjectReference{
						ObjectReference: v1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "Service",
							Name:       "my-service",
						},
						Namespace: "default",
					},
					Ports: v1beta1.ProtocolPorts{
						TCP: []v1beta1.PortConfig{
							{
								Port:        "80",
								Description: "HTTP port",
							},
						},
					},
				},
			},
		},
	}

	errors := validator.ValidateDelete(ctx, mapping)
	if len(errors) > 0 {
		t.Errorf("expected no validation errors for delete but got: %v", errors)
	}
}
