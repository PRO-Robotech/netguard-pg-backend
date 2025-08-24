package base

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/watch"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestBaseStorage_PreserveManagedFields(t *testing.T) {
	// Create test storage
	storage := createTestServiceStorage()

	t.Run("PreserveManagedFields_SimpleCase", func(t *testing.T) {
		// Create source object with managedFields
		now := metav1.NewTime(time.Now())
		source := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:    "kubectl-apply",
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:spec":{"f:description":{}}}`),
						},
					},
				},
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Test service",
			},
		}

		// Create destination object without managedFields
		dest := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
			Spec: netguardv1beta1.ServiceSpec{
				Description: "Updated test service",
			},
		}

		// Preserve managedFields
		err := storage.preserveManagedFields(source, dest)
		require.NoError(t, err)

		// Verify managedFields were copied
		assert.NotNil(t, dest.ManagedFields)
		assert.Len(t, dest.ManagedFields, 1)
		assert.Equal(t, "kubectl-apply", dest.ManagedFields[0].Manager)
		assert.Equal(t, metav1.ManagedFieldsOperationApply, dest.ManagedFields[0].Operation)
		assert.Equal(t, `{"f:spec":{"f:description":{}}}`, string(dest.ManagedFields[0].FieldsV1.Raw))
	})

	t.Run("PreserveManagedFields_MergeCase", func(t *testing.T) {
		// Create source object with managedFields
		now := metav1.NewTime(time.Now())
		source := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:    "kubectl-apply",
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:spec":{"f:description":{}}}`),
						},
					},
					{
						Manager:     "netguard-controller",
						Operation:   metav1.ManagedFieldsOperationUpdate,
						APIVersion:  "netguard.sgroups.io/v1beta1",
						Time:        &now,
						FieldsType:  "FieldsV1",
						Subresource: "status",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:status":{"f:conditions":{}}}`),
						},
					},
				},
			},
		}

		// Create destination object with different managedFields
		dest := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:    "backend-server",
						Operation:  metav1.ManagedFieldsOperationUpdate,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:metadata":{"f:labels":{}}}`),
						},
					},
					// This should be overwritten by source (same manager+operation)
					{
						Manager:    "kubectl-apply",
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:metadata":{"f:annotations":{}}}`),
						},
					},
				},
			},
		}

		// Preserve managedFields
		err := storage.preserveManagedFields(source, dest)
		require.NoError(t, err)

		// Verify managedFields were merged correctly
		assert.NotNil(t, dest.ManagedFields)
		assert.Len(t, dest.ManagedFields, 3) // 2 from source + 1 unique from dest

		// Build a map for easy verification
		fieldMap := make(map[string]metav1.ManagedFieldsEntry)
		for _, field := range dest.ManagedFields {
			key := field.Manager + ":" + string(field.Operation) + ":" + field.Subresource
			fieldMap[key] = field
		}

		// Check kubectl-apply entry (should be from source, overwriting dest)
		kubectlField := fieldMap["kubectl-apply:Apply:"]
		assert.Equal(t, `{"f:spec":{"f:description":{}}}`, string(kubectlField.FieldsV1.Raw))

		// Check netguard-controller entry (should be from source)
		controllerField := fieldMap["netguard-controller:Update:status"]
		assert.Equal(t, `{"f:status":{"f:conditions":{}}}`, string(controllerField.FieldsV1.Raw))
		assert.Equal(t, "status", controllerField.Subresource)

		// Check backend-server entry (should be preserved from dest)
		backendField := fieldMap["backend-server:Update:"]
		assert.Equal(t, `{"f:metadata":{"f:labels":{}}}`, string(backendField.FieldsV1.Raw))
	})

	t.Run("PreserveManagedFields_NilSource", func(t *testing.T) {
		dest := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
		}

		err := storage.preserveManagedFields(nil, dest)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "source or destination is nil")
	})

	t.Run("PreserveManagedFields_NilDestination", func(t *testing.T) {
		source := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
		}

		err := storage.preserveManagedFields(source, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "source or destination is nil")
	})

	t.Run("PreserveManagedFields_EmptySourceManagedFields", func(t *testing.T) {
		source := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:          "test-service",
				Namespace:     "default",
				ManagedFields: nil, // No managedFields
			},
		}

		dest := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
		}

		err := storage.preserveManagedFields(source, dest)
		require.NoError(t, err)

		// Destination should remain unchanged
		assert.Nil(t, dest.ManagedFields)
	})

	t.Run("PreserveManagedFields_DeepCopyPrevention", func(t *testing.T) {
		// Create source with managedFields
		now := metav1.NewTime(time.Now())
		source := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:    "test-manager",
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: "netguard.sgroups.io/v1beta1",
						Time:       &now,
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{
							Raw: []byte(`{"f:spec":{"f:description":{}}}`),
						},
					},
				},
			},
		}

		dest := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
		}

		// Preserve managedFields
		err := storage.preserveManagedFields(source, dest)
		require.NoError(t, err)

		// Modify the source managedFields
		originalManager := source.ManagedFields[0].Manager
		source.ManagedFields[0].Manager = "modified-manager"

		// Destination should not be affected (deep copy protection)
		assert.Equal(t, "test-manager", dest.ManagedFields[0].Manager)
		assert.NotEqual(t, dest.ManagedFields[0].Manager, source.ManagedFields[0].Manager)

		// Restore for consistency
		source.ManagedFields[0].Manager = originalManager
	})
}

