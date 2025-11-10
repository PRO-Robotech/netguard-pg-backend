package worker

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/domain/models"
)

// Test_convertToSyncOperation tests the conversion from domain to sync operations
func Test_convertToSyncOperation(t *testing.T) {
	tests := []struct {
		name     string
		input    domain.SyncOperation
		expected string
	}{
		{
			name:     "CREATE converts to Upsert",
			input:    domain.SyncOperationCreate,
			expected: "Upsert",
		},
		{
			name:     "UPDATE converts to Update",
			input:    domain.SyncOperationUpdate,
			expected: "Update",
		},
		{
			name:     "DELETE converts to Delete",
			input:    domain.SyncOperationDelete,
			expected: "Delete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToSyncOperation(tt.input)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

// Test_applyDelta_AddressGroup tests delta application for AddressGroup
func Test_applyDelta_AddressGroup(t *testing.T) {
	t.Run("add hosts", func(t *testing.T) {
		ag := &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-ag",
					Namespace: "default",
				},
			},
			Hosts: []netguardv1beta1.NamespacedObjectReference{
				{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Host",
						Name:       "host-1",
					},
					Namespace: "default",
				},
			},
		}

		delta := AddressGroupDelta{
			AddHosts: []string{"host-2", "host-3"},
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(ag, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		assert.Len(t, ag.Hosts, 3)
		assert.Equal(t, "host-1", ag.Hosts[0].Name)
		assert.Equal(t, "host-2", ag.Hosts[1].Name)
		assert.Equal(t, "host-3", ag.Hosts[2].Name)
	})

	t.Run("remove hosts", func(t *testing.T) {
		ag := &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-ag",
					Namespace: "default",
				},
			},
			Hosts: []netguardv1beta1.NamespacedObjectReference{
				{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Host",
						Name:       "host-1",
					},
					Namespace: "default",
				},
				{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Host",
						Name:       "host-2",
					},
					Namespace: "default",
				},
				{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Host",
						Name:       "host-3",
					},
					Namespace: "default",
				},
			},
		}

		delta := AddressGroupDelta{
			RemoveHosts: []string{"host-2"},
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(ag, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		assert.Len(t, ag.Hosts, 2)
		assert.Equal(t, "host-1", ag.Hosts[0].Name)
		assert.Equal(t, "host-3", ag.Hosts[1].Name)
	})

	t.Run("add networks", func(t *testing.T) {
		ag := &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-ag",
					Namespace: "default",
				},
			},
			Networks: []models.NetworkItem{
				{Name: "net-1", CIDR: "10.0.0.0/24"},
			},
		}

		delta := AddressGroupDelta{
			AddNetworks: []models.NetworkItem{
				{Name: "net-2", CIDR: "192.168.0.0/24"},
			},
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(ag, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		assert.Len(t, ag.Networks, 2)
		assert.Equal(t, "net-1", ag.Networks[0].Name)
		assert.Equal(t, "net-2", ag.Networks[1].Name)
	})

	t.Run("replace default action", func(t *testing.T) {
		ag := &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-ag",
					Namespace: "default",
				},
			},
			DefaultAction: models.ActionAccept,
		}

		newAction := models.ActionDrop
		delta := AddressGroupDelta{
			ReplaceDefaultAction: &newAction,
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(ag, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		assert.Equal(t, models.ActionDrop, ag.DefaultAction)
	})

	t.Run("replace logs and trace", func(t *testing.T) {
		ag := &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-ag",
					Namespace: "default",
				},
			},
			Logs:  false,
			Trace: false,
		}

		trueVal := true
		delta := AddressGroupDelta{
			ReplaceLogs:  &trueVal,
			ReplaceTrace: &trueVal,
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(ag, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		assert.True(t, ag.Logs)
		assert.True(t, ag.Trace)
	})

	t.Run("no delta on CREATE", func(t *testing.T) {
		ag := &models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-ag",
					Namespace: "default",
				},
			},
			Hosts: []netguardv1beta1.NamespacedObjectReference{
				{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Host",
						Name:       "host-1",
					},
					Namespace: "default",
				},
			},
		}

		delta := AddressGroupDelta{
			AddHosts: []string{"host-2"},
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		// Delta should be ignored for CREATE operations
		err = applyDelta(ag, domain.SyncOperationCreate, deltaJSON)
		require.NoError(t, err)

		// Hosts should remain unchanged
		assert.Len(t, ag.Hosts, 1)
		assert.Equal(t, "host-1", ag.Hosts[0].Name)
	})
}

