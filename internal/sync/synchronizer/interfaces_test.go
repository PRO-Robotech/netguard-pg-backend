package synchronizer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/sync/types"
)

// MockHostReader implements HostReader interface for testing
type MockHostReader struct {
	mock.Mock
}

func (m *MockHostReader) GetHostsWithoutIPSet(ctx context.Context, namespace string) ([]models.Host, error) {
	args := m.Called(ctx, namespace)
	return args.Get(0).([]models.Host), args.Error(1)
}

func (m *MockHostReader) GetHostByUUID(ctx context.Context, uuid string) (*models.Host, error) {
	args := m.Called(ctx, uuid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Host), args.Error(1)
}

func (m *MockHostReader) ListHosts(ctx context.Context, identifiers []HostIdentifier) ([]models.Host, error) {
	args := m.Called(ctx, identifiers)
	return args.Get(0).([]models.Host), args.Error(1)
}

// MockHostWriter implements HostWriter interface for testing
type MockHostWriter struct {
	mock.Mock
}

func (m *MockHostWriter) UpdateHostIPSet(ctx context.Context, hostID string, ipSet []string) error {
	args := m.Called(ctx, hostID, ipSet)
	return args.Error(0)
}

func (m *MockHostWriter) UpdateHostsIPSet(ctx context.Context, updates []types.HostIPSetUpdate) error {
	args := m.Called(ctx, updates)
	return args.Error(0)
}

// MockSGROUPHostReader implements SGROUPHostReader interface for testing
type MockSGROUPHostReader struct {
	mock.Mock
}

func (m *MockSGROUPHostReader) GetHostsByUUIDs(ctx context.Context, uuids []string) ([]*pb.Host, error) {
	args := m.Called(ctx, uuids)
	return args.Get(0).([]*pb.Host), args.Error(1)
}

func (m *MockSGROUPHostReader) ListAllHosts(ctx context.Context) ([]*pb.Host, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*pb.Host), args.Error(1)
}

func (m *MockSGROUPHostReader) GetHostsInSecurityGroup(ctx context.Context, sgNames []string) ([]*pb.Host, error) {
	args := m.Called(ctx, sgNames)
	return args.Get(0).([]*pb.Host), args.Error(1)
}

// Test interface compliance
func TestInterfaceCompliance(t *testing.T) {
	// Test that our mocks implement the interfaces
	var hostReader HostReader = &MockHostReader{}
	var hostWriter HostWriter = &MockHostWriter{}
	var sgroupReader SGROUPHostReader = &MockSGROUPHostReader{}

	assert.NotNil(t, hostReader)
	assert.NotNil(t, hostWriter)
	assert.NotNil(t, sgroupReader)
}

func TestHostIdentifier(t *testing.T) {
	identifier := HostIdentifier{
		Namespace: "default",
		Name:      "test-host",
	}

	assert.Equal(t, "default", identifier.Namespace)
	assert.Equal(t, "test-host", identifier.Name)
}

func TestHostSyncConfig(t *testing.T) {
	// Test default config
	config := DefaultHostSyncConfig()

	assert.Equal(t, 50, config.BatchSize)
	assert.Equal(t, 5, config.MaxConcurrency)
	assert.Equal(t, 30, config.SyncTimeout)
	assert.Equal(t, 3, config.RetryAttempts)
	assert.True(t, config.EnableIPSetValidation)

	// Test custom config
	customConfig := HostSyncConfig{
		BatchSize:             100,
		MaxConcurrency:        10,
		SyncTimeout:           60,
		RetryAttempts:         5,
		EnableIPSetValidation: false,
	}

	assert.Equal(t, 100, customConfig.BatchSize)
	assert.Equal(t, 10, customConfig.MaxConcurrency)
	assert.Equal(t, 60, customConfig.SyncTimeout)
	assert.Equal(t, 5, customConfig.RetryAttempts)
	assert.False(t, customConfig.EnableIPSetValidation)
}
