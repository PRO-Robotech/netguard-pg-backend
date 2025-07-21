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

func TestAddressGroupConverter_ToDomain(t *testing.T) {
	ctx := context.Background()
	converter := NewAddressGroupConverter()

	// Test cases
	testCases := []struct {
		name        string
		input       *netguardv1beta1.AddressGroup
		expected    *models.AddressGroup
		expectError bool
	}{
		{
			name: "valid addressgroup with minimal fields",
			input: &netguardv1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: netguardv1beta1.AddressGroupSpec{
					DefaultAction: netguardv1beta1.ActionAccept,
				},
			},
			expected: &models.AddressGroup{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-ag",
						Namespace: "default",
					},
				},
				DefaultAction: models.ActionAccept,
				Logs:          false,
				Trace:         false,
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
			name: "valid addressgroup with full metadata",
			input: &netguardv1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-ag-full",
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
				Spec: netguardv1beta1.AddressGroupSpec{
					DefaultAction: netguardv1beta1.ActionDrop,
					Logs:          true,
					Trace:         true,
				},
				Status: netguardv1beta1.AddressGroupStatus{
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
			expected: &models.AddressGroup{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-ag-full",
						Namespace: "test-ns",
					},
				},
				DefaultAction: models.ActionDrop,
				Logs:          true,
				Trace:         true,
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

func TestAddressGroupConverter_FromDomain(t *testing.T) {
	ctx := context.Background()
	converter := NewAddressGroupConverter()

	// Test cases
	testCases := []struct {
		name        string
		input       *models.AddressGroup
		expected    *netguardv1beta1.AddressGroup
		expectError bool
	}{
		{
			name: "valid addressgroup with minimal fields",
			input: &models.AddressGroup{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-ag",
						Namespace: "default",
					},
				},
				DefaultAction: models.ActionAccept,
			},
			expected: &netguardv1beta1.AddressGroup{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: netguardv1beta1.AddressGroupSpec{
					DefaultAction: netguardv1beta1.ActionAccept,
					Logs:          false,
					Trace:         false,
				},
				Status: netguardv1beta1.AddressGroupStatus{
					ObservedGeneration: 0,
					Conditions:         nil,
				},
			},
		},
		{
			name: "valid addressgroup with full metadata",
			input: &models.AddressGroup{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-ag-full",
						Namespace: "test-ns",
					},
				},
				DefaultAction: models.ActionDrop,
				Logs:          true,
				Trace:         true,
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
			expected: &netguardv1beta1.AddressGroup{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-ag-full",
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
				Spec: netguardv1beta1.AddressGroupSpec{
					DefaultAction: netguardv1beta1.ActionDrop,
					Logs:          true,
					Trace:         true,
				},
				Status: netguardv1beta1.AddressGroupStatus{
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

func TestAddressGroupConverter_RoundTrip(t *testing.T) {
	ctx := context.Background()
	converter := NewAddressGroupConverter()

	// Test cases for round-trip conversion
	testCases := []struct {
		name string
		k8s  *netguardv1beta1.AddressGroup
	}{
		{
			name: "basic addressgroup",
			k8s: &netguardv1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: netguardv1beta1.AddressGroupSpec{
					DefaultAction: netguardv1beta1.ActionAccept,
				},
			},
		},
		{
			name: "addressgroup with metadata",
			k8s: &netguardv1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-ag-meta",
					Namespace:         "test-ns",
					UID:               types.UID("test-uid"),
					ResourceVersion:   "456",
					Generation:        3,
					CreationTimestamp: metav1.Time{Time: time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)},
					Labels: map[string]string{
						"component": "network",
					},
					Annotations: map[string]string{
						"description": "test address group",
					},
				},
				Spec: netguardv1beta1.AddressGroupSpec{
					DefaultAction: netguardv1beta1.ActionDrop,
					Logs:          true,
					Trace:         true,
				},
				Status: netguardv1beta1.AddressGroupStatus{
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
			assert.Equal(t, "AddressGroup", k8s.TypeMeta.Kind)
		})
	}
}

