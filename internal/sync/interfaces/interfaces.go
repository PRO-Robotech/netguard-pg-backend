package interfaces

import (
	"context"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"
	"google.golang.org/protobuf/types/known/timestamppb"

	"netguard-pg-backend/internal/sync/types"
)

// SyncableEntity defines an entity that can be synchronized with sgroups
type SyncableEntity interface {
	// GetSyncSubjectType returns the type of entity for synchronization
	GetSyncSubjectType() types.SyncSubjectType

	// ToSGroupsProto converts the entity to sgroups protobuf format
	ToSGroupsProto() (interface{}, error)

	// GetSyncKey returns a unique key for the entity
	GetSyncKey() string
}

// EntitySyncer defines a syncer for a specific entity type
type EntitySyncer[T SyncableEntity] interface {
	// Sync synchronizes a single entity
	Sync(ctx context.Context, entity T, operation types.SyncOperation) error

	// SyncBatch synchronizes multiple entities in a batch
	SyncBatch(ctx context.Context, entities []T, operation types.SyncOperation) error

	// GetSupportedSubjectType returns the subject type this syncer supports
	GetSupportedSubjectType() types.SyncSubjectType
}

// SyncManager manages all entity synchronizers
type SyncManager interface {
	// RegisterSyncer registers a syncer for a specific subject type
	RegisterSyncer(subjectType types.SyncSubjectType, syncer interface{}) error

	// SyncEntity synchronizes a single entity using the appropriate syncer
	SyncEntity(ctx context.Context, entity SyncableEntity, operation types.SyncOperation) error

	// SyncEntityForced synchronizes a single entity bypassing debouncing
	SyncEntityForced(ctx context.Context, entity SyncableEntity, operation types.SyncOperation) error

	// SyncBatch synchronizes multiple entities in a batch
	SyncBatch(ctx context.Context, entities []SyncableEntity, operation types.SyncOperation) error

	// Start starts the sync manager background processes
	Start(ctx context.Context) error

	// Stop stops the sync manager
	Stop() error
}

// SGroupGateway defines the interface for communicating with sgroups service
type SGroupGateway interface {
	// Sync sends a synchronization request to sgroups
	Sync(ctx context.Context, req *types.SyncRequest) error

	// Health checks the health of sgroups service
	Health(ctx context.Context) error

	// GetStatuses returns a channel of timestamp updates from SGROUP
	GetStatuses(ctx context.Context) (chan *timestamppb.Timestamp, error)

	// Close closes the gateway connection
	Close() error

	// Host operations for reverse synchronization
	// GetHostsByUUIDs retrieves hosts from SGROUP by their UUIDs
	GetHostsByUUIDs(ctx context.Context, uuids []string) ([]*pb.Host, error)

	// ListAllHosts retrieves all hosts from SGROUP (for full sync)
	ListAllHosts(ctx context.Context) ([]*pb.Host, error)

	// GetHostsInSecurityGroup retrieves hosts from SGROUP that belong to specific security groups
	GetHostsInSecurityGroup(ctx context.Context, sgNames []string) ([]*pb.Host, error)
}

// RetryConfig defines retry configuration for synchronization
type RetryConfig struct {
	MaxRetries    int
	InitialDelay  int // milliseconds
	MaxDelay      int // milliseconds
	BackoffFactor float64
}

// SyncTracker tracks synchronization statistics and provides debouncing
type SyncTracker interface {
	// Track records a sync operation
	Track(subjectType types.SyncSubjectType, operation types.SyncOperation, success bool)

	// GetStats returns synchronization statistics
	GetStats() map[types.SyncSubjectType]SyncStats

	// ShouldSync determines if an entity should be synchronized (debouncing)
	ShouldSync(key string, operation types.SyncOperation) bool

	// ShouldSyncForced forces sync regardless of debouncing
	ShouldSyncForced(key string, operation types.SyncOperation) bool
}

// SyncStats represents synchronization statistics for a subject type
type SyncStats struct {
	TotalRequests   int64
	SuccessfulSyncs int64
	FailedSyncs     int64
	LastSyncTime    int64 // Unix timestamp
	AverageLatency  int64 // milliseconds
}
