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

func TestServiceConverter_ToDomain(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	// Test cases
	testCases := []struct {
		name        string
		input       *netguardv1beta1.Service
		expected    *models.Service
		expectError bool
	}{
		{
			name: "valid service with minimal fields",
			input: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Test service",
				},
			},
			expected: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-service",
						Namespace: "default",
					},
				},
				Description: "Test service",
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
			name: "valid service with ingress ports",
			input: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-service",
					Namespace: "webapp",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Web application service",
					IngressPorts: []netguardv1beta1.IngressPort{
						{
							Protocol:    netguardv1beta1.ProtocolTCP,
							Port:        "80",
							Description: "HTTP port",
						},
						{
							Protocol:    netguardv1beta1.ProtocolTCP,
							Port:        "443",
							Description: "HTTPS port",
						},
						{
							Protocol:    netguardv1beta1.ProtocolUDP,
							Port:        "53",
							Description: "DNS port",
						},
					},
				},
			},
			expected: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "web-service",
						Namespace: "webapp",
					},
				},
				Description: "Web application service",
				IngressPorts: []models.IngressPort{
					{
						Protocol:    models.TCP,
						Port:        "80",
						Description: "HTTP port",
					},
					{
						Protocol:    models.TCP,
						Port:        "443",
						Description: "HTTPS port",
					},
					{
						Protocol:    models.UDP,
						Port:        "53",
						Description: "DNS port",
					},
				},
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
			name: "valid service with full metadata",
			input: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-service-full",
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
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Full featured test service",
					IngressPorts: []netguardv1beta1.IngressPort{
						{
							Protocol:    netguardv1beta1.ProtocolTCP,
							Port:        "8080",
							Description: "Application port",
						},
					},
				},
				Status: netguardv1beta1.ServiceStatus{
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
			expected: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-service-full",
						Namespace: "test-ns",
					},
				},
				Description: "Full featured test service",
				IngressPorts: []models.IngressPort{
					{
						Protocol:    models.TCP,
						Port:        "8080",
						Description: "Application port",
					},
				},
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

