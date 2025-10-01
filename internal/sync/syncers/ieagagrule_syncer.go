package syncers

import (
	"context"
	"fmt"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"
	"github.com/go-logr/logr"

	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// IEAgAgRuleSyncer implements EntitySyncer for IEAgAgRule entities
type IEAgAgRuleSyncer struct {
	gateway interfaces.SGroupGateway
	logger  logr.Logger
}

// NewIEAgAgRuleSyncer creates a new IEAgAgRule syncer
func NewIEAgAgRuleSyncer(gateway interfaces.SGroupGateway, logger logr.Logger) *IEAgAgRuleSyncer {
	return &IEAgAgRuleSyncer{
		gateway: gateway,
		logger:  logger,
	}
}

// Sync synchronizes a single IEAgAgRule entity
func (s *IEAgAgRuleSyncer) Sync(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	if entity == nil {
		return fmt.Errorf("entity cannot be nil")
	}

	// Validate entity type
	if entity.GetSyncSubjectType() != types.SyncSubjectTypeIEAgAgRules {
		return fmt.Errorf("invalid entity type for IEAgAgRuleSyncer: %s", entity.GetSyncSubjectType())
	}

	// Convert entity to single protobuf rule
	protoData, err := entity.ToSGroupsProto()
	if err != nil {
		return fmt.Errorf("failed to convert entity to sgroups proto: %w", err)
	}

	// Cast to *pb.IESgSgRule and wrap in SyncIESgSgRules for single entity
	protoRule, ok := protoData.(*pb.IESgSgRule)
	if !ok {
		return fmt.Errorf("invalid proto data type for entity %s, expected *pb.IESgSgRule, got %T", entity.GetSyncKey(), protoData)
	}

	// Create single-entity batch structure for backward compatibility
	singleEntityBatch := &pb.SyncIESgSgRules{
		Rules: []*pb.IESgSgRule{protoRule},
	}

	// Create sync request
	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeIEAgAgRules,
		Data:        singleEntityBatch, // Send single-entity batch structure
	}

	// Send sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync entity with sgroups: %w", err)
	}

	s.logger.V(1).Info("Successfully synced IEAgAgRule", "key", entity.GetSyncKey(), "operation", operation)

	return nil
}

// SyncBatch synchronizes multiple IEAgAgRule entities in a batch
func (s *IEAgAgRuleSyncer) SyncBatch(ctx context.Context, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	if len(entities) == 0 {
		return nil
	}

	// Validate all entities and convert to protobuf rules
	var protoRules []*pb.IESgSgRule
	entityKeys := make([]string, 0, len(entities))

	for _, entity := range entities {
		if entity == nil {
			continue
		}

		// Validate entity type
		if entity.GetSyncSubjectType() != types.SyncSubjectTypeIEAgAgRules {
			return fmt.Errorf("invalid entity type for IEAgAgRuleSyncer: %s", entity.GetSyncSubjectType())
		}

		// Convert entity to single protobuf rule
		protoData, err := entity.ToSGroupsProto()
		if err != nil {
			return fmt.Errorf("failed to convert entity %s to sgroups proto: %w", entity.GetSyncKey(), err)
		}

		// Cast to *pb.IESgSgRule
		if protoRule, ok := protoData.(*pb.IESgSgRule); ok {
			protoRules = append(protoRules, protoRule)
			entityKeys = append(entityKeys, entity.GetSyncKey())
		} else {
			return fmt.Errorf("invalid proto data type for entity %s, expected *pb.IESgSgRule, got %T", entity.GetSyncKey(), protoData)
		}
	}

	if len(protoRules) == 0 {
		return nil
	}

	// Create aggregated batch sync request
	batchProtoData := &pb.SyncIESgSgRules{
		Rules: protoRules,
	}

	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeIEAgAgRules,
		Data:        batchProtoData, // Send aggregated structure
	}

	// Send batch sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync IEAgAgRule batch with sgroups: %w", err)
	}

	s.logger.V(1).Info("Successfully synced batch of IEAgAgRules", "count", len(entities), "keys", entityKeys, "operation", operation)

	return nil
}

// GetSupportedSubjectType returns the subject type this syncer supports
func (s *IEAgAgRuleSyncer) GetSupportedSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeIEAgAgRules
}
