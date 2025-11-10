package convert

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestIEAgAgRuleConverter_ToDomain(t *testing.T) {
	ctx := context.Background()
	converter := NewIEAgAgRuleConverter()

	// Test cases
	testCases := []struct {
		name        string
		input       *netguardv1beta1.IEAgAgRule
		expected    *models.IEAgAgRule
		expectError bool
	}{
		{
			name: "valid rule with minimal fields",
			input: &netguardv1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rule",
					Namespace: "default",
				},
				Spec: netguardv1beta1.IEAgAgRuleSpec{
					Transport: netguardv1beta1.ProtocolTCP,
					Traffic:   netguardv1beta1.INGRESS,
					AddressGroupLocal: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "local-ag",
						},
					},
					AddressGroup: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "remote-ag",
						},
					},
					Action: netguardv1beta1.ActionAccept,
				},
			},
			expected: &models.IEAgAgRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-rule",
						Namespace: "default",
					},
				},
				Transport: models.TCP,
				Traffic:   models.INGRESS,
				AddressGroupLocal: models.NewAddressGroupRef(
					"local-ag",
					models.WithNamespace("default"), // inherited from rule namespace
				),
				AddressGroup: models.NewAddressGroupRef(
					"remote-ag",
					models.WithNamespace("default"), // inherited from rule namespace
				),
				Action:   models.ActionAccept,
				Logs:     false,
				Priority: 0,
				Meta: models.Meta{
					UID:                "",
					ResourceVersion:    "",
					Generation:         0,
					CreationTS:         metav1.Time{},
					ObservedGeneration: 0,
					Conditions:         nil,
				},
			},
		},
		{
			name: "valid rule with ports - single port",
			input: &netguardv1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rule-port",
					Namespace: "webapp",
				},
				Spec: netguardv1beta1.IEAgAgRuleSpec{
					Transport: netguardv1beta1.ProtocolUDP,
					Traffic:   netguardv1beta1.EGRESS,
					AddressGroupLocal: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "local-ag",
						},
					},
					AddressGroup: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "remote-ag",
						},
					},
					Ports: []netguardv1beta1.PortSpec{
						{
							Port: 80,
						},
						{
							Port: 443,
						},
					},
					Action:   netguardv1beta1.ActionDrop,
					Priority: 100,
				},
			},
			expected: &models.IEAgAgRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-rule-port",
						Namespace: "webapp",
					},
				},
				Transport: models.UDP,
				Traffic:   models.EGRESS,
				AddressGroupLocal: models.NewAddressGroupRef(
					"local-ag",
					models.WithNamespace("webapp"),
				),
				AddressGroup: models.NewAddressGroupRef(
					"remote-ag",
					models.WithNamespace("webapp"),
				),
				Ports: []models.PortSpec{
					{Destination: "80"},
					{Destination: "443"},
				},
				Action:   models.ActionDrop,
				Logs:     false,
				Priority: 100,
				Meta: models.Meta{
					UID:                "",
					ResourceVersion:    "",
					Generation:         0,
					CreationTS:         metav1.Time{},
					ObservedGeneration: 0,
					Conditions:         nil,
				},
			},
		},
		{
			name: "valid rule with port ranges",
			input: &netguardv1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rule-range",
					Namespace: "default",
				},
				Spec: netguardv1beta1.IEAgAgRuleSpec{
					Transport: netguardv1beta1.ProtocolTCP,
					Traffic:   netguardv1beta1.INGRESS,
					AddressGroupLocal: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "local-ag",
						},
					},
					AddressGroup: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "remote-ag",
						},
					},
					Ports: []netguardv1beta1.PortSpec{
						{
							PortRange: &netguardv1beta1.PortRange{
								From: 8000,
								To:   8999,
							},
						},
						{
							Port: 443,
						},
					},
					Action: netguardv1beta1.ActionAccept,
				},
			},
			expected: &models.IEAgAgRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-rule-range",
						Namespace: "default",
					},
				},
				Transport: models.TCP,
				Traffic:   models.INGRESS,
				AddressGroupLocal: models.NewAddressGroupRef(
					"local-ag",
					models.WithNamespace("default"),
				),
				AddressGroup: models.NewAddressGroupRef(
					"remote-ag",
					models.WithNamespace("default"),
				),
				Ports: []models.PortSpec{
					{Destination: "8000-8999"},
					{Destination: "443"},
				},
				Action:   models.ActionAccept,
				Logs:     false,
				Priority: 0,
				Meta: models.Meta{
					UID:                "",
					ResourceVersion:    "",
					Generation:         0,
					CreationTS:         metav1.Time{},
					ObservedGeneration: 0,
					Conditions:         nil,
				},
			},
		},
		{
			name: "valid rule with full metadata",
			input: &netguardv1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-rule-full",
					Namespace:         "test-ns",
					UID:               types.UID("test-uid"),
					ResourceVersion:   "123",
					Generation:        2,
					CreationTimestamp: metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					Labels: map[string]string{
						"app": "test",
						"env": "prod",
					},
					Annotations: map[string]string{
						"note": "test annotation",
					},
				},
				Spec: netguardv1beta1.IEAgAgRuleSpec{
					Transport: netguardv1beta1.ProtocolTCP,
					Traffic:   netguardv1beta1.INGRESS,
					AddressGroupLocal: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "local-ag",
						},
					},
					AddressGroup: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "remote-ag",
						},
					},
					Action:   netguardv1beta1.ActionAccept,
					Priority: 200,
				},
				Status: netguardv1beta1.IEAgAgRuleStatus{
					ObservedGeneration: 2,
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "AllReady",
						},
					},
				},
			},
			expected: &models.IEAgAgRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-rule-full",
						Namespace: "test-ns",
					},
				},
				Transport: models.TCP,
				Traffic:   models.INGRESS,
				AddressGroupLocal: models.NewAddressGroupRef(
					"local-ag",
					models.WithNamespace("test-ns"),
				),
				AddressGroup: models.NewAddressGroupRef(
					"remote-ag",
					models.WithNamespace("test-ns"),
				),
				Action:   models.ActionAccept,
				Logs:     false,
				Priority: 200,
				Meta: models.Meta{
					UID:                "test-uid",
					ResourceVersion:    "123",
					Generation:         2,
					CreationTS:         metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					ObservedGeneration: 2,
					Labels: map[string]string{
						"app": "test",
						"env": "prod",
					},
					Annotations: map[string]string{
						"note": "test annotation",
					},
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "AllReady",
						},
					},
				},
			},
		},
		{
			name:        "nil input",
			input:       nil,
			expected:    nil,
			expectError: true,
		},
		{
			name: "invalid transport protocol",
			input: &netguardv1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rule",
					Namespace: "default",
				},
				Spec: netguardv1beta1.IEAgAgRuleSpec{
					Transport: "INVALID",
					Traffic:   netguardv1beta1.INGRESS,
					AddressGroupLocal: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "local-ag",
						},
					},
					AddressGroup: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "remote-ag",
						},
					},
					Action: netguardv1beta1.ActionAccept,
				},
			},
			expected:    nil,
			expectError: true,
		},
		{
			name: "invalid traffic direction",
			input: &netguardv1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rule",
					Namespace: "default",
				},
				Spec: netguardv1beta1.IEAgAgRuleSpec{
					Transport: netguardv1beta1.ProtocolTCP,
					Traffic:   "INVALID",
					AddressGroupLocal: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "local-ag",
						},
					},
					AddressGroup: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "remote-ag",
						},
					},
					Action: netguardv1beta1.ActionAccept,
				},
			},
			expected:    nil,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.ToDomain(ctx, tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestIEAgAgRuleConverter_FromDomain(t *testing.T) {
	ctx := context.Background()
	converter := NewIEAgAgRuleConverter()

	// Test cases
	testCases := []struct {
		name        string
		input       *models.IEAgAgRule
		expectError bool
		checkFunc   func(t *testing.T, result *netguardv1beta1.IEAgAgRule)
	}{
		{
			name: "valid rule with minimal fields",
			input: &models.IEAgAgRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-rule",
						Namespace: "default",
					},
				},
				Transport: models.TCP,
				Traffic:   models.INGRESS,
				AddressGroupLocal: models.NewAddressGroupRef(
					"local-ag",
					models.WithNamespace("default"),
				),
				AddressGroup: models.NewAddressGroupRef(
					"remote-ag",
					models.WithNamespace("default"),
				),
				Action: models.ActionAccept,
			},
			checkFunc: func(t *testing.T, result *netguardv1beta1.IEAgAgRule) {
				assert.Equal(t, "netguard.sgroups.io/v1beta1", result.TypeMeta.APIVersion)
				assert.Equal(t, "IEAgAgRule", result.TypeMeta.Kind)
				assert.Equal(t, "test-rule", result.Name)
				assert.Equal(t, "default", result.Namespace)
				assert.Equal(t, netguardv1beta1.ProtocolTCP, result.Spec.Transport)
				assert.Equal(t, netguardv1beta1.INGRESS, result.Spec.Traffic)
				assert.Equal(t, "local-ag", result.Spec.AddressGroupLocal.Name)
				assert.Equal(t, "remote-ag", result.Spec.AddressGroup.Name)
				assert.Equal(t, netguardv1beta1.ActionAccept, result.Spec.Action)
			},
		},
		{
			name: "valid rule with single ports",
			input: &models.IEAgAgRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-rule-port",
						Namespace: "webapp",
					},
				},
				Transport: models.UDP,
				Traffic:   models.EGRESS,
				AddressGroupLocal: models.NewAddressGroupRef(
					"local-ag",
					models.WithNamespace("webapp"),
				),
				AddressGroup: models.NewAddressGroupRef(
					"remote-ag",
					models.WithNamespace("webapp"),
				),
				Ports: []models.PortSpec{
					{Destination: "80"},
					{Destination: "443"},
				},
				Action:   models.ActionDrop,
				Priority: 100,
			},
			checkFunc: func(t *testing.T, result *netguardv1beta1.IEAgAgRule) {
				assert.Equal(t, "test-rule-port", result.Name)
				assert.Equal(t, "webapp", result.Namespace)
				assert.Equal(t, netguardv1beta1.ProtocolUDP, result.Spec.Transport)
				assert.Equal(t, netguardv1beta1.EGRESS, result.Spec.Traffic)
				assert.Equal(t, netguardv1beta1.ActionDrop, result.Spec.Action)
				assert.Equal(t, int32(100), result.Spec.Priority)
				require.Len(t, result.Spec.Ports, 2)
				assert.Equal(t, int32(80), result.Spec.Ports[0].Port)
				assert.Equal(t, int32(443), result.Spec.Ports[1].Port)
				assert.Nil(t, result.Spec.Ports[0].PortRange)
				assert.Nil(t, result.Spec.Ports[1].PortRange)
			},
		},
		{
			name: "valid rule with port ranges",
			input: &models.IEAgAgRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-rule-range",
						Namespace: "default",
					},
				},
				Transport: models.TCP,
				Traffic:   models.INGRESS,
				AddressGroupLocal: models.NewAddressGroupRef(
					"local-ag",
					models.WithNamespace("default"),
				),
				AddressGroup: models.NewAddressGroupRef(
					"remote-ag",
					models.WithNamespace("default"),
				),
				Ports: []models.PortSpec{
					{Destination: "8000-8999"},
					{Destination: "443"},
				},
				Action: models.ActionAccept,
			},
			checkFunc: func(t *testing.T, result *netguardv1beta1.IEAgAgRule) {
				assert.Equal(t, "test-rule-range", result.Name)
				require.Len(t, result.Spec.Ports, 2)

				// Check port range
				assert.NotNil(t, result.Spec.Ports[0].PortRange)
				assert.Equal(t, int32(8000), result.Spec.Ports[0].PortRange.From)
				assert.Equal(t, int32(8999), result.Spec.Ports[0].PortRange.To)
				assert.Equal(t, int32(0), result.Spec.Ports[0].Port)

				// Check single port
				assert.Nil(t, result.Spec.Ports[1].PortRange)
				assert.Equal(t, int32(443), result.Spec.Ports[1].Port)
			},
		},
		{
			name:        "nil input",
			input:       nil,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.FromDomain(ctx, tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				tc.checkFunc(t, result)
			}
		})
	}
}

