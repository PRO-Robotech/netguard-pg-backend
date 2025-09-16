package synchronizer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/sync/types"
)

func TestNewHostSynchronizer(t *testing.T) {
	hostReader := &MockHostReader{}
	hostWriter := &MockHostWriter{}
	sgroupReader := &MockSGROUPHostReader{}
	config := DefaultHostSyncConfig()

	synchronizer := NewHostSynchronizer(hostReader, hostWriter, sgroupReader, config)

	assert.NotNil(t, synchronizer)

	// Verify it implements the interface
	var _ HostSynchronizer = synchronizer
}

func TestHostSynchronizer_SyncHosts_EmptyNamespace(t *testing.T) {
	hostReader := &MockHostReader{}
	hostWriter := &MockHostWriter{}
	sgroupReader := &MockSGROUPHostReader{}
	config := DefaultHostSyncConfig()

	synchronizer := NewHostSynchronizer(hostReader, hostWriter, sgroupReader, config)

	// Mock: No hosts without IPSet
	hostReader.On("GetHostsWithoutIPSet", mock.Anything, "test-namespace").
		Return([]models.Host{}, nil)

	ctx := context.Background()
	result, err := synchronizer.SyncHosts(ctx, "test-namespace")

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.TotalRequested)
	assert.Equal(t, 0, result.TotalSynced)
	assert.Equal(t, 0, result.TotalFailed)
	assert.True(t, result.IsEmpty())

	hostReader.AssertExpectations(t)
}

func TestHostSynchronizer_SyncHosts_Success(t *testing.T) {
	hostReader := &MockHostReader{}
	hostWriter := &MockHostWriter{}
	sgroupReader := &MockSGROUPHostReader{}
	config := DefaultHostSyncConfig()

	synchronizer := NewHostSynchronizer(hostReader, hostWriter, sgroupReader, config)

	// Create test hosts
	host1 := models.Host{}
	host1.Name = "host1"
	host1.UUID = "uuid1"
	host1.Namespace = "default"

	host2 := models.Host{}
	host2.Name = "host2"
	host2.UUID = "uuid2"
	host2.Namespace = "default"

	hosts := []models.Host{host1, host2}

	// Create SGROUP hosts with IPSets
	sgroupHost1 := &pb.Host{
		Name:   "host1",
		Uuid:   "uuid1",
		SgName: "sg1",
		IpList: &pb.IPList{
			IPs: []string{"192.168.1.10", "10.0.0.10"},
		},
	}

	sgroupHost2 := &pb.Host{
		Name:   "host2",
		Uuid:   "uuid2",
		SgName: "sg2",
		IpList: &pb.IPList{
			IPs: []string{"192.168.1.20"},
		},
	}

	sgroupHosts := []*pb.Host{sgroupHost1, sgroupHost2}

	// Set up mocks
	hostReader.On("GetHostsWithoutIPSet", mock.Anything, "default").
		Return(hosts, nil)

	sgroupReader.On("GetHostsByUUIDs", mock.Anything, []string{"uuid1", "uuid2"}).
		Return(sgroupHosts, nil)

	// Mock the writer to expect updates
	expectedUpdates := []types.HostIPSetUpdate{
		{
			HostUUID:  "uuid1",
			HostID:    host1.GetID(),
			Namespace: "default",
			Name:      "host1",
			IPSet:     []string{"192.168.1.10", "10.0.0.10"},
			SGName:    "sg1",
		},
		{
			HostUUID:  "uuid2",
			HostID:    host2.GetID(),
			Namespace: "default",
			Name:      "host2",
			IPSet:     []string{"192.168.1.20"},
			SGName:    "sg2",
		},
	}

	hostWriter.On("UpdateHostsIPSet", mock.Anything, expectedUpdates).
		Return(nil)

	ctx := context.Background()
	result, err := synchronizer.SyncHosts(ctx, "default")

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.TotalRequested)
	assert.Equal(t, 2, result.TotalSynced)
	assert.Equal(t, 0, result.TotalFailed)
	assert.False(t, result.HasErrors())
	assert.Equal(t, 100.0, result.SuccessRate())

	// Verify synced hosts
	assert.Contains(t, result.SyncedHostUUIDs, "uuid1")
	assert.Contains(t, result.SyncedHostUUIDs, "uuid2")

	hostReader.AssertExpectations(t)
	sgroupReader.AssertExpectations(t)
	hostWriter.AssertExpectations(t)
}

