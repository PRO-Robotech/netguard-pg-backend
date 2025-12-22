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

func TestHostConverter_ToDomain(t *testing.T) {
	ctx := context.Background()
	converter := &HostConverter{}

	testCases := []struct {
		name        string
		input       *netguardv1beta1.Host
		expected    *models.Host
		expectError bool
	}{
		{
			name: "valid host with minimal fields",
			input: &netguardv1beta1.Host{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "test-uuid-123",
				},
			},
			expected: &models.Host{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-host",
						Namespace: "default",
					},
				},
				UUID: "test-uuid-123",
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
			name: "valid host with full metadata and status",
			input: &netguardv1beta1.Host{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-host-full",
					Namespace:         "test-ns",
					UID:               types.UID("test-uid"),
					ResourceVersion:   "456",
					Generation:        3,
					CreationTimestamp: metav1.Time{Time: time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)},
					Labels: map[string]string{
						"app": "test",
					},
					Annotations: map[string]string{
						"note": "test annotation",
					},
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "full-uuid-456",
				},
				Status: netguardv1beta1.HostStatus{
					HostName:           "server1.example.com",
					AddressGroupName:   "ag-test",
					IsBound:            true,
					ObservedGeneration: 3,
					BindingRef: &netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "HostBinding",
							Name:       "test-binding",
						},
						Namespace: "test-ns",
					},
					AddressGroupRef: &netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "ag-test",
						},
						Namespace: "test-ns",
					},
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "Bound",
						},
					},
				},
			},
			expected: &models.Host{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-host-full",
						Namespace: "test-ns",
					},
				},
				UUID:             "full-uuid-456",
				HostName:         "server1.example.com",
				AddressGroupName: "ag-test",
				IsBound:          true,
				BindingRef: &netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "HostBinding",
						Name:       "test-binding",
					},
					Namespace: "test-ns",
				},
				AddressGroupRef: &netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "ag-test",
					},
					Namespace: "test-ns",
				},
				Meta: models.Meta{
					UID:                "test-uid",
					ResourceVersion:    "456",
					Generation:         3,
					CreationTS:         metav1.Time{Time: time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)},
					ObservedGeneration: 3,
					Labels: map[string]string{
						"app": "test",
					},
					Annotations: map[string]string{
						"note": "test annotation",
					},
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "Bound",
						},
					},
				},
			},
		},
		{
			name: "valid host with IPList",
			input: &netguardv1beta1.Host{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host-ip",
					Namespace: "default",
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "ip-uuid",
				},
				IPList: []netguardv1beta1.IPItem{
					{IP: "192.168.1.1"},
					{IP: "10.0.0.1"},
				},
			},
			expected: &models.Host{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-host-ip",
						Namespace: "default",
					},
				},
				UUID: "ip-uuid",
				IpList: []models.IPItem{
					{IP: "192.168.1.1"},
					{IP: "10.0.0.1"},
				},
				Meta: models.Meta{},
			},
		},
		{
			name: "valid host with MetaInfo",
			input: &netguardv1beta1.Host{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host-meta",
					Namespace: "default",
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "meta-uuid",
				},
				MetaInfo: &netguardv1beta1.HostMetaInfo{
					HostName:        "server1",
					Os:              "linux",
					Platform:        "ubuntu",
					PlatformFamily:  "debian",
					PlatformVersion: "22.04",
					KernelVersion:   "5.15.0",
				},
			},
			expected: &models.Host{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-host-meta",
						Namespace: "default",
					},
				},
				UUID: "meta-uuid",
				MetaInfo: &models.HostMetaInfo{
					HostName:        "server1",
					Os:              "linux",
					Platform:        "ubuntu",
					PlatformFamily:  "debian",
					PlatformVersion: "22.04",
					KernelVersion:   "5.15.0",
				},
				Meta: models.Meta{},
			},
		},
		{
			name: "valid host with IPList and MetaInfo",
			input: &netguardv1beta1.Host{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host-full-data",
					Namespace: "default",
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "full-data-uuid",
				},
				IPList: []netguardv1beta1.IPItem{
					{IP: "192.168.1.100"},
				},
				MetaInfo: &netguardv1beta1.HostMetaInfo{
					HostName: "server2",
					Os:       "windows",
				},
			},
			expected: &models.Host{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-host-full-data",
						Namespace: "default",
					},
				},
				UUID: "full-data-uuid",
				IpList: []models.IPItem{
					{IP: "192.168.1.100"},
				},
				MetaInfo: &models.HostMetaInfo{
					HostName: "server2",
					Os:       "windows",
				},
				Meta: models.Meta{},
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

