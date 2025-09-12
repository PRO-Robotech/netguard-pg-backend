package syncers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// HostSyncer implements EntitySyncer for Host entities
type HostSyncer struct {
	gateway interfaces.SGroupGateway
	logger  logr.Logger
}

// NewHostSyncer creates a new Host syncer
func NewHostSyncer(gateway interfaces.SGroupGateway, logger logr.Logger) *HostSyncer {
	return &HostSyncer{
		gateway: gateway,
		logger:  logger,
	}
}

// Sync synchronizes a single Host entity
func (s *HostSyncer) Sync(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	if entity == nil {
		return fmt.Errorf("entity cannot be nil")
	}

	fmt.Printf("🔧 DEBUG: HostSyncer.Sync - Starting sync for entity %s (operation: %s)\n", entity.GetSyncKey(), operation)

	// Validate entity type
	if entity.GetSyncSubjectType() != types.SyncSubjectTypeHosts {
		return fmt.Errorf("invalid entity type for HostSyncer: %s", entity.GetSyncSubjectType())
	}

	// Convert entity to single protobuf host
	protoData, err := entity.ToSGroupsProto()
	if err != nil {
		fmt.Printf("❌ ERROR: HostSyncer.Sync - Failed to convert entity %s to proto: %v\n", entity.GetSyncKey(), err)
		return fmt.Errorf("failed to convert entity to sgroups proto: %w", err)
	}

	// Cast to *pb.Host and wrap in SyncHosts for single entity
	protoHost, ok := protoData.(*pb.Host)
	if !ok {
		return fmt.Errorf("invalid proto data type for entity %s, expected *pb.Host, got %T", entity.GetSyncKey(), protoData)
	}

	// Create single-entity batch structure for backward compatibility
	singleEntityBatch := &pb.SyncHosts{
		Hosts: []*pb.Host{protoHost},
	}

	fmt.Printf("🔧 DEBUG: HostSyncer.Sync - Converted entity %s to proto and wrapped in batch structure\n", entity.GetSyncKey())

	// Create sync request
	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeHosts,
		Data:        singleEntityBatch, // Send single-entity batch structure
	}

	fmt.Printf("🔧 DEBUG: HostSyncer.Sync - Sending sync request to gateway for entity %s\n", entity.GetSyncKey())

	// Send sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		fmt.Printf("❌ ERROR: HostSyncer.Sync - Gateway sync failed for entity %s: %v\n", entity.GetSyncKey(), err)
		return fmt.Errorf("failed to sync Host with sgroups: %w", err)
	}

	fmt.Printf("✅ DEBUG: HostSyncer.Sync - Successfully completed sync for entity %s\n", entity.GetSyncKey())

	s.logger.V(1).Info("Successfully synced Host",
		"key", entity.GetSyncKey(),
		"operation", operation)

	return nil
}

// SyncBatch synchronizes multiple Host entities in a batch
func (s *HostSyncer) SyncBatch(ctx context.Context, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	if len(entities) == 0 {
		return nil
	}

	// Validate all entities and convert to protobuf hosts
	var protoHosts []*pb.Host
	entityKeys := make([]string, 0, len(entities))

	for _, entity := range entities {
		if entity == nil {
			continue
		}

		if entity.GetSyncSubjectType() != types.SyncSubjectTypeHosts {
			return fmt.Errorf("invalid entity type for HostSyncer: %s", entity.GetSyncSubjectType())
		}

		// Convert entity to single protobuf host
		protoData, err := entity.ToSGroupsProto()
		if err != nil {
			return fmt.Errorf("failed to convert entity %s to sgroups proto: %w", entity.GetSyncKey(), err)
		}

		// Cast to *pb.Host
		if protoHost, ok := protoData.(*pb.Host); ok {
			protoHosts = append(protoHosts, protoHost)
			entityKeys = append(entityKeys, entity.GetSyncKey())
		} else {
			return fmt.Errorf("invalid proto data type for entity %s, expected *pb.Host, got %T", entity.GetSyncKey(), protoData)
		}
	}

	if len(protoHosts) == 0 {
		return nil
	}

	// Create aggregated batch sync request
	batchProtoData := &pb.SyncHosts{
		Hosts: protoHosts,
	}

	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeHosts,
		Data:        batchProtoData, // Send aggregated structure
	}

	// Send batch sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync Host batch with sgroups: %w", err)
	}

	s.logger.Info("Successfully synced Host batch",
		"count", len(protoHosts),
		"operation", operation,
		"keys", entityKeys)

	return nil
}

// GetSupportedSubjectType returns the subject type this syncer supports
func (s *HostSyncer) GetSupportedSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeHosts
}