func TestHostSynchronizer_SyncHosts_SGROUPError(t *testing.T) {
	hostReader := &MockHostReader{}
	hostWriter := &MockHostWriter{}
	sgroupReader := &MockSGROUPHostReader{}
	config := DefaultHostSyncConfig()

	synchronizer := NewHostSynchronizer(hostReader, hostWriter, sgroupReader, config)

	// Create test host
	host1 := models.Host{}
	host1.Name = "host1"
	host1.UUID = "uuid1"
	host1.Namespace = "default"

	hosts := []models.Host{host1}

	// Set up mocks
	hostReader.On("GetHostsWithoutIPSet", mock.Anything, "default").
		Return(hosts, nil)

	sgroupReader.On("GetHostsByUUIDs", mock.Anything, []string{"uuid1"}).
		Return([]*pb.Host{}, errors.New("SGROUP connection failed"))

	ctx := context.Background()
	result, err := synchronizer.SyncHosts(ctx, "default")

	require.NoError(t, err) // Sync continues even with batch errors
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.TotalRequested)
	assert.Equal(t, 0, result.TotalSynced)
	assert.Equal(t, 1, result.TotalFailed)
	assert.True(t, result.HasErrors())

	// Verify failed host
	assert.Contains(t, result.FailedUUIDs, "uuid1")
	assert.Contains(t, result.GetError("uuid1"), "SGROUP query failed")

	hostReader.AssertExpectations(t)
	sgroupReader.AssertExpectations(t)
}

func TestHostSynchronizer_SyncHosts_NoIPSet(t *testing.T) {
	hostReader := &MockHostReader{}
	hostWriter := &MockHostWriter{}
	sgroupReader := &MockSGROUPHostReader{}
	config := DefaultHostSyncConfig()

	synchronizer := NewHostSynchronizer(hostReader, hostWriter, sgroupReader, config)

	// Create test host
	host1 := models.Host{}
	host1.Name = "host1"
	host1.UUID = "uuid1"
	host1.Namespace = "default"

	hosts := []models.Host{host1}

	// SGROUP host without IPSet
	sgroupHost1 := &pb.Host{
		Name:   "host1",
		Uuid:   "uuid1",
		SgName: "sg1",
		IpList: &pb.IPList{
			IPs: []string{}, // Empty IP list
		},
	}

	sgroupHosts := []*pb.Host{sgroupHost1}

	// Set up mocks
	hostReader.On("GetHostsWithoutIPSet", mock.Anything, "default").
		Return(hosts, nil)

	sgroupReader.On("GetHostsByUUIDs", mock.Anything, []string{"uuid1"}).
		Return(sgroupHosts, nil)

	ctx := context.Background()
	result, err := synchronizer.SyncHosts(ctx, "default")

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.TotalRequested)
	assert.Equal(t, 0, result.TotalSynced)
	assert.Equal(t, 1, result.TotalFailed)
	assert.True(t, result.HasErrors())

	// Verify failed host
	assert.Contains(t, result.FailedUUIDs, "uuid1")
	assert.Equal(t, "no valid IP addresses found in SGROUP", result.GetError("uuid1"))

	hostReader.AssertExpectations(t)
	sgroupReader.AssertExpectations(t)
}

func TestHostSynchronizer_SyncHostsByUUIDs(t *testing.T) {
	hostReader := &MockHostReader{}
	hostWriter := &MockHostWriter{}
	sgroupReader := &MockSGROUPHostReader{}
	config := DefaultHostSyncConfig()

	synchronizer := NewHostSynchronizer(hostReader, hostWriter, sgroupReader, config)

	// Create test host
	host1 := models.Host{}
	host1.Name = "host1"
	host1.UUID = "uuid1"
	host1.Namespace = "default"

	// SGROUP host with IPSet
	sgroupHost1 := &pb.Host{
		Name:   "host1",
		Uuid:   "uuid1",
		SgName: "sg1",
		IpList: &pb.IPList{
			IPs: []string{"192.168.1.10"},
		},
	}

	// Set up mocks
	hostReader.On("GetHostByUUID", mock.Anything, "uuid1").
		Return(&host1, nil)

	sgroupReader.On("GetHostsByUUIDs", mock.Anything, []string{"uuid1"}).
		Return([]*pb.Host{sgroupHost1}, nil)

	expectedUpdate := []types.HostIPSetUpdate{
		{
			HostUUID:  "uuid1",
			HostID:    host1.GetID(),
			Namespace: "default",
			Name:      "host1",
			IPSet:     []string{"192.168.1.10"},
			SGName:    "sg1",
		},
	}

	hostWriter.On("UpdateHostsIPSet", mock.Anything, expectedUpdate).
		Return(nil)

	ctx := context.Background()
	result, err := synchronizer.SyncHostsByUUIDs(ctx, []string{"uuid1"})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.TotalRequested)
	assert.Equal(t, 1, result.TotalSynced)
	assert.Equal(t, 0, result.TotalFailed)
	assert.Equal(t, "by_uuids", result.GetDetail("sync_type"))

	hostReader.AssertExpectations(t)
	sgroupReader.AssertExpectations(t)
	hostWriter.AssertExpectations(t)
}