func TestHostConverter_FromDomain(t *testing.T) {
	ctx := context.Background()
	converter := &HostConverter{}

	testCases := []struct {
		name        string
		input       *models.Host
		expected    *netguardv1beta1.Host
		expectError bool
	}{
		{
			name: "valid host with minimal fields",
			input: &models.Host{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-host",
						Namespace: "default",
					},
				},
				UUID: "test-uuid-123",
			},
			expected: &netguardv1beta1.Host{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Host",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "test-uuid-123",
				},
				Status: netguardv1beta1.HostStatus{
					ObservedGeneration: 0,
					Conditions:         nil,
				},
			},
		},
		{
			name: "valid host with full metadata",
			input: &models.Host{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-host-full",
						Namespace: "test-ns",
					},
				},
				UUID:             "full-uuid-456",
				HostName:         "server1.example.com",
				AddressGroupName: "ag-test",
				IsBound:          true,
				BindingRef: &netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "HostBinding",
						Name:       "test-binding",
					},
					Namespace: "test-ns",
				},
				AddressGroupRef: &netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "ag-test",
					},
					Namespace: "test-ns",
				},
				Meta: models.Meta{
					UID:                "test-uid",
					ResourceVersion:    "456",
					Generation:         3,
					CreationTS:         metav1.Time{Time: time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)},
					ObservedGeneration: 3,
					Labels: map[string]string{
						"app": "test",
					},
					Annotations: map[string]string{
						"note": "test annotation",
					},
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "Bound",
						},
					},
				},
			},
			expected: &netguardv1beta1.Host{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Host",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-host-full",
					Namespace:         "test-ns",
					UID:               types.UID("test-uid"),
					ResourceVersion:   "456",
					Generation:        3,
					CreationTimestamp: metav1.Time{Time: time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)},
					Labels: map[string]string{
						"app": "test",
					},
					Annotations: map[string]string{
						"note": "test annotation",
					},
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "full-uuid-456",
				},
				Status: netguardv1beta1.HostStatus{
					HostName:           "server1.example.com",
					AddressGroupName:   "ag-test",
					IsBound:            true,
					ObservedGeneration: 3,
					BindingRef: &netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "HostBinding",
							Name:       "test-binding",
						},
						Namespace: "test-ns",
					},
					AddressGroupRef: &netguardv1beta1.NamespacedObjectReference{
						ObjectReference: netguardv1beta1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1beta1",
							Kind:       "AddressGroup",
							Name:       "ag-test",
						},
						Namespace: "test-ns",
					},
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "Bound",
						},
					},
				},
			},
		},
		{
			name: "valid host with IPList",
			input: &models.Host{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-host-ip",
						Namespace: "default",
					},
				},
				UUID: "ip-uuid",
				IpList: []models.IPItem{
					{IP: "192.168.1.1"},
					{IP: "10.0.0.1"},
				},
			},
			expected: &netguardv1beta1.Host{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Host",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host-ip",
					Namespace: "default",
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "ip-uuid",
				},
				IPList: []netguardv1beta1.IPItem{
					{IP: "192.168.1.1"},
					{IP: "10.0.0.1"},
				},
				Status: netguardv1beta1.HostStatus{},
			},
		},
		{
			name: "valid host with MetaInfo",
			input: &models.Host{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-host-meta",
						Namespace: "default",
					},
				},
				UUID: "meta-uuid",
				MetaInfo: &models.HostMetaInfo{
					HostName:        "server1",
					Os:              "linux",
					Platform:        "ubuntu",
					PlatformFamily:  "debian",
					PlatformVersion: "22.04",
					KernelVersion:   "5.15.0",
				},
			},
			expected: &netguardv1beta1.Host{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Host",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host-meta",
					Namespace: "default",
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "meta-uuid",
				},
				MetaInfo: &netguardv1beta1.HostMetaInfo{
					HostName:        "server1",
					Os:              "linux",
					Platform:        "ubuntu",
					PlatformFamily:  "debian",
					PlatformVersion: "22.04",
					KernelVersion:   "5.15.0",
				},
				Status: netguardv1beta1.HostStatus{},
			},
		},
		{
			name: "valid host with IPList and MetaInfo",
			input: &models.Host{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-host-full-data",
						Namespace: "default",
					},
				},
				UUID: "full-data-uuid",
				IpList: []models.IPItem{
					{IP: "192.168.1.100"},
				},
				MetaInfo: &models.HostMetaInfo{
					HostName: "server2",
					Os:       "windows",
				},
			},
			expected: &netguardv1beta1.Host{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Host",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host-full-data",
					Namespace: "default",
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "full-data-uuid",
				},
				IPList: []netguardv1beta1.IPItem{
					{IP: "192.168.1.100"},
				},
				MetaInfo: &netguardv1beta1.HostMetaInfo{
					HostName: "server2",
					Os:       "windows",
				},
				Status: netguardv1beta1.HostStatus{},
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