func TestIEAgAgRuleConverter_RoundTrip(t *testing.T) {
	ctx := context.Background()
	converter := NewIEAgAgRuleConverter()

	// Test cases for round-trip conversion
	testCases := []struct {
		name string
		k8s  *netguardv1beta1.IEAgAgRule
	}{
		{
			name: "basic rule",
			k8s: &netguardv1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rule",
					Namespace: "default",
				},
				Spec: netguardv1beta1.IEAgAgRuleSpec{
					Transport: netguardv1beta1.ProtocolTCP,
					Traffic:   netguardv1beta1.INGRESS,
					AddressGroupLocal: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "local-ag",
						},
					},
					AddressGroup: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "remote-ag",
						},
					},
					Action: netguardv1beta1.ActionAccept,
				},
			},
		},
		{
			name: "rule with ports",
			k8s: &netguardv1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rule-ports",
					Namespace: "webapp",
				},
				Spec: netguardv1beta1.IEAgAgRuleSpec{
					Transport: netguardv1beta1.ProtocolUDP,
					Traffic:   netguardv1beta1.EGRESS,
					AddressGroupLocal: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "local-ag",
						},
					},
					AddressGroup: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "remote-ag",
						},
					},
					Ports: []netguardv1beta1.PortSpec{
						{Port: 80},
						{PortRange: &netguardv1beta1.PortRange{From: 8000, To: 8999}},
					},
					Action:   netguardv1beta1.ActionDrop,
					Priority: 150,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// K8s -> Domain
			domain, err := converter.ToDomain(ctx, tc.k8s)
			require.NoError(t, err)
			require.NotNil(t, domain)

			// Domain -> K8s
			k8s, err := converter.FromDomain(ctx, domain)
			require.NoError(t, err)
			require.NotNil(t, k8s)

			// Compare essential fields
			assert.Equal(t, tc.k8s.ObjectMeta.Name, k8s.ObjectMeta.Name)
			assert.Equal(t, tc.k8s.ObjectMeta.Namespace, k8s.ObjectMeta.Namespace)
			assert.Equal(t, tc.k8s.Spec.Transport, k8s.Spec.Transport)
			assert.Equal(t, tc.k8s.Spec.Traffic, k8s.Spec.Traffic)
			assert.Equal(t, tc.k8s.Spec.AddressGroupLocal.Name, k8s.Spec.AddressGroupLocal.Name)
			assert.Equal(t, tc.k8s.Spec.AddressGroup.Name, k8s.Spec.AddressGroup.Name)
			assert.Equal(t, tc.k8s.Spec.Action, k8s.Spec.Action)
			assert.Equal(t, tc.k8s.Spec.Priority, k8s.Spec.Priority)
			assert.Equal(t, len(tc.k8s.Spec.Ports), len(k8s.Spec.Ports))

			// Verify TypeMeta is set correctly
			assert.Equal(t, "netguard.sgroups.io/v1beta1", k8s.TypeMeta.APIVersion)
			assert.Equal(t, "IEAgAgRule", k8s.TypeMeta.Kind)
		})
	}
}