func TestBaseStorage_MergeManagedFields(t *testing.T) {
	storage := createTestServiceStorage()

	t.Run("MergeManagedFields_NoConflicts", func(t *testing.T) {
		now := metav1.NewTime(time.Now())
		sourceFields := []metav1.ManagedFieldsEntry{
			{
				Manager:    "manager-a",
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: "v1",
				Time:       &now,
				FieldsType: "FieldsV1",
			},
		}

		destFields := []metav1.ManagedFieldsEntry{
			{
				Manager:    "manager-b",
				Operation:  metav1.ManagedFieldsOperationUpdate,
				APIVersion: "v1",
				Time:       &now,
				FieldsType: "FieldsV1",
			},
		}

		merged := storage.mergeManagedFields(sourceFields, destFields)

		assert.Len(t, merged, 2)

		// Check that both fields are present
		managerNames := make([]string, 0, 2)
		for _, field := range merged {
			managerNames = append(managerNames, field.Manager)
		}
		assert.Contains(t, managerNames, "manager-a")
		assert.Contains(t, managerNames, "manager-b")
	})

	t.Run("MergeManagedFields_WithConflicts", func(t *testing.T) {
		now := metav1.NewTime(time.Now())
		sourceFields := []metav1.ManagedFieldsEntry{
			{
				Manager:    "same-manager",
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: "v1",
				Time:       &now,
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{
					Raw: []byte(`{"source":"data"}`),
				},
			},
		}

		destFields := []metav1.ManagedFieldsEntry{
			{
				Manager:    "same-manager",
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: "v1",
				Time:       &now,
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{
					Raw: []byte(`{"dest":"data"}`),
				},
			},
		}

		merged := storage.mergeManagedFields(sourceFields, destFields)

		// Should prioritize source field (only one entry)
		assert.Len(t, merged, 1)
		assert.Equal(t, "same-manager", merged[0].Manager)
		assert.Equal(t, `{"source":"data"}`, string(merged[0].FieldsV1.Raw))
	})

	t.Run("MergeManagedFields_WithSubresources", func(t *testing.T) {
		now := metav1.NewTime(time.Now())
		sourceFields := []metav1.ManagedFieldsEntry{
			{
				Manager:     "same-manager",
				Operation:   metav1.ManagedFieldsOperationUpdate,
				APIVersion:  "v1",
				Time:        &now,
				FieldsType:  "FieldsV1",
				Subresource: "status",
				FieldsV1: &metav1.FieldsV1{
					Raw: []byte(`{"status":"source"}`),
				},
			},
		}

		destFields := []metav1.ManagedFieldsEntry{
			{
				Manager:     "same-manager",
				Operation:   metav1.ManagedFieldsOperationUpdate,
				APIVersion:  "v1",
				Time:        &now,
				FieldsType:  "FieldsV1",
				Subresource: "", // Different subresource (main resource)
				FieldsV1: &metav1.FieldsV1{
					Raw: []byte(`{"main":"dest"}`),
				},
			},
		}

		merged := storage.mergeManagedFields(sourceFields, destFields)

		// Should have both entries (different subresources)
		assert.Len(t, merged, 2)

		subresources := make(map[string]string)
		for _, field := range merged {
			subresources[field.Subresource] = string(field.FieldsV1.Raw)
		}

		assert.Equal(t, `{"status":"source"}`, subresources["status"])
		assert.Equal(t, `{"main":"dest"}`, subresources[""])
	})
}