func TestHostConverter_RoundTrip(t *testing.T) {
	ctx := context.Background()
	converter := &HostConverter{}

	testCases := []struct {
		name string
		k8s  *netguardv1beta1.Host
	}{
		{
			name: "basic host",
			k8s: &netguardv1beta1.Host{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "test-uuid",
				},
			},
		},
		{
			name: "host with metadata",
			k8s: &netguardv1beta1.Host{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-host-meta",
					Namespace:         "test-ns",
					UID:               types.UID("test-uid"),
					ResourceVersion:   "789",
					Generation:        5,
					CreationTimestamp: metav1.Time{Time: time.Date(2023, 7, 20, 15, 30, 0, 0, time.UTC)},
					Labels: map[string]string{
						"env": "prod",
					},
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "meta-uuid",
				},
				Status: netguardv1beta1.HostStatus{
					HostName:           "server.example.com",
					AddressGroupName:   "ag-prod",
					IsBound:            true,
					ObservedGeneration: 5,
				},
			},
		},
		{
			name: "host with IPList",
			k8s: &netguardv1beta1.Host{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host-ip",
					Namespace: "default",
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "ip-uuid",
				},
				IPList: []netguardv1beta1.IPItem{
					{IP: "192.168.1.1"},
					{IP: "10.0.0.1"},
					{IP: "172.16.0.1"},
				},
			},
		},
		{
			name: "host with MetaInfo",
			k8s: &netguardv1beta1.Host{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host-metainfo",
					Namespace: "default",
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "metainfo-uuid",
				},
				MetaInfo: &netguardv1beta1.HostMetaInfo{
					HostName:        "server1",
					Os:              "linux",
					Platform:        "ubuntu",
					PlatformFamily:  "debian",
					PlatformVersion: "22.04",
					KernelVersion:   "5.15.0-generic",
				},
			},
		},
		{
			name: "host with IPList and MetaInfo",
			k8s: &netguardv1beta1.Host{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host-complete",
					Namespace: "production",
				},
				Spec: netguardv1beta1.HostSpec{
					UUID: "complete-uuid",
				},
				IPList: []netguardv1beta1.IPItem{
					{IP: "10.10.10.10"},
				},
				MetaInfo: &netguardv1beta1.HostMetaInfo{
					HostName: "prod-server",
					Os:       "centos",
				},
				Status: netguardv1beta1.HostStatus{
					IsBound: true,
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
			assert.Equal(t, tc.k8s.ObjectMeta.UID, k8s.ObjectMeta.UID)
			assert.Equal(t, tc.k8s.ObjectMeta.ResourceVersion, k8s.ObjectMeta.ResourceVersion)
			assert.Equal(t, tc.k8s.ObjectMeta.Generation, k8s.ObjectMeta.Generation)
			assert.Equal(t, tc.k8s.ObjectMeta.CreationTimestamp, k8s.ObjectMeta.CreationTimestamp)
			assert.Equal(t, tc.k8s.ObjectMeta.Labels, k8s.ObjectMeta.Labels)
			assert.Equal(t, tc.k8s.Spec, k8s.Spec)
			assert.Equal(t, tc.k8s.IPList, k8s.IPList)
			assert.Equal(t, tc.k8s.MetaInfo, k8s.MetaInfo)
			assert.Equal(t, tc.k8s.Status.HostName, k8s.Status.HostName)
			assert.Equal(t, tc.k8s.Status.AddressGroupName, k8s.Status.AddressGroupName)
			assert.Equal(t, tc.k8s.Status.IsBound, k8s.Status.IsBound)

			// Verify TypeMeta is set correctly
			assert.Equal(t, "netguard.sgroups.io/v1beta1", k8s.TypeMeta.APIVersion)
			assert.Equal(t, "Host", k8s.TypeMeta.Kind)
		})
	}
}