func TestIEAgAgRuleConverter_EnumConversions(t *testing.T) {
	ctx := context.Background()
	converter := NewIEAgAgRuleConverter()

	// Test enum conversions
	testCases := []struct {
		name            string
		k8sTransport    netguardv1beta1.TransportProtocol
		domainTransport models.TransportProtocol
		k8sTraffic      netguardv1beta1.Traffic
		domainTraffic   models.Traffic
		k8sAction       netguardv1beta1.RuleAction
		domainAction    models.RuleAction
	}{
		{
			name:            "TCP Ingress Accept",
			k8sTransport:    netguardv1beta1.ProtocolTCP,
			domainTransport: models.TCP,
			k8sTraffic:      netguardv1beta1.INGRESS,
			domainTraffic:   models.INGRESS,
			k8sAction:       netguardv1beta1.ActionAccept,
			domainAction:    models.ActionAccept,
		},
		{
			name:            "UDP Egress Drop",
			k8sTransport:    netguardv1beta1.ProtocolUDP,
			domainTransport: models.UDP,
			k8sTraffic:      netguardv1beta1.EGRESS,
			domainTraffic:   models.EGRESS,
			k8sAction:       netguardv1beta1.ActionDrop,
			domainAction:    models.ActionDrop,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// K8s -> Domain
			k8sObj := &netguardv1beta1.IEAgAgRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rule",
					Namespace: "default",
				},
				Spec: netguardv1beta1.IEAgAgRuleSpec{
					Transport: tc.k8sTransport,
					Traffic:   tc.k8sTraffic,
					AddressGroupLocal: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "local-ag",
						},
					},
					AddressGroup: netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							Name: "remote-ag",
						},
					},
					Action: tc.k8sAction,
				},
			}

			domain, err := converter.ToDomain(ctx, k8sObj)
			require.NoError(t, err)
			assert.Equal(t, tc.domainTransport, domain.Transport)
			assert.Equal(t, tc.domainTraffic, domain.Traffic)
			assert.Equal(t, tc.domainAction, domain.Action)

			// Domain -> K8s
			k8sResult, err := converter.FromDomain(ctx, domain)
			require.NoError(t, err)
			assert.Equal(t, tc.k8sTransport, k8sResult.Spec.Transport)
			assert.Equal(t, tc.k8sTraffic, k8sResult.Spec.Traffic)
			assert.Equal(t, tc.k8sAction, k8sResult.Spec.Action)
		})
	}
}

