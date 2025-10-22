package syncers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// SvcSvcRuleSyncer implements EntitySyncer for SvcSvcRule entities
type SvcSvcRuleSyncer struct {
	gateway interfaces.SGroupGateway
	logger  logr.Logger
}

// NewSvcSvcRuleSyncer creates a new SvcSvcRule syncer
func NewSvcSvcRuleSyncer(gateway interfaces.SGroupGateway, logger logr.Logger) *SvcSvcRuleSyncer {
	return &SvcSvcRuleSyncer{
		gateway: gateway,
		logger:  logger,
	}
}

// Sync synchronizes a single SvcSvcRule entity
func (s *SvcSvcRuleSyncer) Sync(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	if entity == nil {
		return fmt.Errorf("entity cannot be nil")
	}

	// Validate entity type
	if entity.GetSyncSubjectType() != types.SyncSubjectTypeSvcSvcRules {
		return fmt.Errorf("invalid entity type for SvcSvcRuleSyncer: %s", entity.GetSyncSubjectType())
	}

	// Convert entity to single protobuf svc-svc rule
	protoData, err := entity.ToSGroupsProto()
	if err != nil {
		return fmt.Errorf("failed to convert entity to sgroups proto: %w", err)
	}

	// Cast to *pb.SvcSvcRule and wrap in SyncSvcSvcRules for single entity
	protoRule, ok := protoData.(*pb.SvcSvcRule)
	if !ok {
		return fmt.Errorf("invalid proto data type for entity %s, expected *pb.SvcSvcRule, got %T", entity.GetSyncKey(), protoData)
	}

	// Create single-entity batch structure for backward compatibility
	singleEntityBatch := &pb.SyncSvcSvcRules{
		Rules: []*pb.SvcSvcRule{protoRule},
	}

	// Create sync request
	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeSvcSvcRules,
		Data:        singleEntityBatch, // Send single-entity batch structure
	}

	// Send sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync SvcSvcRule with sgroups: %w", err)
	}

	s.logger.V(1).Info("Successfully synced SvcSvcRule",
		"key", entity.GetSyncKey(),
		"operation", operation)

	return nil
}

// SyncBatch synchronizes multiple SvcSvcRule entities in a batch
func (s *SvcSvcRuleSyncer) SyncBatch(ctx context.Context, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	if len(entities) == 0 {
		return nil
	}

	// Validate all entities and convert to protobuf svc-svc rules
	var protoRules []*pb.SvcSvcRule
	entityKeys := make([]string, 0, len(entities))

	for _, entity := range entities {
		if entity == nil {
			continue
		}

		if entity.GetSyncSubjectType() != types.SyncSubjectTypeSvcSvcRules {
			return fmt.Errorf("invalid entity type for SvcSvcRuleSyncer: %s", entity.GetSyncSubjectType())
		}

		// Convert entity to single protobuf svc-svc rule
		protoData, err := entity.ToSGroupsProto()
		if err != nil {
			return fmt.Errorf("failed to convert entity %s to sgroups proto: %w", entity.GetSyncKey(), err)
		}

		// Cast to *pb.SvcSvcRule
		if protoRule, ok := protoData.(*pb.SvcSvcRule); ok {
			protoRules = append(protoRules, protoRule)
			entityKeys = append(entityKeys, entity.GetSyncKey())
		} else {
			return fmt.Errorf("invalid proto data type for entity %s, expected *pb.SvcSvcRule, got %T", entity.GetSyncKey(), protoData)
		}
	}

	if len(protoRules) == 0 {
		return nil
	}

	// Create aggregated batch sync request
	batchProtoData := &pb.SyncSvcSvcRules{
		Rules: protoRules,
	}

	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeSvcSvcRules,
		Data:        batchProtoData, // Send aggregated structure
	}

	// Send batch sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync SvcSvcRule batch with sgroups: %w", err)
	}

	s.logger.Info("Successfully synced SvcSvcRule batch",
		"count", len(protoRules),
		"operation", operation,
		"keys", entityKeys)

	return nil
}

// GetSupportedSubjectType returns the subject type this syncer supports
func (s *SvcSvcRuleSyncer) GetSupportedSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeSvcSvcRules
}