func TestHostConverter_ToList(t *testing.T) {
	ctx := context.Background()
	converter := &HostConverter{}

	testCases := []struct {
		name        string
		input       []*models.Host
		expectError bool
		checkFunc   func(t *testing.T, result *netguardv1beta1.HostList)
	}{
		{
			name:  "empty list",
			input: []*models.Host{},
			checkFunc: func(t *testing.T, result *netguardv1beta1.HostList) {
				assert.Equal(t, "netguard.sgroups.io/v1beta1", result.TypeMeta.APIVersion)
				assert.Equal(t, "HostList", result.TypeMeta.Kind)
				assert.Equal(t, 0, len(result.Items))
			},
		},
		{
			name: "single item",
			input: []*models.Host{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "test-host",
							Namespace: "default",
						},
					},
					UUID: "single-uuid",
				},
			},
			checkFunc: func(t *testing.T, result *netguardv1beta1.HostList) {
				assert.Equal(t, "netguard.sgroups.io/v1beta1", result.TypeMeta.APIVersion)
				assert.Equal(t, "HostList", result.TypeMeta.Kind)
				assert.Equal(t, 1, len(result.Items))
				assert.Equal(t, "test-host", result.Items[0].Name)
				assert.Equal(t, "default", result.Items[0].Namespace)
				assert.Equal(t, "single-uuid", result.Items[0].Spec.UUID)
			},
		},
		{
			name: "multiple items with MetaInfo",
			input: []*models.Host{
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "host-1",
							Namespace: "default",
						},
					},
					UUID: "uuid-1",
					IpList: []models.IPItem{
						{IP: "192.168.1.1"},
					},
				},
				{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      "host-2",
							Namespace: "default",
						},
					},
					UUID: "uuid-2",
					MetaInfo: &models.HostMetaInfo{
						HostName: "server2",
						Os:       "linux",
					},
				},
			},
			checkFunc: func(t *testing.T, result *netguardv1beta1.HostList) {
				assert.Equal(t, "netguard.sgroups.io/v1beta1", result.TypeMeta.APIVersion)
				assert.Equal(t, "HostList", result.TypeMeta.Kind)
				assert.Equal(t, 2, len(result.Items))

				// Check first item
				assert.Equal(t, "host-1", result.Items[0].Name)
				assert.Equal(t, "uuid-1", result.Items[0].Spec.UUID)
				assert.Len(t, result.Items[0].IPList, 1)
				assert.Equal(t, "192.168.1.1", result.Items[0].IPList[0].IP)
				assert.Nil(t, result.Items[0].MetaInfo)

				// Check second item
				assert.Equal(t, "host-2", result.Items[1].Name)
				assert.Equal(t, "uuid-2", result.Items[1].Spec.UUID)
				assert.Nil(t, result.Items[1].IPList)
				assert.NotNil(t, result.Items[1].MetaInfo)
				assert.Equal(t, "server2", result.Items[1].MetaInfo.HostName)
				assert.Equal(t, "linux", result.Items[1].MetaInfo.Os)
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

				list, ok := result.(*netguardv1beta1.HostList)
				require.True(t, ok, "Result should be *netguardv1beta1.HostList")

				tc.checkFunc(t, list)
			}
		})
	}
}