// Test_applyDelta_Host tests delta application for Host
func Test_applyDelta_Host(t *testing.T) {
	t.Run("replace IP list", func(t *testing.T) {
		host := &models.Host{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-host",
					Namespace: "default",
				},
			},
			UUID: "test-uuid",
		}
		host.SetIpList([]string{"10.0.0.1", "10.0.0.2"})

		delta := HostDelta{
			ReplaceIpList: []string{"192.168.1.1", "192.168.1.2"},
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(host, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		ipList := host.GetIpList()
		assert.Len(t, ipList, 2)
		assert.Equal(t, "192.168.1.1", ipList[0])
		assert.Equal(t, "192.168.1.2", ipList[1])
	})

	t.Run("add IPs", func(t *testing.T) {
		host := &models.Host{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-host",
					Namespace: "default",
				},
			},
			UUID: "test-uuid",
		}
		host.SetIpList([]string{"10.0.0.1"})

		delta := HostDelta{
			AddIPs: []string{"10.0.0.2", "10.0.0.3"},
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(host, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		ipList := host.GetIpList()
		assert.Len(t, ipList, 3)
		assert.Contains(t, ipList, "10.0.0.1")
		assert.Contains(t, ipList, "10.0.0.2")
		assert.Contains(t, ipList, "10.0.0.3")
	})

	t.Run("remove IPs", func(t *testing.T) {
		host := &models.Host{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-host",
					Namespace: "default",
				},
			},
			UUID: "test-uuid",
		}
		host.SetIpList([]string{"10.0.0.1", "10.0.0.2", "10.0.0.3"})

		delta := HostDelta{
			RemoveIPs: []string{"10.0.0.2"},
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(host, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		ipList := host.GetIpList()
		assert.Len(t, ipList, 2)
		assert.Contains(t, ipList, "10.0.0.1")
		assert.Contains(t, ipList, "10.0.0.3")
		assert.NotContains(t, ipList, "10.0.0.2")
	})
}

// Test_applyDelta_Network tests delta application for Network
func Test_applyDelta_Network(t *testing.T) {
	t.Run("replace CIDR", func(t *testing.T) {
		network := &models.Network{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-network",
					Namespace: "default",
				},
			},
			CIDR: "10.0.0.0/16",
		}

		newCIDR := "192.168.0.0/24"
		delta := NetworkDelta{
			ReplaceCIDR: &newCIDR,
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(network, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		assert.Equal(t, "192.168.0.0/24", network.CIDR)
	})
}

