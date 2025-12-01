package syncers

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"
)

// MockSGroupGateway is a mock implementation of SGroupGateway
type MockSGroupGateway struct {
	mock.Mock
}

func (m *MockSGroupGateway) Sync(ctx context.Context, req *types.SyncRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockSGroupGateway) Health(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockSGroupGateway) GetStatuses(ctx context.Context) (chan *timestamppb.Timestamp, error) {
	args := m.Called(ctx)
	return args.Get(0).(chan *timestamppb.Timestamp), args.Error(1)
}

func (m *MockSGroupGateway) GetHostsByUUIDs(ctx context.Context, uuids []string) ([]*pb.Host, error) {
	args := m.Called(ctx, uuids)
	return args.Get(0).([]*pb.Host), args.Error(1)
}

func (m *MockSGroupGateway) ListAllHosts(ctx context.Context) ([]*pb.Host, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*pb.Host), args.Error(1)
}

func (m *MockSGroupGateway) GetHostsInSecurityGroup(ctx context.Context, sgNames []string) ([]*pb.Host, error) {
	args := m.Called(ctx, sgNames)
	return args.Get(0).([]*pb.Host), args.Error(1)
}

func (m *MockSGroupGateway) Close() error {
	args := m.Called()
	return args.Error(0)
}

// MockSyncableEntity is a mock implementation of SyncableEntity
type MockSyncableEntity struct {
	mock.Mock
}

func (m *MockSyncableEntity) GetSyncSubjectType() types.SyncSubjectType {
	args := m.Called()
	return args.Get(0).(types.SyncSubjectType)
}

func (m *MockSyncableEntity) ToSGroupsProto() (interface{}, error) {
	args := m.Called()
	return args.Get(0), args.Error(1)
}

func (m *MockSyncableEntity) GetSyncKey() string {
	args := m.Called()
	return args.String(0)
}

func TestAddressGroupSyncer_Sync(t *testing.T) {
	tests := []struct {
		name          string
		entity        interfaces.SyncableEntity
		operation     types.SyncOperation
		setupMocks    func(*MockSGroupGateway, interfaces.SyncableEntity)
		expectedError bool
	}{
		{
			name:      "successful sync",
			entity:    &MockSyncableEntity{}, // Добавляем entity
			operation: types.SyncOperationUpsert,
			setupMocks: func(gateway *MockSGroupGateway, entity interfaces.SyncableEntity) {
				gateway.On("Sync", mock.Anything, mock.MatchedBy(func(req *types.SyncRequest) bool {
					return req.Operation == types.SyncOperationUpsert &&
						req.SubjectType == types.SyncSubjectTypeGroups
				})).Return(nil)
			},
			expectedError: false,
		},
		{
			name:      "nil entity",
			entity:    nil,
			operation: types.SyncOperationUpsert,
			setupMocks: func(gateway *MockSGroupGateway, entity interfaces.SyncableEntity) {
				// No setup needed for nil entity test
			},
			expectedError: true,
		},
		{
			name:      "wrong entity type",
			entity:    &MockSyncableEntity{}, // Добавляем entity
			operation: types.SyncOperationUpsert,
			setupMocks: func(gateway *MockSGroupGateway, entity interfaces.SyncableEntity) {
				// No gateway calls expected for wrong entity type
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockGateway := &MockSGroupGateway{}
			logger := logr.Discard()
			syncer := NewAddressGroupSyncer(mockGateway, logger)

			var entity interfaces.SyncableEntity
			if tt.entity == nil {
				entity = nil
			} else {
				// Use the entity from test case and setup its methods
				if mockEntity, ok := tt.entity.(*MockSyncableEntity); ok {
					if tt.name == "wrong entity type" {
						// Setup for wrong entity type (Services instead of Groups)
						mockEntity.On("GetSyncSubjectType").Return(types.SyncSubjectTypeServices)
						mockEntity.On("GetSyncKey").Return("test-service")
					} else {
						// Setup for correct entity type (Groups)
						mockEntity.On("GetSyncSubjectType").Return(types.SyncSubjectTypeGroups)
						mockEntity.On("GetSyncKey").Return("test-group")
						mockEntity.On("ToSGroupsProto").Return(&pb.SecGroup{Name: "test-group"}, nil)
					}
				}
				entity = tt.entity
			}

			tt.setupMocks(mockGateway, entity)

			// Execute
			err := syncer.Sync(context.Background(), entity, tt.operation)

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockGateway.AssertExpectations(t)
		})
	}
}

