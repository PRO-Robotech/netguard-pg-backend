package syncers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/PRO-Robotech/protos/pkg/api/common"
	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// NetworkSyncer implements EntitySyncer for Network entities
type NetworkSyncer struct {
	gateway interfaces.SGroupGateway
	logger  logr.Logger
}

// NewNetworkSyncer creates a new Network syncer
func NewNetworkSyncer(gateway interfaces.SGroupGateway, logger logr.Logger) *NetworkSyncer {
	return &NetworkSyncer{
		gateway: gateway,
		logger:  logger,
	}
}

// convertToSGroupsProto converts any data to proper sgroups protobuf format
func (s *NetworkSyncer) convertToSGroupsProto(entity interfaces.SyncableEntity) (*pb.SyncNetworks, error) {
	// Get raw data from entity
	rawData, err := entity.ToSGroupsProto()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw proto data: %w", err)
	}

	// Handle map[string]interface{} case
	if dataMap, ok := rawData.(map[string]interface{}); ok {
		name := ""
		cidr := ""

		if nameVal, exists := dataMap["name"]; exists {
			if nameStr, ok := nameVal.(string); ok {
				name = nameStr
			}
		}

		if cidrVal, exists := dataMap["cidr"]; exists {
			if cidrStr, ok := cidrVal.(string); ok {
				cidr = cidrStr
			}
		}

		// Add namespace to name if provided
		if namespaceVal, exists := dataMap["namespace"]; exists {
			if namespaceStr, ok := namespaceVal.(string); ok && namespaceStr != "" {
				name = fmt.Sprintf("%s/%s", namespaceStr, name)
			}
		}

		// Create proper sgroups protobuf
		protoNetwork := &pb.Network{
			Name: name,
			Network: &common.Networks_NetIP{
				CIDR: cidr,
			},
		}

		return &pb.SyncNetworks{
			Networks: []*pb.Network{protoNetwork},
		}, nil
	}

	// If already proper format, return as is
	if syncNetworks, ok := rawData.(*pb.SyncNetworks); ok {
		return syncNetworks, nil
	}

	return nil, fmt.Errorf("unsupported data format: %T", rawData)
}

// Sync synchronizes a single Network entity
func (s *NetworkSyncer) Sync(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	if entity == nil {
		return fmt.Errorf("entity cannot be nil")
	}

	// Validate entity type
	if entity.GetSyncSubjectType() != types.SyncSubjectTypeNetworks {
		return fmt.Errorf("invalid entity type for NetworkSyncer: %s", entity.GetSyncSubjectType())
	}

	// Convert entity to single protobuf network
	protoData, err := entity.ToSGroupsProto()
	if err != nil {
		return fmt.Errorf("failed to convert entity to sgroups proto: %w", err)
	}

	// Cast to *pb.Network and wrap in SyncNetworks for single entity
	protoNetwork, ok := protoData.(*pb.Network)
	if !ok {
		return fmt.Errorf("invalid proto data type for entity %s, expected *pb.Network, got %T", entity.GetSyncKey(), protoData)
	}

	// Create single-entity batch structure for backward compatibility
	singleEntityBatch := &pb.SyncNetworks{
		Networks: []*pb.Network{protoNetwork},
	}

	// Create sync request
	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeNetworks,
		Data:        singleEntityBatch, // Send single-entity batch structure
	}

	// Send sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync Network with sgroups: %w", err)
	}

	s.logger.V(1).Info("Successfully synced Network",
		"key", entity.GetSyncKey(),
		"operation", operation)

	return nil
}

// SyncBatch synchronizes multiple Network entities in a batch
func (s *NetworkSyncer) SyncBatch(ctx context.Context, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	if len(entities) == 0 {
		return nil
	}

	// Validate all entities and convert to protobuf networks
	var protoNetworks []*pb.Network
	entityKeys := make([]string, 0, len(entities))

	for _, entity := range entities {
		if entity == nil {
			continue
		}

		if entity.GetSyncSubjectType() != types.SyncSubjectTypeNetworks {
			return fmt.Errorf("invalid entity type for NetworkSyncer: %s", entity.GetSyncSubjectType())
		}

		// Convert entity to single protobuf network
		protoData, err := entity.ToSGroupsProto()
		if err != nil {
			return fmt.Errorf("failed to convert entity %s to sgroups proto: %w", entity.GetSyncKey(), err)
		}

		// Cast to *pb.Network
		if protoNetwork, ok := protoData.(*pb.Network); ok {
			protoNetworks = append(protoNetworks, protoNetwork)
			entityKeys = append(entityKeys, entity.GetSyncKey())
		} else {
			return fmt.Errorf("invalid proto data type for entity %s, expected *pb.Network, got %T", entity.GetSyncKey(), protoData)
		}
	}

	if len(protoNetworks) == 0 {
		return nil
	}

	// Create aggregated batch sync request
	batchProtoData := &pb.SyncNetworks{
		Networks: protoNetworks,
	}

	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeNetworks,
		Data:        batchProtoData, // Send aggregated structure
	}

	// Send batch sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		return fmt.Errorf("failed to sync Network batch with sgroups: %w", err)
	}

	s.logger.Info("Successfully synced Network batch",
		"count", len(protoNetworks),
		"operation", operation,
		"keys", entityKeys)

	return nil
}

// GetSupportedSubjectType returns the subject type this syncer supports
func (s *NetworkSyncer) GetSupportedSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeNetworks
}
