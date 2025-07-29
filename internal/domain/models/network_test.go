package models

import (
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"testing"
)

func TestNetwork_NewNetwork(t *testing.T) {
	network := NewNetwork("test-network", "default", "192.168.1.0/24")

	if network.Name != "test-network" {
		t.Errorf("Expected name 'test-network', got '%s'", network.Name)
	}

	if network.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", network.Namespace)
	}

	if network.CIDR != "192.168.1.0/24" {
		t.Errorf("Expected CIDR '192.168.1.0/24', got '%s'", network.CIDR)
	}

	if network.IsBound {
		t.Error("Expected IsBound to be false for new network")
	}
}

func TestNetwork_GetName(t *testing.T) {
	network := &Network{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{Name: "test-network"},
		},
	}
	if network.GetName() != "test-network" {
		t.Errorf("Expected 'test-network', got '%s'", network.GetName())
	}
}

func TestNetwork_GetNamespace(t *testing.T) {
	network := &Network{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{Namespace: "default"},
		},
	}
	if network.GetNamespace() != "default" {
		t.Errorf("Expected 'default', got '%s'", network.GetNamespace())
	}
}

func TestNetwork_Key(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		expected  string
	}{
		{
			name:      "test-network",
			namespace: "default",
			expected:  "default/test-network",
		},
		{
			name:      "test-network",
			namespace: "",
			expected:  "test-network",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network := &Network{
				SelfRef: SelfRef{
					ResourceIdentifier: ResourceIdentifier{
						Name:      tt.name,
						Namespace: tt.namespace,
					},
				},
			}
			if network.Key() != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, network.Key())
			}
		})
	}
}

func TestNetwork_GetID(t *testing.T) {
	network := &Network{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Name:      "test-network",
				Namespace: "default",
			},
		},
	}
	if network.GetID() != "default/test-network" {
		t.Errorf("Expected 'default/test-network', got '%s'", network.GetID())
	}
}

func TestNetwork_GetGeneration(t *testing.T) {
	network := &Network{
		Meta: Meta{Generation: 42},
	}
	if network.GetGeneration() != 42 {
		t.Errorf("Expected 42, got %d", network.GetGeneration())
	}
}

func TestNetwork_SetIsBound(t *testing.T) {
	network := &Network{}
	network.SetIsBound(true)
	if !network.IsBound {
		t.Error("Expected IsBound to be true")
	}

	network.SetIsBound(false)
	if network.IsBound {
		t.Error("Expected IsBound to be false")
	}
}

func TestNetwork_ClearBinding(t *testing.T) {
	network := &Network{
		IsBound:         true,
		BindingRef:      &v1beta1.ObjectReference{Name: "test-binding"},
		AddressGroupRef: &v1beta1.ObjectReference{Name: "test-group"},
	}

	network.ClearBinding()

	if network.IsBound {
		t.Error("Expected IsBound to be false after clearing")
	}

	if network.BindingRef != nil {
		t.Error("Expected BindingRef to be nil after clearing")
	}

	if network.AddressGroupRef != nil {
		t.Error("Expected AddressGroupRef to be nil after clearing")
	}
}