func TestBaseStorage_ManagedFieldKey(t *testing.T) {
	storage := createTestServiceStorage()
	now := metav1.NewTime(time.Now())

	t.Run("ManagedFieldKey_Generation", func(t *testing.T) {
		field := metav1.ManagedFieldsEntry{
			Manager:     "test-manager",
			Operation:   metav1.ManagedFieldsOperationApply,
			APIVersion:  "v1",
			Time:        &now,
			FieldsType:  "FieldsV1",
			Subresource: "status",
		}

		key := storage.managedFieldKey(field)
		expected := "test-manager:Apply:status"
		assert.Equal(t, expected, key)
	})

	t.Run("ManagedFieldKey_EmptySubresource", func(t *testing.T) {
		field := metav1.ManagedFieldsEntry{
			Manager:     "test-manager",
			Operation:   metav1.ManagedFieldsOperationUpdate,
			APIVersion:  "v1",
			Time:        &now,
			FieldsType:  "FieldsV1",
			Subresource: "", // Empty subresource
		}

		key := storage.managedFieldKey(field)
		expected := "test-manager:Update:"
		assert.Equal(t, expected, key)
	})

	t.Run("ManagedFieldKey_Uniqueness", func(t *testing.T) {
		field1 := metav1.ManagedFieldsEntry{
			Manager:     "manager-a",
			Operation:   metav1.ManagedFieldsOperationApply,
			Subresource: "status",
		}

		field2 := metav1.ManagedFieldsEntry{
			Manager:     "manager-a",
			Operation:   metav1.ManagedFieldsOperationApply,
			Subresource: "", // Different subresource
		}

		field3 := metav1.ManagedFieldsEntry{
			Manager:     "manager-a",
			Operation:   metav1.ManagedFieldsOperationUpdate, // Different operation
			Subresource: "status",
		}

		key1 := storage.managedFieldKey(field1)
		key2 := storage.managedFieldKey(field2)
		key3 := storage.managedFieldKey(field3)

		// All keys should be different
		assert.NotEqual(t, key1, key2)
		assert.NotEqual(t, key1, key3)
		assert.NotEqual(t, key2, key3)
	})
}

// createTestServiceStorage creates a BaseStorage instance for testing
func createTestServiceStorage() *BaseStorage[*netguardv1beta1.Service, *models.Service] {
	return NewBaseStorage[*netguardv1beta1.Service, *models.Service](
		func() *netguardv1beta1.Service { return &netguardv1beta1.Service{} },
		func() runtime.Object { return &netguardv1beta1.ServiceList{} },
		&mockBackendOps{},
		&mockConverter{},
		&mockValidator{},
		watch.NewBroadcaster(100, watch.DropIfChannelFull),
		"services",
		"Service",
		true,
	)
}

// Mock implementations for testing
type mockConverter struct{}

func (m *mockConverter) ToDomain(ctx context.Context, k8sObj *netguardv1beta1.Service) (*models.Service, error) {
	return &models.Service{}, nil
}

func (m *mockConverter) FromDomain(ctx context.Context, domainObj *models.Service) (*netguardv1beta1.Service, error) {
	return &netguardv1beta1.Service{}, nil
}

func (m *mockConverter) ToList(ctx context.Context, domainObjs []*models.Service) (runtime.Object, error) {
	return &netguardv1beta1.ServiceList{}, nil
}

type mockBackendOps struct{}

func (m *mockBackendOps) Get(ctx context.Context, id models.ResourceIdentifier) (**models.Service, error) {
	service := &models.Service{}
	return &service, nil
}

func (m *mockBackendOps) List(ctx context.Context, scope ports.Scope) ([]*models.Service, error) {
	return []*models.Service{}, nil
}

func (m *mockBackendOps) Create(ctx context.Context, obj **models.Service) error {
	return nil
}

func (m *mockBackendOps) Update(ctx context.Context, obj **models.Service) error {
	return nil
}

func (m *mockBackendOps) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return nil
}

type mockValidator struct{}

func (m *mockValidator) ValidateCreate(ctx context.Context, obj *netguardv1beta1.Service) field.ErrorList {
	return nil
}

func (m *mockValidator) ValidateUpdate(ctx context.Context, new, old *netguardv1beta1.Service) field.ErrorList {
	return nil
}

func (m *mockValidator) ValidateDelete(ctx context.Context, obj *netguardv1beta1.Service) field.ErrorList {
	return nil
}