func TestAddressGroupConverter_ToList(t *testing.T) {
	ctx := context.Background()
	converter := NewAddressGroupConverter()

	// Test cases
	testCases := []struct {
		name        string
		input       []*models.AddressGroup
		expectError bool
		checkFunc   func(t *testing.T, result *netguardv1beta1.AddressGroupList)
	}{
		{
			name:  "empty list",
			input: []*models.AddressGroup{},
			checkFunc: func(t *testing.T, result *netguardv1beta1.AddressGroupList) {
				assert.Equal(t, "netguard.sgroups.io/v1beta1", result.TypeMeta.APIVersion)
				assert.Equal(t, "AddressGroupList", result.TypeMeta.Kind)
				assert.Equal(t, 0, len(result.Items))
			},
		},
		{
			name: "single item",
			input: []*models.AddressGroup{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "test-ag",
							Namespace: "default",
						},
					},
					DefaultAction: models.ActionAccept,
				},
			},
			checkFunc: func(t *testing.T, result *netguardv1beta1.AddressGroupList) {
				assert.Equal(t, "netguard.sgroups.io/v1beta1", result.TypeMeta.APIVersion)
				assert.Equal(t, "AddressGroupList", result.TypeMeta.Kind)
				assert.Equal(t, 1, len(result.Items))
				assert.Equal(t, "test-ag", result.Items[0].Name)
				assert.Equal(t, "default", result.Items[0].Namespace)
				assert.Equal(t, netguardv1beta1.ActionAccept, result.Items[0].Spec.DefaultAction)
			},
		},
		{
			name: "multiple items",
			input: []*models.AddressGroup{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "test-ag-1",
							Namespace: "default",
						},
					},
					DefaultAction: models.ActionAccept,
				},
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "test-ag-2",
							Namespace: "default",
						},
					},
					DefaultAction: models.ActionDrop,
					Logs:          true,
				},
			},
			checkFunc: func(t *testing.T, result *netguardv1beta1.AddressGroupList) {
				assert.Equal(t, "netguard.sgroups.io/v1beta1", result.TypeMeta.APIVersion)
				assert.Equal(t, "AddressGroupList", result.TypeMeta.Kind)
				assert.Equal(t, 2, len(result.Items))

				// Check first item
				assert.Equal(t, "test-ag-1", result.Items[0].Name)
				assert.Equal(t, netguardv1beta1.ActionAccept, result.Items[0].Spec.DefaultAction)

				// Check second item
				assert.Equal(t, "test-ag-2", result.Items[1].Name)
				assert.Equal(t, netguardv1beta1.ActionDrop, result.Items[1].Spec.DefaultAction)
				assert.Equal(t, true, result.Items[1].Spec.Logs)
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

				list, ok := result.(*netguardv1beta1.AddressGroupList)
				require.True(t, ok, "Result should be *netguardv1beta1.AddressGroupList")

				tc.checkFunc(t, list)
			}
		})
	}
}

func TestAddressGroupConverter_EnumConversion(t *testing.T) {
	ctx := context.Background()
	converter := NewAddressGroupConverter()

	// Test enum conversion both ways
	testCases := []struct {
		name         string
		k8sAction    netguardv1beta1.RuleAction
		domainAction models.RuleAction
	}{
		{
			name:         "ACCEPT action",
			k8sAction:    netguardv1beta1.ActionAccept,
			domainAction: models.ActionAccept,
		},
		{
			name:         "DROP action",
			k8sAction:    netguardv1beta1.ActionDrop,
			domainAction: models.ActionDrop,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// K8s -> Domain
			k8sObj := &netguardv1beta1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ag",
					Namespace: "default",
				},
				Spec: netguardv1beta1.AddressGroupSpec{
					DefaultAction: tc.k8sAction,
				},
			}

			domain, err := converter.ToDomain(ctx, k8sObj)
			require.NoError(t, err)
			assert.Equal(t, tc.domainAction, domain.DefaultAction)

			// Domain -> K8s
			k8sResult, err := converter.FromDomain(ctx, domain)
			require.NoError(t, err)
			assert.Equal(t, tc.k8sAction, k8sResult.Spec.DefaultAction)
		})
	}
}
