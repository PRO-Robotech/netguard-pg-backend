package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/registry"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// ============================================================================
// Test: extractAddressGroupDependencies
// ============================================================================

func TestExtractAddressGroupDependencies_WithHosts(t *testing.T) {
	// Create mock worker
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	// Create AddressGroup with 3 hosts in AggregatedHosts
	// IMPORTANT: AggregatedHosts is for status display only, NOT dependencies!
	// AddressGroup should have NO dependencies on Hosts to avoid circular dependency
	ag := &models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-ag",
				Namespace: "default",
			},
		},
		AggregatedHosts: []models.HostReference{
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Host",
						Name:       "host-1",
					},
					Namespace: "default",
				},
				UUID:   "uuid-1",
				Source: models.HostSourceSpec,
			},
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Host",
						Name:       "host-2",
					},
					Namespace: "default",
				},
				UUID:   "uuid-2",
				Source: models.HostSourceBinding,
			},
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Host",
						Name:       "host-3",
					},
					Namespace: "default",
				},
				UUID:   "uuid-3",
				Source: models.HostSourceSpec,
			},
		},
	}

	// Extract dependencies
	deps, err := worker.extractAddressGroupDependencies(ag)
	require.NoError(t, err)

	// Verify: AddressGroup should have NO dependencies on Hosts
	// This prevents circular dependency deadlock (Host→AG, AG→Host)
	assert.Len(t, deps, 0, "AddressGroup should NOT depend on Hosts in AggregatedHosts")
}

func TestExtractAddressGroupDependencies_NoHosts(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	// Create AddressGroup with NO hosts
	ag := &models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-ag",
				Namespace: "default",
			},
		},
		AggregatedHosts: []models.HostReference{}, // Empty
	}

	// Extract dependencies
	deps, err := worker.extractAddressGroupDependencies(ag)
	require.NoError(t, err)

	// Verify
	assert.Len(t, deps, 0, "should have no dependencies")
}

func TestExtractAddressGroupDependencies_InvalidType(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	// Pass wrong type
	_, err := worker.extractAddressGroupDependencies("not-an-address-group")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource type")
}

// ============================================================================
// Test: extractNetworkDependencies
// ============================================================================

func TestExtractNetworkDependencies_WithAddressGroup(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	// Create Network with AddressGroup reference
	network := &models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-network",
				Namespace: "default",
			},
		},
		AddressGroupRef: &netguardv1beta1.NamespacedObjectReference{
			ObjectReference: netguardv1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "test-ag",
			},
			Namespace: "default",
		},
	}

	// Extract dependencies
	deps, err := worker.extractNetworkDependencies(network)
	require.NoError(t, err)

	// Verify
	assert.Len(t, deps, 1, "should have 1 AddressGroup dependency")
	assert.Equal(t, string(registry.TypeAddressGroup), deps[0].Type)
	assert.Equal(t, "test-ag", deps[0].Name)
	assert.Equal(t, "default", deps[0].Namespace) // Same as Network
	assert.Contains(t, deps[0].Reason, "1:1")
}

func TestExtractNetworkDependencies_NoAddressGroup(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	// Create Network WITHOUT AddressGroup reference
	network := &models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-network",
				Namespace: "default",
			},
		},
		AddressGroupRef: nil, // No reference
	}

	// Extract dependencies
	deps, err := worker.extractNetworkDependencies(network)
	require.NoError(t, err)

	// Verify
	assert.Len(t, deps, 0, "should have no dependencies")
}

func TestExtractNetworkDependencies_InvalidType(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	_, err := worker.extractNetworkDependencies("not-a-network")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource type")
}

// ============================================================================
// Test: extractServiceDependencies
// ============================================================================

func TestExtractServiceDependencies_WithAddressGroups(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	// Create Service with 2 AddressGroups
	service := &models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-service",
				Namespace: "default",
			},
		},
		AggregatedAddressGroups: []models.AddressGroupReference{
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "ag-1",
					},
					Namespace: "default",
				},
				Source: models.AddressGroupSourceSpec,
			},
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "AddressGroup",
						Name:       "ag-2",
					},
					Namespace: "default",
				},
				Source: models.AddressGroupSourceBinding,
			},
		},
	}

	// Extract dependencies
	deps, err := worker.extractServiceDependencies(service)
	require.NoError(t, err)

	// Verify
	assert.Len(t, deps, 2, "should have 2 AddressGroup dependencies")
	assert.Equal(t, string(registry.TypeAddressGroup), deps[0].Type)
	assert.Equal(t, "ag-1", deps[0].Name)
	assert.Equal(t, "default", deps[0].Namespace)
	assert.Contains(t, deps[0].Reason, "spec")

	assert.Equal(t, "ag-2", deps[1].Name)
	assert.Contains(t, deps[1].Reason, "binding")
}

func TestExtractServiceDependencies_NoAddressGroups(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	// Create Service with NO AddressGroups
	service := &models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-service",
				Namespace: "default",
			},
		},
		AggregatedAddressGroups: []models.AddressGroupReference{}, // Empty
	}

	// Extract dependencies
	deps, err := worker.extractServiceDependencies(service)
	require.NoError(t, err)

	// Verify
	assert.Len(t, deps, 0, "should have no dependencies")
}

func TestExtractServiceDependencies_InvalidType(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	_, err := worker.extractServiceDependencies("not-a-service")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource type")
}

// ============================================================================
// Test: extractEntityDependencies (dispatcher)
// ============================================================================

func TestExtractEntityDependencies_Host(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	// Host has no dependencies
	host := &models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-host",
				Namespace: "default",
			},
		},
	}

	deps, err := worker.extractEntityDependencies(string(registry.TypeHost), host)
	require.NoError(t, err)
	assert.Len(t, deps, 0, "Host should have no dependencies")
}

func TestExtractEntityDependencies_UnknownType(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	_, err := worker.extractEntityDependencies("UnknownType", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown entity resource type")
}

// ============================================================================
// Test: Dependency Deduplication
// ============================================================================

func TestExtractAddressGroupDependencies_DuplicateHosts(t *testing.T) {
	// Test that duplicate hosts (same name) from different sources
	// are properly extracted
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	ag := &models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-ag",
				Namespace: "default",
			},
		},
		AggregatedHosts: []models.HostReference{
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Host",
						Name:       "host-1",
					},
					Namespace: "default",
				},
				Source: models.HostSourceSpec,
			},
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Host",
						Name:       "host-1", // Same host, different source
					},
					Namespace: "default",
				},
				Source: models.HostSourceBinding,
			},
		},
	}

	deps, err := worker.extractAddressGroupDependencies(ag)
	require.NoError(t, err)

	// AddressGroup should have NO dependencies on Hosts
	// AggregatedHosts is for status display only, not for SGROUP sync
	assert.Len(t, deps, 0, "AddressGroup should NOT depend on Hosts, even from different sources")
}

func TestComputeResourceReadinessFlag(t *testing.T) {
	cases := []struct {
		name     string
		ready    bool
		deleting bool
		expected bool
	}{
		{name: "ready", ready: true, deleting: false, expected: true},
		{name: "deleting", ready: false, deleting: true, expected: true},
		{name: "neither", ready: false, deleting: false, expected: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, computeResourceReadinessFlag(tc.ready, tc.deleting))
		})
	}
}