func TestServiceConverter_FromDomain(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	// Test cases
	testCases := []struct {
		name        string
		input       *models.Service
		expected    *netguardv1beta1.Service
		expectError bool
	}{
		{
			name: "valid service with minimal fields",
			input: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-service",
						Namespace: "default",
					},
				},
				Description: "Test service",
			},
			expected: &netguardv1beta1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Test service",
				},
				Status: netguardv1beta1.ServiceStatus{
					ObservedGeneration: 0,
					Conditions:         nil,
				},
			},
		},
		{
			name: "valid service with ingress ports",
			input: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "web-service",
						Namespace: "webapp",
					},
				},
				Description: "Web application service",
				IngressPorts: []models.IngressPort{
					{
						Protocol:    models.TCP,
						Port:        "80",
						Description: "HTTP port",
					},
					{
						Protocol:    models.UDP,
						Port:        "53",
						Description: "DNS port",
					},
				},
			},
			expected: &netguardv1beta1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-service",
					Namespace: "webapp",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Web application service",
					IngressPorts: []netguardv1beta1.IngressPort{
						{
							Protocol:    netguardv1beta1.ProtocolTCP,
							Port:        "80",
							Description: "HTTP port",
						},
						{
							Protocol:    netguardv1beta1.ProtocolUDP,
							Port:        "53",
							Description: "DNS port",
						},
					},
				},
				Status: netguardv1beta1.ServiceStatus{
					ObservedGeneration: 0,
					Conditions:         nil,
				},
			},
		},
		{
			name: "valid service with full metadata",
			input: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-service-full",
						Namespace: "test-ns",
					},
				},
				Description: "Full featured test service",
				IngressPorts: []models.IngressPort{
					{
						Protocol:    models.TCP,
						Port:        "8080",
						Description: "Application port",
					},
				},
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
			expected: &netguardv1beta1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-service-full",
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
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Full featured test service",
					IngressPorts: []netguardv1beta1.IngressPort{
						{
							Protocol:    netguardv1beta1.ProtocolTCP,
							Port:        "8080",
							Description: "Application port",
						},
					},
				},
				Status: netguardv1beta1.ServiceStatus{
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
		},
		{
			name:        "nil input",
			input:       nil,
			expected:    nil,
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
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestServiceConverter_RoundTrip(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	// Test cases for round-trip conversion
	testCases := []struct {
		name string
		k8s  *netguardv1beta1.Service
	}{
		{
			name: "basic service",
			k8s: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Test service",
				},
			},
		},
		{
			name: "service with ports",
			k8s: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-service",
					Namespace: "webapp",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Web service",
					IngressPorts: []netguardv1beta1.IngressPort{
						{
							Protocol:    netguardv1beta1.ProtocolTCP,
							Port:        "80",
							Description: "HTTP",
						},
						{
							Protocol:    netguardv1beta1.ProtocolUDP,
							Port:        "53",
							Description: "DNS",
						},
					},
				},
			},
		},
		{
			name: "service with metadata",
			k8s: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-service-meta",
					Namespace:         "test-ns",
					UID:               types.UID("test-uid"),
					ResourceVersion:   "456",
					Generation:        3,
					CreationTimestamp: metav1.Time{Time: time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)},
					Labels: map[string]string{
						"component": "api",
					},
					Annotations: map[string]string{
						"description": "test service",
					},
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Test service with metadata",
					IngressPorts: []netguardv1beta1.IngressPort{
						{
							Protocol:    netguardv1beta1.ProtocolTCP,
							Port:        "8080",
							Description: "API port",
						},
					},
				},
				Status: netguardv1beta1.ServiceStatus{
					ObservedGeneration: 3,
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "Synchronized",
						},
					},
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

			// Compare essential fields (ignoring TypeMeta which is added by FromDomain)
			assert.Equal(t, tc.k8s.ObjectMeta.Name, k8s.ObjectMeta.Name)
			assert.Equal(t, tc.k8s.ObjectMeta.Namespace, k8s.ObjectMeta.Namespace)
			assert.Equal(t, tc.k8s.ObjectMeta.UID, k8s.ObjectMeta.UID)
			assert.Equal(t, tc.k8s.ObjectMeta.ResourceVersion, k8s.ObjectMeta.ResourceVersion)
			assert.Equal(t, tc.k8s.ObjectMeta.Generation, k8s.ObjectMeta.Generation)
			assert.Equal(t, tc.k8s.ObjectMeta.CreationTimestamp, k8s.ObjectMeta.CreationTimestamp)
			assert.Equal(t, tc.k8s.ObjectMeta.Labels, k8s.ObjectMeta.Labels)
			assert.Equal(t, tc.k8s.ObjectMeta.Annotations, k8s.ObjectMeta.Annotations)
			assert.Equal(t, tc.k8s.Spec, k8s.Spec)
			assert.Equal(t, tc.k8s.Status, k8s.Status)

			// Verify TypeMeta is set correctly
			assert.Equal(t, "netguard.sgroups.io/v1beta1", k8s.TypeMeta.APIVersion)
			assert.Equal(t, "Service", k8s.TypeMeta.Kind)
		})
	}
}