// Test_applyDelta_Service tests delta application for Service
func Test_applyDelta_Service(t *testing.T) {
	t.Run("replace ingress ports", func(t *testing.T) {
		service := &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-service",
					Namespace: "default",
				},
			},
			IngressPorts: []models.IngressPort{
				{Port: "80", Protocol: models.TCP},
			},
		}

		delta := ServiceDelta{
			ReplaceIngressPorts: []models.IngressPort{
				{Port: "443", Protocol: models.TCP},
				{Port: "8080", Protocol: models.TCP},
			},
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(service, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		assert.Len(t, service.IngressPorts, 2)
		assert.Equal(t, "443", service.IngressPorts[0].Port)
		assert.Equal(t, "8080", service.IngressPorts[1].Port)
	})

	t.Run("add ingress ports", func(t *testing.T) {
		service := &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-service",
					Namespace: "default",
				},
			},
			IngressPorts: []models.IngressPort{
				{Port: "80", Protocol: models.TCP},
			},
		}

		delta := ServiceDelta{
			AddIngressPorts: []models.IngressPort{
				{Port: "443", Protocol: models.TCP},
			},
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(service, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		assert.Len(t, service.IngressPorts, 2)
		assert.Equal(t, "80", service.IngressPorts[0].Port)
		assert.Equal(t, "443", service.IngressPorts[1].Port)
	})

	t.Run("remove ingress ports", func(t *testing.T) {
		service := &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-service",
					Namespace: "default",
				},
			},
			IngressPorts: []models.IngressPort{
				{Port: "80", Protocol: models.TCP},
				{Port: "443", Protocol: models.TCP},
				{Port: "8080", Protocol: models.TCP},
			},
		}

		delta := ServiceDelta{
			RemoveIngressPorts: []string{"443"},
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(service, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		assert.Len(t, service.IngressPorts, 2)
		assert.Equal(t, "80", service.IngressPorts[0].Port)
		assert.Equal(t, "8080", service.IngressPorts[1].Port)
	})

	t.Run("replace description", func(t *testing.T) {
		service := &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-service",
					Namespace: "default",
				},
			},
			Description: "Old description",
		}

		newDesc := "New description"
		delta := ServiceDelta{
			ReplaceDescription: &newDesc,
		}
		deltaJSON, err := json.Marshal(delta)
		require.NoError(t, err)

		err = applyDelta(service, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		assert.Equal(t, "New description", service.Description)
	})
}

// Test_applyDelta_InvalidJSON tests error handling for invalid delta JSON
func Test_applyDelta_InvalidJSON(t *testing.T) {
	ag := &models.AddressGroup{}
	invalidJSON := json.RawMessage(`{invalid json}`)

	err := applyDelta(ag, domain.SyncOperationUpdate, invalidJSON)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

// Test_applyDelta_NilDelta tests that nil delta is handled gracefully
func Test_applyDelta_NilDelta(t *testing.T) {
	ag := &models.AddressGroup{}

	err := applyDelta(ag, domain.SyncOperationUpdate, nil)
	assert.NoError(t, err)
}

// Test_applyDelta_EmptyDelta tests that empty delta is handled gracefully
func Test_applyDelta_EmptyDelta(t *testing.T) {
	ag := &models.AddressGroup{}

	err := applyDelta(ag, domain.SyncOperationUpdate, json.RawMessage{})
	assert.NoError(t, err)
}

// Test coverage helper: Verify delta application avoids duplicates
func Test_applyDelta_AvoidsDuplicates(t *testing.T) {
	t.Run("AddressGroup hosts", func(t *testing.T) {
		ag := &models.AddressGroup{
			Hosts: []netguardv1beta1.NamespacedObjectReference{
				{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Host",
						Name:       "host-1",
					},
					Namespace: "default",
				},
			},
		}

		// Try to add host-1 again
		delta := AddressGroupDelta{
			AddHosts: []string{"host-1", "host-2"},
		}
		deltaJSON, _ := json.Marshal(delta)

		err := applyDelta(ag, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		// Should have host-1 only once, plus host-2
		assert.Len(t, ag.Hosts, 2)
	})

	t.Run("Host IPs", func(t *testing.T) {
		host := &models.Host{}
		host.SetIpList([]string{"10.0.0.1"})

		delta := HostDelta{
			AddIPs: []string{"10.0.0.1", "10.0.0.2"},
		}
		deltaJSON, _ := json.Marshal(delta)

		err := applyDelta(host, domain.SyncOperationUpdate, deltaJSON)
		require.NoError(t, err)

		ipList := host.GetIpList()
		// Should have 10.0.0.1 only once, plus 10.0.0.2
		assert.Len(t, ipList, 2)
	})
}