func TestIEAgAgRuleConverter_PortParsing(t *testing.T) {
	ctx := context.Background()
	converter := NewIEAgAgRuleConverter()

	// Test port parsing edge cases
	testCases := []struct {
		name             string
		domainPorts      []models.PortSpec
		expectedK8sPorts []netguardv1beta1.PortSpec
		expectError      bool
	}{
		{
			name: "single ports",
			domainPorts: []models.PortSpec{
				{Destination: "80"},
				{Destination: "443"},
			},
			expectedK8sPorts: []netguardv1beta1.PortSpec{
				{Port: 80},
				{Port: 443},
			},
		},
		{
			name: "port ranges",
			domainPorts: []models.PortSpec{
				{Destination: "8000-8999"},
				{Destination: "9000-9999"},
			},
			expectedK8sPorts: []netguardv1beta1.PortSpec{
				{PortRange: &netguardv1beta1.PortRange{From: 8000, To: 8999}},
				{PortRange: &netguardv1beta1.PortRange{From: 9000, To: 9999}},
			},
		},
		{
			name: "mixed ports and ranges",
			domainPorts: []models.PortSpec{
				{Destination: "80"},
				{Destination: "8000-8999"},
				{Destination: "443"},
			},
			expectedK8sPorts: []netguardv1beta1.PortSpec{
				{Port: 80},
				{PortRange: &netguardv1beta1.PortRange{From: 8000, To: 8999}},
				{Port: 443},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			domain := &models.IEAgAgRule{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-rule",
						Namespace: "default",
					},
				},
				Transport: models.TCP,
				Traffic:   models.INGRESS,
				AddressGroupLocal: models.NewAddressGroupRef(
					"local-ag",
					models.WithNamespace("default"),
				),
				AddressGroup: models.NewAddressGroupRef(
					"remote-ag",
					models.WithNamespace("default"),
				),
				Ports:  tc.domainPorts,
				Action: models.ActionAccept,
			}

			k8sResult, err := converter.FromDomain(ctx, domain)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Len(t, k8sResult.Spec.Ports, len(tc.expectedK8sPorts))

				for i, expectedPort := range tc.expectedK8sPorts {
					actualPort := k8sResult.Spec.Ports[i]
					assert.Equal(t, expectedPort.Port, actualPort.Port)

					if expectedPort.PortRange != nil {
						require.NotNil(t, actualPort.PortRange)
						assert.Equal(t, expectedPort.PortRange.From, actualPort.PortRange.From)
						assert.Equal(t, expectedPort.PortRange.To, actualPort.PortRange.To)
					} else {
						assert.Nil(t, actualPort.PortRange)
					}
				}
			}
		})
	}
}