func TestServiceConverter_ToList(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	// Test cases
	testCases := []struct {
		name        string
		input       []*models.Service
		expectError bool
		checkFunc   func(t *testing.T, result *netguardv1beta1.ServiceList)
	}{
		{
			name:  "empty list",
			input: []*models.Service{},
			checkFunc: func(t *testing.T, result *netguardv1beta1.ServiceList) {
				assert.Equal(t, "netguard.sgroups.io/v1beta1", result.TypeMeta.APIVersion)
				assert.Equal(t, "ServiceList", result.TypeMeta.Kind)
				assert.Equal(t, 0, len(result.Items))
			},
		},
		{
			name: "single item",
			input: []*models.Service{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "test-service",
							Namespace: "default",
						},
					},
					Description: "Test service",
				},
			},
			checkFunc: func(t *testing.T, result *netguardv1beta1.ServiceList) {
				assert.Equal(t, "netguard.sgroups.io/v1beta1", result.TypeMeta.APIVersion)
				assert.Equal(t, "ServiceList", result.TypeMeta.Kind)
				assert.Equal(t, 1, len(result.Items))
				assert.Equal(t, "test-service", result.Items[0].Name)
				assert.Equal(t, "default", result.Items[0].Namespace)
				assert.Equal(t, "Test service", result.Items[0].Spec.Description)
			},
		},
		{
			name: "multiple items with ports",
			input: []*models.Service{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "web-service",
							Namespace: "default",
						},
					},
					Description: "Web service",
					IngressPorts: []models.IngressPort{
						{
							Protocol:    models.TCP,
							Port:        "80",
							Description: "HTTP",
						},
					},
				},
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "api-service",
							Namespace: "default",
						},
					},
					Description: "API service",
					IngressPorts: []models.IngressPort{
						{
							Protocol:    models.TCP,
							Port:        "8080",
							Description: "API",
						},
						{
							Protocol:    models.UDP,
							Port:        "53",
							Description: "DNS",
						},
					},
				},
			},
			checkFunc: func(t *testing.T, result *netguardv1beta1.ServiceList) {
				assert.Equal(t, "netguard.sgroups.io/v1beta1", result.TypeMeta.APIVersion)
				assert.Equal(t, "ServiceList", result.TypeMeta.Kind)
				assert.Equal(t, 2, len(result.Items))

				// Check first item
				assert.Equal(t, "web-service", result.Items[0].Name)
				assert.Equal(t, "Web service", result.Items[0].Spec.Description)
				assert.Equal(t, 1, len(result.Items[0].Spec.IngressPorts))
				assert.Equal(t, netguardv1beta1.ProtocolTCP, result.Items[0].Spec.IngressPorts[0].Protocol)
				assert.Equal(t, "80", result.Items[0].Spec.IngressPorts[0].Port)

				// Check second item
				assert.Equal(t, "api-service", result.Items[1].Name)
				assert.Equal(t, "API service", result.Items[1].Spec.Description)
				assert.Equal(t, 2, len(result.Items[1].Spec.IngressPorts))
				assert.Equal(t, netguardv1beta1.ProtocolTCP, result.Items[1].Spec.IngressPorts[0].Protocol)
				assert.Equal(t, "8080", result.Items[1].Spec.IngressPorts[0].Port)
				assert.Equal(t, netguardv1beta1.ProtocolUDP, result.Items[1].Spec.IngressPorts[1].Protocol)
				assert.Equal(t, "53", result.Items[1].Spec.IngressPorts[1].Port)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.ToList(ctx, tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)

				list, ok := result.(*netguardv1beta1.ServiceList)
				require.True(t, ok, "Result should be *netguardv1beta1.ServiceList")

				tc.checkFunc(t, list)
			}
		})
	}
}

func TestServiceConverter_ProtocolConversion(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	// Test protocol conversion both ways
	testCases := []struct {
		name           string
		k8sProtocol    netguardv1beta1.TransportProtocol
		domainProtocol models.TransportProtocol
	}{
		{
			name:           "TCP protocol",
			k8sProtocol:    netguardv1beta1.ProtocolTCP,
			domainProtocol: models.TCP,
		},
		{
			name:           "UDP protocol",
			k8sProtocol:    netguardv1beta1.ProtocolUDP,
			domainProtocol: models.UDP,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// K8s -> Domain
			k8sObj := &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Test service",
					IngressPorts: []netguardv1beta1.IngressPort{
						{
							Protocol:    tc.k8sProtocol,
							Port:        "8080",
							Description: "Test port",
						},
					},
				},
			}

			domain, err := converter.ToDomain(ctx, k8sObj)
			require.NoError(t, err)
			require.Len(t, domain.IngressPorts, 1)
			assert.Equal(t, tc.domainProtocol, domain.IngressPorts[0].Protocol)

			// Domain -> K8s
			k8sResult, err := converter.FromDomain(ctx, domain)
			require.NoError(t, err)
			require.Len(t, k8sResult.Spec.IngressPorts, 1)
			assert.Equal(t, tc.k8sProtocol, k8sResult.Spec.IngressPorts[0].Protocol)
		})
	}
}
