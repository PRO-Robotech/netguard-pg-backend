package syncers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// IECidrSvcRuleSyncer implements EntitySyncer for IECidrSvcRule entities.
type IECidrSvcRuleSyncer struct {
	gateway interfaces.SGroupGateway
	logger  logr.Logger
}

// NewIECidrSvcRuleSyncer constructs a syncer for CIDR-to-service rules.
func NewIECidrSvcRuleSyncer(gateway interfaces.SGroupGateway, logger logr.Logger) *IECidrSvcRuleSyncer {
	return &IECidrSvcRuleSyncer{
		gateway: gateway,
		logger:  logger,
	}
}

// Sync sends a single IECidrSvcRule to SGROUP.
func (s *IECidrSvcRuleSyncer) Sync(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	if entity == nil {
		return fmt.Errorf("entity cannot be nil")
	}

	if entity.GetSyncSubjectType() != types.SyncSubjectTypeIECidrSvcRules {
		return fmt.Errorf("invalid entity type for IECidrSvcRuleSyncer: %s", entity.GetSyncSubjectType())
	}

	protoData, err := entity.ToSGroupsProto()
	if err != nil {
		return fmt.Errorf("failed to convert entity to sgroups proto: %w", err)
	}

	protoRule, ok := protoData.(*pb.IECidrSvcRule)
	if !ok {
		return fmt.Errorf("invalid proto data type for entity %s, expected *pb.IECidrSvcRule, got %T", entity.GetSyncKey(), protoData)
	}

	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeIECidrSvcRules,
		Data: &pb.SyncIECidrSvcRules{
			Rules: []*pb.IECidrSvcRule{protoRule},
		},
	}

	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync IECidrSvcRule with sgroups: %w", err)
	}

	s.logger.V(1).Info("Successfully synced IECidrSvcRule",
		"key", entity.GetSyncKey(),
		"operation", operation)

	return nil
}

// SyncBatch sends multiple IECidrSvcRules in a single request.
func (s *IECidrSvcRuleSyncer) SyncBatch(ctx context.Context, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	if len(entities) == 0 {
		return nil
	}

	var (
		protoRules []*pb.IECidrSvcRule
		entityKeys = make([]string, 0, len(entities))
	)

	for _, entity := range entities {
		if entity == nil {
			continue
		}

		if entity.GetSyncSubjectType() != types.SyncSubjectTypeIECidrSvcRules {
			return fmt.Errorf("invalid entity type for IECidrSvcRuleSyncer: %s", entity.GetSyncSubjectType())
		}

		protoData, err := entity.ToSGroupsProto()
		if err != nil {
			return fmt.Errorf("failed to convert entity %s to sgroups proto: %w", entity.GetSyncKey(), err)
		}

		protoRule, ok := protoData.(*pb.IECidrSvcRule)
		if !ok {
			return fmt.Errorf("invalid proto data type for entity %s, expected *pb.IECidrSvcRule, got %T", entity.GetSyncKey(), protoData)
		}

		protoRules = append(protoRules, protoRule)
		entityKeys = append(entityKeys, entity.GetSyncKey())
	}

	if len(protoRules) == 0 {
		return nil
	}

	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeIECidrSvcRules,
		Data: &pb.SyncIECidrSvcRules{
			Rules: protoRules,
		},
	}

	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync IECidrSvcRule batch with sgroups: %w", err)
	}

	s.logger.Info("Successfully synced IECidrSvcRule batch",
		"count", len(protoRules),
		"operation", operation,
		"keys", entityKeys)

	return nil
}

// GetSupportedSubjectType reports the subject type handled by this syncer.
func (s *IECidrSvcRuleSyncer) GetSupportedSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeIECidrSvcRules
}
