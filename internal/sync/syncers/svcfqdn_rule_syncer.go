package syncers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// SvcFqdnRuleSyncer implements EntitySyncer for SvcFqdnRule entities.
type SvcFqdnRuleSyncer struct {
	gateway interfaces.SGroupGateway
	logger  logr.Logger
}

// NewSvcFqdnRuleSyncer constructs a syncer for service-to-FQDN rules.
func NewSvcFqdnRuleSyncer(gateway interfaces.SGroupGateway, logger logr.Logger) *SvcFqdnRuleSyncer {
	return &SvcFqdnRuleSyncer{
		gateway: gateway,
		logger:  logger,
	}
}

// Sync sends a single SvcFqdnRule to SGROUP.
func (s *SvcFqdnRuleSyncer) Sync(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	if entity == nil {
		return fmt.Errorf("entity cannot be nil")
	}

	if entity.GetSyncSubjectType() != types.SyncSubjectTypeSvcFqdnRules {
		return fmt.Errorf("invalid entity type for SvcFqdnRuleSyncer: %s", entity.GetSyncSubjectType())
	}

	protoData, err := entity.ToSGroupsProto()
	if err != nil {
		return fmt.Errorf("failed to convert entity to sgroups proto: %w", err)
	}

	protoRule, ok := protoData.(*pb.SvcFqdnRule)
	if !ok {
		return fmt.Errorf("invalid proto data type for entity %s, expected *pb.SvcFqdnRule, got %T", entity.GetSyncKey(), protoData)
	}

	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeSvcFqdnRules,
		Data: &pb.SyncSvcFqdnRules{
			Rules: []*pb.SvcFqdnRule{protoRule},
		},
	}

	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync SvcFqdnRule with sgroups: %w", err)
	}

	s.logger.V(1).Info("Successfully synced SvcFqdnRule",
		"key", entity.GetSyncKey(),
		"operation", operation)

	return nil
}

// SyncBatch sends multiple SvcFqdnRules in a single request.
func (s *SvcFqdnRuleSyncer) SyncBatch(ctx context.Context, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	if len(entities) == 0 {
		return nil
	}

	var (
		protoRules []*pb.SvcFqdnRule
		entityKeys = make([]string, 0, len(entities))
	)

	for _, entity := range entities {
		if entity == nil {
			continue
		}

		if entity.GetSyncSubjectType() != types.SyncSubjectTypeSvcFqdnRules {
			return fmt.Errorf("invalid entity type for SvcFqdnRuleSyncer: %s", entity.GetSyncSubjectType())
		}

		protoData, err := entity.ToSGroupsProto()
		if err != nil {
			return fmt.Errorf("failed to convert entity %s to sgroups proto: %w", entity.GetSyncKey(), err)
		}

		protoRule, ok := protoData.(*pb.SvcFqdnRule)
		if !ok {
			return fmt.Errorf("invalid proto data type for entity %s, expected *pb.SvcFqdnRule, got %T", entity.GetSyncKey(), protoData)
		}

		protoRules = append(protoRules, protoRule)
		entityKeys = append(entityKeys, entity.GetSyncKey())
	}

	if len(protoRules) == 0 {
		return nil
	}

	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeSvcFqdnRules,
		Data: &pb.SyncSvcFqdnRules{
			Rules: protoRules,
		},
	}

	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync SvcFqdnRule batch with sgroups: %w", err)
	}

	s.logger.Info("Successfully synced SvcFqdnRule batch",
		"count", len(protoRules),
		"operation", operation,
		"keys", entityKeys)

	return nil
}

// GetSupportedSubjectType reports the subject type handled by this syncer.
func (s *SvcFqdnRuleSyncer) GetSupportedSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeSvcFqdnRules
}