func TestHostSynchronizer_SyncAllHosts(t *testing.T) {
	hostReader := &MockHostReader{}
	hostWriter := &MockHostWriter{}
	sgroupReader := &MockSGROUPHostReader{}
	config := DefaultHostSyncConfig()

	synchronizer := NewHostSynchronizer(hostReader, hostWriter, sgroupReader, config)

	// Create test hosts from different namespaces
	host1 := models.Host{}
	host1.Name = "host1"
	host1.UUID = "uuid1"
	host1.Namespace = "default"

	host2 := models.Host{}
	host2.Name = "host2"
	host2.UUID = "uuid2"
	host2.Namespace = "kube-system"

	hosts := []models.Host{host1, host2}

	// SGROUP hosts
	sgroupHost1 := &pb.Host{
		Name:   "host1",
		Uuid:   "uuid1",
		IpList: &pb.IPList{IPs: []string{"192.168.1.10"}},
	}

	sgroupHost2 := &pb.Host{
		Name:   "host2",
		Uuid:   "uuid2",
		IpList: &pb.IPList{IPs: []string{"192.168.1.20"}},
	}

	// Set up mocks
	hostReader.On("GetHostsWithoutIPSet", mock.Anything, ""). // Empty namespace = all
									Return(hosts, nil)

	sgroupReader.On("GetHostsByUUIDs", mock.Anything, []string{"uuid1", "uuid2"}).
		Return([]*pb.Host{sgroupHost1, sgroupHost2}, nil)

	hostWriter.On("UpdateHostsIPSet", mock.Anything, mock.AnythingOfType("[]types.HostIPSetUpdate")).
		Return(nil)

	ctx := context.Background()
	result, err := synchronizer.SyncAllHosts(ctx)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.TotalRequested)
	assert.Equal(t, 2, result.TotalSynced)
	assert.Equal(t, 0, result.TotalFailed)
	assert.Equal(t, "full_sync", result.GetDetail("sync_type"))

	hostReader.AssertExpectations(t)
	sgroupReader.AssertExpectations(t)
	hostWriter.AssertExpectations(t)
}

func TestHostSynchronizer_isValidIP(t *testing.T) {
	hostReader := &MockHostReader{}
	hostWriter := &MockHostWriter{}
	sgroupReader := &MockSGROUPHostReader{}
	config := DefaultHostSyncConfig()

	synchronizer := NewHostSynchronizer(hostReader, hostWriter, sgroupReader, config).(*hostSynchronizer)

	// Test valid IPs
	assert.True(t, synchronizer.isValidIP("192.168.1.1"))
	assert.True(t, synchronizer.isValidIP("10.0.0.1"))
	assert.True(t, synchronizer.isValidIP("::1"))
	assert.True(t, synchronizer.isValidIP("2001:db8::1"))

	// Test invalid IPs
	assert.False(t, synchronizer.isValidIP("invalid"))
	assert.False(t, synchronizer.isValidIP("256.256.256.256"))
	assert.False(t, synchronizer.isValidIP(""))
	assert.False(t, synchronizer.isValidIP("192.168.1"))
}

func TestHostSynchronizer_createBatches(t *testing.T) {
	hostReader := &MockHostReader{}
	hostWriter := &MockHostWriter{}
	sgroupReader := &MockSGROUPHostReader{}
	config := DefaultHostSyncConfig()

	synchronizer := NewHostSynchronizer(hostReader, hostWriter, sgroupReader, config).(*hostSynchronizer)

	// Test normal batching
	uuids := []string{"uuid1", "uuid2", "uuid3", "uuid4", "uuid5"}
	batches := synchronizer.createBatches(uuids, 2)

	assert.Len(t, batches, 3)
	assert.Equal(t, []string{"uuid1", "uuid2"}, batches[0])
	assert.Equal(t, []string{"uuid3", "uuid4"}, batches[1])
	assert.Equal(t, []string{"uuid5"}, batches[2])

	// Test edge cases
	emptyBatches := synchronizer.createBatches([]string{}, 5)
	assert.Len(t, emptyBatches, 0)

	singleBatch := synchronizer.createBatches([]string{"uuid1"}, 5)
	assert.Len(t, singleBatch, 1)
	assert.Equal(t, []string{"uuid1"}, singleBatch[0])

	// Test zero batch size (should use default)
	zeroBatches := synchronizer.createBatches([]string{"uuid1", "uuid2"}, 0)
	assert.Len(t, zeroBatches, 1) // Should use default batch size
}
