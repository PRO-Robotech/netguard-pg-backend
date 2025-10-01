package syncers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// AddressGroupSyncer implements EntitySyncer for AddressGroup entities
type AddressGroupSyncer struct {
	gateway interfaces.SGroupGateway
	logger  logr.Logger
}

// NewAddressGroupSyncer creates a new AddressGroup syncer
func NewAddressGroupSyncer(gateway interfaces.SGroupGateway, logger logr.Logger) *AddressGroupSyncer {
	return &AddressGroupSyncer{
		gateway: gateway,
		logger:  logger,
	}
}

// Sync synchronizes a single AddressGroup entity
func (s *AddressGroupSyncer) Sync(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	if entity == nil {
		return fmt.Errorf("entity cannot be nil")
	}

	// Validate entity type
	if entity.GetSyncSubjectType() != types.SyncSubjectTypeGroups {
		return fmt.Errorf("invalid entity type for AddressGroupSyncer: %s", entity.GetSyncSubjectType())
	}

	// Convert entity to single protobuf group
	protoData, err := entity.ToSGroupsProto()
	if err != nil {
		return fmt.Errorf("failed to convert entity to sgroups proto: %w", err)
	}

	// Cast to *pb.SecGroup and wrap in SyncSecurityGroups for single entity
	protoGroup, ok := protoData.(*pb.SecGroup)
	if !ok {
		return fmt.Errorf("invalid proto data type for entity %s, expected *pb.SecGroup, got %T", entity.GetSyncKey(), protoData)
	}

	// Create single-entity batch structure for backward compatibility
	singleEntityBatch := &pb.SyncSecurityGroups{
		Groups: []*pb.SecGroup{protoGroup},
	}

	// Create sync request
	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeGroups,
		Data:        singleEntityBatch, // Send single-entity batch structure
	}

	// Send sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync AddressGroup with sgroups: %w", err)
	}

	s.logger.V(1).Info("Successfully synced AddressGroup",
		"key", entity.GetSyncKey(),
		"operation", operation)

	return nil
}

// SyncBatch synchronizes multiple AddressGroup entities in a batch
func (s *AddressGroupSyncer) SyncBatch(ctx context.Context, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	if len(entities) == 0 {
		return nil
	}

	// Validate all entities and convert to protobuf groups
	var protoGroups []*pb.SecGroup
	entityKeys := make([]string, 0, len(entities))

	for _, entity := range entities {
		if entity == nil {
			continue
		}

		// Validate entity type
		if entity.GetSyncSubjectType() != types.SyncSubjectTypeGroups {
			return fmt.Errorf("invalid entity type for AddressGroupSyncer: %s", entity.GetSyncSubjectType())
		}

		// Convert entity to single protobuf group
		protoData, err := entity.ToSGroupsProto()
		if err != nil {
			return fmt.Errorf("failed to convert entity %s to sgroups proto: %w", entity.GetSyncKey(), err)
		}

		// Cast to *pb.SecGroup
		if protoGroup, ok := protoData.(*pb.SecGroup); ok {
			protoGroups = append(protoGroups, protoGroup)
			entityKeys = append(entityKeys, entity.GetSyncKey())
		} else {
			return fmt.Errorf("invalid proto data type for entity %s, expected *pb.SecGroup, got %T", entity.GetSyncKey(), protoData)
		}
	}

	if len(protoGroups) == 0 {
		return nil
	}

	// Create aggregated batch sync request
	batchProtoData := &pb.SyncSecurityGroups{
		Groups: protoGroups,
	}

	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeGroups,
		Data:        batchProtoData, // Send aggregated structure
	}

	// Send batch sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync AddressGroup batch with sgroups: %w", err)
	}

	s.logger.Info("Successfully synced AddressGroup batch",
		"count", len(protoGroups),
		"operation", operation,
		"keys", entityKeys)

	return nil
}

// GetSupportedSubjectType returns the subject type this syncer supports
func (s *AddressGroupSyncer) GetSupportedSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeGroups
}
