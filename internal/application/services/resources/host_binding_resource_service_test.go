package resources

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/application/services/resources/testutil"
	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestHostBindingResourceService_CreateHostBindingUpdatesParents(t *testing.T) {
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()

	hostService := NewHostResourceService(mockRegistry, mockSyncManager, nil)
	validationService := NewValidationService(mockRegistry, mockSyncManager)
	agConditionManager := testutil.NewMockConditionManager()
	addressGroupService := NewAddressGroupResourceService(
		mockRegistry,
		mockSyncManager,
		agConditionManager,
		validationService,
		hostService,
	)
	hostBindingService := NewHostBindingResourceService(
		mockRegistry,
		hostService,
		addressGroupService,
		mockSyncManager,
		nil,
	)

	host := models.NewHost("test-host", "test-namespace", "host-uuid")
	host.Meta.TouchOnCreate()
	host.Meta.SetCondition(metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "UnitTest",
		Message:            "Ready for binding",
		LastTransitionTime: metav1.Now(),
	})

	addressGroup := testutil.CreateTestAddressGroup("test-address-group", "test-namespace")
	addressGroup.Meta.TouchOnCreate()
	addressGroup.Meta.SetCondition(metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "UnitTest",
		Message:            "Ready for binding",
		LastTransitionTime: metav1.Now(),
	})

	mockRegistry.SetupTestData(map[string]interface{}{
		"host_test-namespace/test-host":                  host,
		"addressgroup_test-namespace/test-address-group": &addressGroup,
	})

	agBefore, err := addressGroupService.GetAddressGroupByID(
		context.Background(),
		models.ResourceIdentifier{Name: "test-address-group", Namespace: "test-namespace"},
	)
	require.NoError(t, err)
	prevRV := agBefore.Meta.ResourceVersion

	binding := &models.HostBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-binding",
				Namespace: "test-namespace",
			},
		},
		HostRef: netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Host",
				Name:       "test-host",
			},
			Namespace: "test-namespace",
		},
		AddressGroupRef: netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "test-address-group",
			},
			Namespace: "test-namespace",
		},
	}
	binding.Meta.TouchOnCreate()

	err = hostBindingService.CreateHostBinding(context.Background(), binding)
	require.NoError(t, err)

	updatedHost, err := hostService.GetHost(context.Background(), models.ResourceIdentifier{
		Name:      "test-host",
		Namespace: "test-namespace",
	})
	require.NoError(t, err)
	require.True(t, updatedHost.IsBound)
	require.NotNil(t, updatedHost.BindingRef)
	require.Equal(t, "test-binding", updatedHost.BindingRef.Name)
	require.NotNil(t, updatedHost.AddressGroupRef)
	require.Equal(t, "test-address-group", updatedHost.AddressGroupRef.Name)

	updatedAG, err := addressGroupService.GetAddressGroupByID(
		context.Background(),
		models.ResourceIdentifier{Name: "test-address-group", Namespace: "test-namespace"},
	)
	require.NoError(t, err)
	require.NotNil(t, updatedAG)
	if prevRV == "" {
		require.NotEmpty(t, updatedAG.Meta.ResourceVersion)
	} else {
		require.NotEqual(t, prevRV, updatedAG.Meta.ResourceVersion)
	}
}
