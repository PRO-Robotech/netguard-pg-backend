package syncers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

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

	fmt.Printf("🔧 DEBUG: AddressGroupSyncer.Sync - Starting sync for entity %s (operation: %s)\n", entity.GetSyncKey(), operation)

	// Validate entity type
	if entity.GetSyncSubjectType() != types.SyncSubjectTypeGroups {
		return fmt.Errorf("invalid entity type for AddressGroupSyncer: %s", entity.GetSyncSubjectType())
	}

	// Convert entity to sgroups protobuf format
	protoData, err := entity.ToSGroupsProto()
	if err != nil {
		fmt.Printf("❌ ERROR: AddressGroupSyncer.Sync - Failed to convert entity %s to proto: %v\n", entity.GetSyncKey(), err)
		return fmt.Errorf("failed to convert entity to sgroups proto: %w", err)
	}

	fmt.Printf("🔧 DEBUG: AddressGroupSyncer.Sync - Converted entity %s to proto\n", entity.GetSyncKey())

	// Create sync request
	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeGroups,
		Data:        protoData,
	}

	fmt.Printf("🔧 DEBUG: AddressGroupSyncer.Sync - Sending sync request to gateway for entity %s\n", entity.GetSyncKey())

	// Send sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		fmt.Printf("❌ ERROR: AddressGroupSyncer.Sync - Gateway sync failed for entity %s: %v\n", entity.GetSyncKey(), err)
		return fmt.Errorf("failed to sync AddressGroup with sgroups: %w", err)
	}

	fmt.Printf("✅ DEBUG: AddressGroupSyncer.Sync - Successfully completed sync for entity %s\n", entity.GetSyncKey())

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

	// Validate all entities are AddressGroups
	protoEntities := make([]interface{}, 0, len(entities))
	entityKeys := make([]string, 0, len(entities))

	for _, entity := range entities {
		if entity == nil {
			continue
		}

		if entity.GetSyncSubjectType() != types.SyncSubjectTypeGroups {
			return fmt.Errorf("invalid entity type for AddressGroupSyncer: %s", entity.GetSyncSubjectType())
		}

		// Convert entity to sgroups protobuf format
		protoData, err := entity.ToSGroupsProto()
		if err != nil {
			return fmt.Errorf("failed to convert entity %s to sgroups proto: %w", entity.GetSyncKey(), err)
		}

		protoEntities = append(protoEntities, protoData)
		entityKeys = append(entityKeys, entity.GetSyncKey())
	}

	if len(protoEntities) == 0 {
		return nil
	}

	// Create batch sync request
	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeGroups,
		Data:        protoEntities, // Array of proto entities
	}

	// Send batch sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync AddressGroup batch with sgroups: %w", err)
	}

	s.logger.Info("Successfully synced AddressGroup batch",
		"count", len(protoEntities),
		"operation", operation,
		"keys", entityKeys)

	return nil
}

// GetSupportedSubjectType returns the subject type this syncer supports
func (s *AddressGroupSyncer) GetSupportedSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeGroups
}
