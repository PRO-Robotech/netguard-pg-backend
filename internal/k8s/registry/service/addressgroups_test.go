package service

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
)

func TestAddressGroupsREST_Get(t *testing.T) {
	// Create mock backend client
	mockClient := client.NewMockBackendClient()

	// Create addressgroups REST handler
	addressGroupsREST := NewAddressGroupsREST(mockClient)

	// Prepare context with namespace
	ctx := context.WithValue(context.Background(), "namespace", "default")

	// Test Get operation
	result, err := addressGroupsREST.Get(ctx, "test-service", &metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Verify result type
	addressGroupsSpec, ok := result.(*netguardv1beta1.AddressGroupsSpec)
	if !ok {
		t.Fatalf("Expected *AddressGroupsSpec, got %T", result)
	}

	// Verify metadata
	if addressGroupsSpec.ObjectMeta.Name != "test-service" {
		t.Errorf("Expected name 'test-service', got %s", addressGroupsSpec.ObjectMeta.Name)
	}
	if addressGroupsSpec.ObjectMeta.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got %s", addressGroupsSpec.ObjectMeta.Namespace)
	}
}

func TestAddressGroupsREST_New(t *testing.T) {
	mockClient := client.NewMockBackendClient()
	addressGroupsREST := NewAddressGroupsREST(mockClient)

	obj := addressGroupsREST.New()

	addressGroupsSpec, ok := obj.(*netguardv1beta1.AddressGroupsSpec)
	if !ok {
		t.Fatalf("Expected *AddressGroupsSpec, got %T", obj)
	}

	// Verify TypeMeta
	if addressGroupsSpec.APIVersion != "netguard.sgroups.io/v1beta1" {
		t.Errorf("Expected APIVersion 'netguard.sgroups.io/v1beta1', got %s", addressGroupsSpec.APIVersion)
	}
	if addressGroupsSpec.Kind != "AddressGroupsSpec" {
		t.Errorf("Expected Kind 'AddressGroupsSpec', got %s", addressGroupsSpec.Kind)
	}
}

func TestAddressGroupsREST_ConvertToTable(t *testing.T) {
	mockClient := client.NewMockBackendClient()
	addressGroupsREST := NewAddressGroupsREST(mockClient)

	// Create test object
	addressGroupsSpec := &netguardv1beta1.AddressGroupsSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroupsSpec",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Items: []netguardv1beta1.NamespacedObjectReference{
			{
				ObjectReference: netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "test-ag",
				},
				Namespace: "default",
			},
		},
	}

	// Test ConvertToTable
	ctx := context.Background()
	table, err := addressGroupsREST.ConvertToTable(ctx, addressGroupsSpec, nil)
	if err != nil {
		t.Fatalf("ConvertToTable failed: %v", err)
	}

	// Verify table structure
	if len(table.ColumnDefinitions) != 3 {
		t.Errorf("Expected 3 columns, got %d", len(table.ColumnDefinitions))
	}
	if len(table.Rows) != 1 {
		t.Errorf("Expected 1 row, got %d", len(table.Rows))
	}

	// Verify row data
	row := table.Rows[0]
	if len(row.Cells) != 3 {
		t.Errorf("Expected 3 cells, got %d", len(row.Cells))
	}
	if row.Cells[0] != "test-service" {
		t.Errorf("Expected service name 'test-service', got %v", row.Cells[0])
	}
	if row.Cells[1] != "default" {
		t.Errorf("Expected namespace 'default', got %v", row.Cells[1])
	}
	if row.Cells[2] != 1 {
		t.Errorf("Expected 1 address group, got %v", row.Cells[2])
	}
}
