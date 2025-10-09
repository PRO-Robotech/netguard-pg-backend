package syncers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// ServiceSyncer implements EntitySyncer for Service entities
type ServiceSyncer struct {
	gateway interfaces.SGroupGateway
	logger  logr.Logger
}

// NewServiceSyncer creates a new Service syncer
func NewServiceSyncer(gateway interfaces.SGroupGateway, logger logr.Logger) *ServiceSyncer {
	return &ServiceSyncer{
		gateway: gateway,
		logger:  logger,
	}
}

// Sync synchronizes a single Service entity
func (s *ServiceSyncer) Sync(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	if entity == nil {
		return fmt.Errorf("entity cannot be nil")
	}

	// Validate entity type
	if entity.GetSyncSubjectType() != types.SyncSubjectTypeServices {
		return fmt.Errorf("invalid entity type for ServiceSyncer: %s", entity.GetSyncSubjectType())
	}

	// Convert entity to single protobuf service
	protoData, err := entity.ToSGroupsProto()
	if err != nil {
		return fmt.Errorf("failed to convert entity to sgroups proto: %w", err)
	}

	// Cast to *pb.Service and wrap in SyncServices for single entity
	protoService, ok := protoData.(*pb.Service)
	if !ok {
		return fmt.Errorf("invalid proto data type for entity %s, expected *pb.Service, got %T", entity.GetSyncKey(), protoData)
	}

	// Create single-entity batch structure for backward compatibility
	singleEntityBatch := &pb.SyncServices{
		Services: []*pb.Service{protoService},
	}

	// Create sync request
	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeServices,
		Data:        singleEntityBatch, // Send single-entity batch structure
	}

	// Send sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync Service with sgroups: %w", err)
	}

	s.logger.V(1).Info("Successfully synced Service",
		"key", entity.GetSyncKey(),
		"operation", operation)

	return nil
}

// SyncBatch synchronizes multiple Service entities in a batch
func (s *ServiceSyncer) SyncBatch(ctx context.Context, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	if len(entities) == 0 {
		return nil
	}

	// Validate all entities and convert to protobuf services
	var protoServices []*pb.Service
	entityKeys := make([]string, 0, len(entities))

	for _, entity := range entities {
		if entity == nil {
			continue
		}

		if entity.GetSyncSubjectType() != types.SyncSubjectTypeServices {
			return fmt.Errorf("invalid entity type for ServiceSyncer: %s", entity.GetSyncSubjectType())
		}

		// Convert entity to single protobuf service
		protoData, err := entity.ToSGroupsProto()
		if err != nil {
			return fmt.Errorf("failed to convert entity %s to sgroups proto: %w", entity.GetSyncKey(), err)
		}

		// Cast to *pb.Service
		if protoService, ok := protoData.(*pb.Service); ok {
			protoServices = append(protoServices, protoService)
			entityKeys = append(entityKeys, entity.GetSyncKey())
		} else {
			return fmt.Errorf("invalid proto data type for entity %s, expected *pb.Service, got %T", entity.GetSyncKey(), protoData)
		}
	}

	if len(protoServices) == 0 {
		return nil
	}

	// Create aggregated batch sync request
	batchProtoData := &pb.SyncServices{
		Services: protoServices,
	}

	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeServices,
		Data:        batchProtoData, // Send aggregated structure
	}

	// Send batch sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync Service batch with sgroups: %w", err)
	}

	s.logger.Info("Successfully synced Service batch",
		"count", len(protoServices),
		"operation", operation,
		"keys", entityKeys)

	return nil
}

// GetSupportedSubjectType returns the subject type this syncer supports
func (s *ServiceSyncer) GetSupportedSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeServices
}
