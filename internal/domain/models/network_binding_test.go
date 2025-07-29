package models

import (
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"testing"
)

func TestNetworkBinding_NewNetworkBinding(t *testing.T) {
	networkRef := v1beta1.ObjectReference{Name: "test-network"}
	addressGroupRef := v1beta1.ObjectReference{Name: "test-group"}

	binding := NewNetworkBinding("test-binding", "default", networkRef, addressGroupRef)

	if binding.Name != "test-binding" {
		t.Errorf("Expected name 'test-binding', got '%s'", binding.Name)
	}

	if binding.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", binding.Namespace)
	}

	if binding.NetworkRef.Name != "test-network" {
		t.Errorf("Expected network ref name 'test-network', got '%s'", binding.NetworkRef.Name)
	}

	if binding.AddressGroupRef.Name != "test-group" {
		t.Errorf("Expected address group ref name 'test-group', got '%s'", binding.AddressGroupRef.Name)
	}
}

func TestNetworkBinding_GetName(t *testing.T) {
	binding := &NetworkBinding{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{Name: "test-binding"},
		},
	}
	if binding.GetName() != "test-binding" {
		t.Errorf("Expected 'test-binding', got '%s'", binding.GetName())
	}
}

func TestNetworkBinding_GetNamespace(t *testing.T) {
	binding := &NetworkBinding{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{Namespace: "default"},
		},
	}
	if binding.GetNamespace() != "default" {
		t.Errorf("Expected 'default', got '%s'", binding.GetNamespace())
	}
}

func TestNetworkBinding_Key(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		expected  string
	}{
		{
			name:      "test-binding",
			namespace: "default",
			expected:  "default/test-binding",
		},
		{
			name:      "test-binding",
			namespace: "",
			expected:  "test-binding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binding := &NetworkBinding{
				SelfRef: SelfRef{
					ResourceIdentifier: ResourceIdentifier{
						Name:      tt.name,
						Namespace: tt.namespace,
					},
				},
			}
			if binding.Key() != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, binding.Key())
			}
		})
	}
}

func TestNetworkBinding_GetID(t *testing.T) {
	binding := &NetworkBinding{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Name:      "test-binding",
				Namespace: "default",
			},
		},
	}
	if binding.GetID() != "default/test-binding" {
		t.Errorf("Expected 'default/test-binding', got '%s'", binding.GetID())
	}
}

func TestNetworkBinding_GetGeneration(t *testing.T) {
	binding := &NetworkBinding{
		Meta: Meta{Generation: 42},
	}
	if binding.GetGeneration() != 42 {
		t.Errorf("Expected 42, got %d", binding.GetGeneration())
	}
}

func TestNetworkBinding_SetNetworkItem(t *testing.T) {
	binding := &NetworkBinding{}
	networkItem := NetworkItem{
		CIDR: "192.168.1.0/24",
	}

	binding.SetNetworkItem(networkItem)

	if binding.NetworkItem.CIDR != "192.168.1.0/24" {
		t.Errorf("Expected CIDR '192.168.1.0/24', got '%s'", binding.NetworkItem.CIDR)
	}
}