func TestAddressGroupSyncer_Sync_UpsertWhenNetworksEmpty(t *testing.T) {
	// Updated test: AddressGroupSyncer now only performs a single upsert operation
	// regardless of networks state (hosts are no longer sent to SGROUP)
	mockGateway := &MockSGroupGateway{}
	logger := logr.Discard()
	syncer := NewAddressGroupSyncer(mockGateway, logger)

	ag := &models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "ag-clear",
				Namespace: "incloud-sgroups",
			},
		},
		DefaultAction: models.ActionAccept,
		Networks:      []models.NetworkItem{},
	}

	upsertMatcher := mock.MatchedBy(func(req *types.SyncRequest) bool {
		batch, ok := req.Data.(*pb.SyncSecurityGroups)
		if !ok || len(batch.Groups) != 1 {
			return false
		}
		grp := batch.Groups[0]
		return req.Operation == types.SyncOperationUpsert &&
			req.SubjectType == types.SyncSubjectTypeGroups &&
			grp.GetName() == "incloud-sgroups/ag-clear" &&
			len(grp.GetNetworks()) == 0
	})

	mockGateway.On("Sync", mock.Anything, upsertMatcher).Return(nil).Once()

	err := syncer.Sync(context.Background(), ag, types.SyncOperationUpsert)
	assert.NoError(t, err)
	mockGateway.AssertExpectations(t)
}

func TestAddressGroupSyncer_Sync_UpsertWhenNetworksPresent(t *testing.T) {
	mockGateway := &MockSGroupGateway{}
	logger := logr.Discard()
	syncer := NewAddressGroupSyncer(mockGateway, logger)

	ag := &models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "ag-with-net",
				Namespace: "incloud-sgroups",
			},
		},
		DefaultAction: models.ActionAccept,
		Networks: []models.NetworkItem{
			{
				Name:      "net-1",
				Namespace: "incloud-sgroups",
				CIDR:      "10.0.0.0/24",
			},
		},
	}

	upsertMatcher := mock.MatchedBy(func(req *types.SyncRequest) bool {
		batch, ok := req.Data.(*pb.SyncSecurityGroups)
		if !ok || len(batch.Groups) != 1 {
			return false
		}
		grp := batch.Groups[0]
		return req.Operation == types.SyncOperationUpsert &&
			req.SubjectType == types.SyncSubjectTypeGroups &&
			len(grp.GetNetworks()) == 1 &&
			grp.GetNetworks()[0] == "incloud-sgroups/net-1"
	})

	mockGateway.On("Sync", mock.Anything, upsertMatcher).Return(nil).Once()

	err := syncer.Sync(context.Background(), ag, types.SyncOperationUpsert)
	assert.NoError(t, err)
	mockGateway.AssertExpectations(t)
}

func TestAddressGroupSyncer_SyncBatch(t *testing.T) {
	tests := []struct {
		name          string
		entities      []interfaces.SyncableEntity
		operation     types.SyncOperation
		setupMocks    func(*MockSGroupGateway)
		expectedError bool
	}{
		{
			name:      "successful batch sync",
			operation: types.SyncOperationUpsert,
			setupMocks: func(gateway *MockSGroupGateway) {
				gateway.On("Sync", mock.Anything, mock.MatchedBy(func(req *types.SyncRequest) bool {
					return req.Operation == types.SyncOperationUpsert &&
						req.SubjectType == types.SyncSubjectTypeGroups
				})).Return(nil)
			},
			expectedError: false,
		},
		{
			name:      "empty entities",
			entities:  []interfaces.SyncableEntity{},
			operation: types.SyncOperationUpsert,
			setupMocks: func(gateway *MockSGroupGateway) {
				// No calls expected for empty entities
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockGateway := &MockSGroupGateway{}
			logger := logr.Discard()
			syncer := NewAddressGroupSyncer(mockGateway, logger)

			entities := tt.entities
			if tt.name == "successful batch sync" {
				// Create mock test entities
				mockEntity1 := &MockSyncableEntity{}
				mockEntity1.On("GetSyncSubjectType").Return(types.SyncSubjectTypeGroups)
				mockEntity1.On("GetSyncKey").Return("test-group-1")
				mockEntity1.On("ToSGroupsProto").Return(&pb.SecGroup{Name: "test-group-1"}, nil)

				mockEntity2 := &MockSyncableEntity{}
				mockEntity2.On("GetSyncSubjectType").Return(types.SyncSubjectTypeGroups)
				mockEntity2.On("GetSyncKey").Return("test-group-2")
				mockEntity2.On("ToSGroupsProto").Return(&pb.SecGroup{Name: "test-group-2"}, nil)

				entities = []interfaces.SyncableEntity{mockEntity1, mockEntity2}
			}

			tt.setupMocks(mockGateway)

			// Execute
			err := syncer.SyncBatch(context.Background(), entities, tt.operation)

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockGateway.AssertExpectations(t)
		})
	}
}

func TestAddressGroupSyncer_GetSupportedSubjectType(t *testing.T) {
	mockGateway := &MockSGroupGateway{}
	logger := logr.Discard()
	syncer := NewAddressGroupSyncer(mockGateway, logger)

	subjectType := syncer.GetSupportedSubjectType()

	assert.Equal(t, types.SyncSubjectTypeGroups, subjectType)
}