func TestHostConverter_MetaInfoConversion(t *testing.T) {
	ctx := context.Background()
	converter := &HostConverter{}

	t.Run("MetaInfo fields are preserved in round-trip", func(t *testing.T) {
		k8sHost := &netguardv1beta1.Host{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "metainfo-test",
				Namespace: "default",
			},
			Spec: netguardv1beta1.HostSpec{
				UUID: "test-uuid",
			},
			MetaInfo: &netguardv1beta1.HostMetaInfo{
				HostName:        "prod-server-01",
				Os:              "linux",
				Platform:        "ubuntu",
				PlatformFamily:  "debian",
				PlatformVersion: "22.04.3 LTS",
				KernelVersion:   "5.15.0-91-generic",
			},
		}

		// K8s -> Domain
		domain, err := converter.ToDomain(ctx, k8sHost)
		require.NoError(t, err)
		require.NotNil(t, domain.MetaInfo)

		assert.Equal(t, "prod-server-01", domain.MetaInfo.HostName)
		assert.Equal(t, "linux", domain.MetaInfo.Os)
		assert.Equal(t, "ubuntu", domain.MetaInfo.Platform)
		assert.Equal(t, "debian", domain.MetaInfo.PlatformFamily)
		assert.Equal(t, "22.04.3 LTS", domain.MetaInfo.PlatformVersion)
		assert.Equal(t, "5.15.0-91-generic", domain.MetaInfo.KernelVersion)

		// Domain -> K8s
		k8sResult, err := converter.FromDomain(ctx, domain)
		require.NoError(t, err)
		require.NotNil(t, k8sResult.MetaInfo)

		assert.Equal(t, k8sHost.MetaInfo, k8sResult.MetaInfo)
	})

	t.Run("nil MetaInfo is handled correctly", func(t *testing.T) {
		k8sHost := &netguardv1beta1.Host{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "no-metainfo-test",
				Namespace: "default",
			},
			Spec: netguardv1beta1.HostSpec{
				UUID: "test-uuid",
			},
			MetaInfo: nil,
		}

		domain, err := converter.ToDomain(ctx, k8sHost)
		require.NoError(t, err)
		assert.Nil(t, domain.MetaInfo)

		k8sResult, err := converter.FromDomain(ctx, domain)
		require.NoError(t, err)
		assert.Nil(t, k8sResult.MetaInfo)
	})

	t.Run("partial MetaInfo is handled correctly", func(t *testing.T) {
		k8sHost := &netguardv1beta1.Host{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "partial-metainfo-test",
				Namespace: "default",
			},
			Spec: netguardv1beta1.HostSpec{
				UUID: "test-uuid",
			},
			MetaInfo: &netguardv1beta1.HostMetaInfo{
				HostName: "server-only-hostname",
				// Other fields are empty
			},
		}

		domain, err := converter.ToDomain(ctx, k8sHost)
		require.NoError(t, err)
		require.NotNil(t, domain.MetaInfo)
		assert.Equal(t, "server-only-hostname", domain.MetaInfo.HostName)
		assert.Empty(t, domain.MetaInfo.Os)
		assert.Empty(t, domain.MetaInfo.Platform)

		k8sResult, err := converter.FromDomain(ctx, domain)
		require.NoError(t, err)
		require.NotNil(t, k8sResult.MetaInfo)
		assert.Equal(t, "server-only-hostname", k8sResult.MetaInfo.HostName)
		assert.Empty(t, k8sResult.MetaInfo.Os)
	})
}

func TestHostConverter_IPListAndMetaInfoCombination(t *testing.T) {
	ctx := context.Background()
	converter := &HostConverter{}

	t.Run("IPList and MetaInfo are both preserved", func(t *testing.T) {
		k8sHost := &netguardv1beta1.Host{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "combined-test",
				Namespace: "default",
			},
			Spec: netguardv1beta1.HostSpec{
				UUID: "combined-uuid",
			},
			IPList: []netguardv1beta1.IPItem{
				{IP: "10.0.0.1"},
				{IP: "10.0.0.2"},
			},
			MetaInfo: &netguardv1beta1.HostMetaInfo{
				HostName: "combined-server",
				Os:       "linux",
			},
		}

		// K8s -> Domain
		domain, err := converter.ToDomain(ctx, k8sHost)
		require.NoError(t, err)

		assert.Len(t, domain.IpList, 2)
		assert.Equal(t, "10.0.0.1", domain.IpList[0].IP)
		assert.Equal(t, "10.0.0.2", domain.IpList[1].IP)

		require.NotNil(t, domain.MetaInfo)
		assert.Equal(t, "combined-server", domain.MetaInfo.HostName)
		assert.Equal(t, "linux", domain.MetaInfo.Os)

		// Domain -> K8s
		k8sResult, err := converter.FromDomain(ctx, domain)
		require.NoError(t, err)

		assert.Len(t, k8sResult.IPList, 2)
		assert.Equal(t, k8sHost.IPList, k8sResult.IPList)

		require.NotNil(t, k8sResult.MetaInfo)
		assert.Equal(t, k8sHost.MetaInfo, k8sResult.MetaInfo)
	})
}