func TestIEAgAgRuleConverter_Trace_Conversion(t *testing.T) {
	ctx := context.Background()
	converter := NewIEAgAgRuleConverter()

	t.Run("ToDomain_WithTrace_Success", func(t *testing.T) {
		// Arrange
		k8sRule := &netguardv1beta1.IEAgAgRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rule-trace",
				Namespace: "default",
			},
			Spec: netguardv1beta1.IEAgAgRuleSpec{
				Transport: netguardv1beta1.ProtocolTCP,
				Traffic:   netguardv1beta1.INGRESS,
				AddressGroupLocal: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name: "local-ag",
					},
				},
				AddressGroup: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						Name: "remote-ag",
					},
				},
				Action: netguardv1beta1.ActionAccept,
				Trace:  true, // Test trace enabled
			},
		}

		// Act
		domainRule, err := converter.ToDomain(ctx, k8sRule)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, domainRule)
		assert.True(t, domainRule.Trace, "Trace field should be true when enabled in K8s spec")
	})

	t.Run("FromDomain_WithTrace_Success", func(t *testing.T) {
		// Arrange
		domainRule := &models.IEAgAgRule{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-rule-trace",
					Namespace: "default",
				},
			},
			Transport: models.TCP,
			Traffic:   models.INGRESS,
			AddressGroupLocal: models.NewAddressGroupRef(
				"local-ag",
				models.WithNamespace("default"),
			),
			AddressGroup: models.NewAddressGroupRef(
				"remote-ag",
				models.WithNamespace("default"),
			),
			Action: models.ActionAccept,
			Trace:  true, // Test trace enabled
		}

		// Act
		k8sRule, err := converter.FromDomain(ctx, domainRule)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, k8sRule)
		assert.True(t, k8sRule.Spec.Trace, "Trace field should be true when enabled in domain model")
	})
}
