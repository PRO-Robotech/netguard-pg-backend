package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"netguard-pg-backend/internal/application/utils"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// NetworkBindingService provides business logic for NetworkBinding resources
type NetworkBindingService struct {
	repo           ports.Registry
	networkService *NetworkService
	syncTracker    *utils.SyncTracker
	retryConfig    utils.RetryConfig
	syncManager    interfaces.SyncManager
}

// NewNetworkBindingService creates a new NetworkBindingService
func NewNetworkBindingService(repo ports.Registry, networkService *NetworkService, syncManager interfaces.SyncManager) *NetworkBindingService {
	return &NetworkBindingService{
		repo:           repo,
		networkService: networkService,
		syncTracker:    utils.NewSyncTracker(1 * time.Second),
		retryConfig:    utils.DefaultRetryConfig(),
		syncManager:    syncManager,
	}
}

// CreateNetworkBinding creates a new NetworkBinding with business logic validation
func (s *NetworkBindingService) CreateNetworkBinding(ctx context.Context, binding *models.NetworkBinding) error {
	// Convert ObjectReference to ResourceIdentifier for validation
	networkRef := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
	addressGroupRef := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.Namespace}

	// Validate that the referenced Network exists and is not already bound
	bindingID := models.ResourceIdentifier{Name: binding.GetID(), Namespace: binding.Namespace}
	if err := s.networkService.ValidateNetworkBinding(ctx, networkRef, bindingID); err != nil {
		return fmt.Errorf("network validation failed: %w", err)
	}

	// Validate that the referenced AddressGroup exists
	if err := s.validateAddressGroup(ctx, addressGroupRef); err != nil {
		return fmt.Errorf("address group validation failed: %w", err)
	}

	// Check if NetworkBinding already exists
	existing, err := s.getNetworkBindingByID(ctx, binding.Key())
	if err != nil {
		return fmt.Errorf("failed to check existing network binding: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("network binding already exists: %s", binding.Key())
	}

	// Initialize metadata
	binding.GetMeta().TouchOnCreate()

	// Create the network binding
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Convert to slice for sync
	bindings := []models.NetworkBinding{*binding}
	if err := writer.SyncNetworkBindings(ctx, bindings, ports.EmptyScope{}); err != nil {
		return fmt.Errorf("failed to sync network bindings: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network binding creation: %w", err)
	}

	// Update the Network to mark it as bound
	if err := s.networkService.UpdateNetworkBinding(ctx, networkRef, bindingID, addressGroupRef); err != nil {
		return fmt.Errorf("failed to update network binding: %w", err)
	}

	// Update AddressGroup.Networks.Items to include the Network
	if err := s.updateAddressGroupNetworks(ctx, addressGroupRef, networkRef, binding, true); err != nil {
		return fmt.Errorf("failed to update address group networks: %w", err)
	}

	// Sync with external systems
	return s.syncNetworkBindingWithExternal(ctx, binding, "create")
}

// UpdateNetworkBinding updates an existing NetworkBinding with business logic validation
func (s *NetworkBindingService) UpdateNetworkBinding(ctx context.Context, binding *models.NetworkBinding) error {
	// Check if NetworkBinding exists
	existing, err := s.getNetworkBindingByID(ctx, binding.Key())
	if err != nil {
		return fmt.Errorf("failed to get existing network binding: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("network binding not found: %s", binding.Key())
	}

	// Convert ObjectReference to ResourceIdentifier for validation
	networkRef := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
	addressGroupRef := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.Namespace}

	// Validate that the referenced Network exists
	if err := s.validateNetwork(ctx, networkRef); err != nil {
		return fmt.Errorf("network validation failed: %w", err)
	}

	// Validate that the referenced AddressGroup exists
	if err := s.validateAddressGroup(ctx, addressGroupRef); err != nil {
		return fmt.Errorf("address group validation failed: %w", err)
	}

	// Check if Network or AddressGroup references have changed
	if existing.NetworkRef.Name != binding.NetworkRef.Name || existing.AddressGroupRef.Name != binding.AddressGroupRef.Name {
		// Convert existing ObjectReference to ResourceIdentifier
		existingNetworkRef := models.ResourceIdentifier{Name: existing.NetworkRef.Name, Namespace: existing.Namespace}
		existingAddressGroupRef := models.ResourceIdentifier{Name: existing.AddressGroupRef.Name, Namespace: existing.Namespace}

		// Remove binding from old Network
		if err := s.networkService.RemoveNetworkBinding(ctx, existingNetworkRef); err != nil {
			return fmt.Errorf("failed to remove old network binding: %w", err)
		}

		// Remove Network from old AddressGroup
		if err := s.updateAddressGroupNetworks(ctx, existingAddressGroupRef, existingNetworkRef, existing, false); err != nil {
			return fmt.Errorf("failed to remove network from old address group: %w", err)
		}

		// Validate that the new Network is not already bound
		bindingID := models.ResourceIdentifier{Name: binding.GetID(), Namespace: binding.Namespace}
		if err := s.networkService.ValidateNetworkBinding(ctx, networkRef, bindingID); err != nil {
			return fmt.Errorf("new network validation failed: %w", err)
		}

		// Update the new Network to mark it as bound
		if err := s.networkService.UpdateNetworkBinding(ctx, networkRef, bindingID, addressGroupRef); err != nil {
			return fmt.Errorf("failed to update new network binding: %w", err)
		}

		// Add Network to new AddressGroup
		if err := s.updateAddressGroupNetworks(ctx, addressGroupRef, networkRef, binding, true); err != nil {
			return fmt.Errorf("failed to add network to new address group: %w", err)
		}
	}

	// Update metadata
	binding.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	// Update the network binding
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Convert to slice for sync
	bindings := []models.NetworkBinding{*binding}
	if err := writer.SyncNetworkBindings(ctx, bindings, ports.EmptyScope{}); err != nil {
		return fmt.Errorf("failed to sync network bindings: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network binding update: %w", err)
	}

	// Sync with external systems
	return s.syncNetworkBindingWithExternal(ctx, binding, "update")
}

// DeleteNetworkBinding deletes a NetworkBinding with cleanup logic
func (s *NetworkBindingService) DeleteNetworkBinding(ctx context.Context, id models.ResourceIdentifier) error {
	// Check if NetworkBinding exists
	existing, err := s.getNetworkBindingByID(ctx, id.Key())
	if err != nil {
		return fmt.Errorf("failed to get network binding: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("network binding not found: %s", id.Key())
	}

	// Convert ObjectReference to ResourceIdentifier
	networkRef := models.ResourceIdentifier{Name: existing.NetworkRef.Name, Namespace: existing.Namespace}
	addressGroupRef := models.ResourceIdentifier{Name: existing.AddressGroupRef.Name, Namespace: existing.Namespace}

	// Remove binding from Network
	if err := s.networkService.RemoveNetworkBinding(ctx, networkRef); err != nil {
		return fmt.Errorf("failed to remove network binding: %w", err)
	}

	// Remove Network from AddressGroup
	if err := s.updateAddressGroupNetworks(ctx, addressGroupRef, networkRef, existing, false); err != nil {
		return fmt.Errorf("failed to remove network from address group: %w", err)
	}

	// Delete the network binding
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	if err := writer.DeleteNetworkBindingsByIDs(ctx, []models.ResourceIdentifier{id}); err != nil {
		return fmt.Errorf("failed to delete network binding: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network binding deletion: %w", err)
	}

	// Sync deletion with external systems
	return s.syncNetworkBindingWithExternal(ctx, existing, "delete")
}

// GetNetworkBinding retrieves a NetworkBinding by ID
func (s *NetworkBindingService) GetNetworkBinding(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	binding, err := reader.GetNetworkBindingByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get network binding: %w", err)
	}

	return binding, nil
}

// ListNetworkBindings retrieves all NetworkBindings
func (s *NetworkBindingService) ListNetworkBindings(ctx context.Context, scope ports.Scope) ([]models.NetworkBinding, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	var bindings []models.NetworkBinding
	err = reader.ListNetworkBindings(ctx, func(binding models.NetworkBinding) error {
		bindings = append(bindings, binding)
		return nil
	}, scope)

	if err != nil {
		return nil, fmt.Errorf("failed to list network bindings: %w", err)
	}

	return bindings, nil
}

// Helper methods

func (s *NetworkBindingService) getNetworkBindingByID(ctx context.Context, id string) (*models.NetworkBinding, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	resourceID := models.ResourceIdentifier{Name: id}
	return reader.GetNetworkBindingByID(ctx, resourceID)
}

func (s *NetworkBindingService) validateNetwork(ctx context.Context, networkRef models.ResourceIdentifier) error {
	network, err := s.networkService.GetNetwork(ctx, networkRef)
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if network == nil {
		return fmt.Errorf("network not found: %s", networkRef.Key())
	}
	return nil
}

func (s *NetworkBindingService) validateAddressGroup(ctx context.Context, addressGroupRef models.ResourceIdentifier) error {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	addressGroup, err := reader.GetAddressGroupByID(ctx, addressGroupRef)
	if err != nil {
		return fmt.Errorf("failed to get address group: %w", err)
	}
	if addressGroup == nil {
		return fmt.Errorf("address group not found: %s", addressGroupRef.Key())
	}
	return nil
}

// updateAddressGroupNetworks updates the Networks.Items field in AddressGroup
func (s *NetworkBindingService) updateAddressGroupNetworks(ctx context.Context, addressGroupRef, networkRef models.ResourceIdentifier, binding *models.NetworkBinding, add bool) error {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	// Get the AddressGroup
	addressGroup, err := reader.GetAddressGroupByID(ctx, addressGroupRef)
	if err != nil {
		return fmt.Errorf("failed to get address group: %w", err)
	}
	if addressGroup == nil {
		return fmt.Errorf("address group not found: %s", addressGroupRef.Key())
	}

	// Get the Network
	network, err := s.networkService.GetNetwork(ctx, networkRef)
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if network == nil {
		return fmt.Errorf("network not found: %s", networkRef.Key())
	}

	// Generate network name (namespace/name format)
	networkName := fmt.Sprintf("%s/%s", network.Namespace, network.Name)

	if add {
		// Add network to AddressGroup.Networks.Items
		log.Printf("🔗 Adding network %s to AddressGroup %s", networkName, addressGroupRef.Key())

		// Check if network already exists
		networkExists := false
		for _, existingNetwork := range addressGroup.Networks {
			if existingNetwork.Name == networkName {
				networkExists = true
				break
			}
		}

		if !networkExists {
			// Create new NetworkItem
			networkItem := models.NetworkItem{
				Name:       networkName,
				CIDR:       network.CIDR,
				ApiVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Network",
				Namespace:  network.Namespace,
			}

			// Add to Networks slice
			addressGroup.Networks = append(addressGroup.Networks, networkItem)
			log.Printf("✅ Added network %s to AddressGroup %s", networkName, addressGroupRef.Key())
		} else {
			log.Printf("ℹ️  Network %s already exists in AddressGroup %s", networkName, addressGroupRef.Key())
		}
	} else {
		// Remove network from AddressGroup.Networks.Items
		log.Printf("🔗 Removing network %s from AddressGroup %s", networkName, addressGroupRef.Key())

		var updatedNetworks []models.NetworkItem
		for _, existingNetwork := range addressGroup.Networks {
			if existingNetwork.Name != networkName {
				updatedNetworks = append(updatedNetworks, existingNetwork)
			}
		}

		if len(updatedNetworks) != len(addressGroup.Networks) {
			addressGroup.Networks = updatedNetworks
			log.Printf("✅ Removed network %s from AddressGroup %s", networkName, addressGroupRef.Key())
		} else {
			log.Printf("ℹ️  Network %s not found in AddressGroup %s", networkName, addressGroupRef.Key())
		}
	}

	// Update metadata
	addressGroup.Meta.TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	// Sync the updated AddressGroup using NetworkService
	if err := s.networkService.UpdateAddressGroup(ctx, addressGroup); err != nil {
		return fmt.Errorf("failed to update address group: %w", err)
	}

	// Sync with SGROUP
	if s.syncManager != nil {
		log.Printf("🔄 Syncing AddressGroup %s with SGROUP", addressGroupRef.Key())
		if syncErr := s.syncManager.SyncEntity(ctx, addressGroup, types.SyncOperationUpsert); syncErr != nil {
			log.Printf("❌ Failed to sync AddressGroup %s with SGROUP: %v", addressGroupRef.Key(), syncErr)
			// Don't fail the operation, sync can be retried later
		} else {
			log.Printf("✅ Successfully synced AddressGroup %s with SGROUP", addressGroupRef.Key())
		}
	}

	return nil
}

func (s *NetworkBindingService) syncNetworkBindingWithExternal(ctx context.Context, binding *models.NetworkBinding, operation string) error {
	syncKey := fmt.Sprintf("%s-%s", operation, binding.GetID())

	// Check debouncing
	if !s.syncTracker.ShouldSync(syncKey) {
		return nil // Skip sync due to debouncing
	}

	// Execute sync with retry
	err := utils.ExecuteWithRetry(ctx, s.retryConfig, func() error {
		// NetworkBinding itself is not synced with SGROUP
		// Only Network and AddressGroup resources are synced
		log.Printf("ℹ️  NetworkBinding %s is not synced with SGROUP (it's just a linking document)", binding.Key())
		return nil
	})

	if err != nil {
		s.syncTracker.RecordFailure(syncKey, err)
		utils.SetSyncFailedCondition(binding, err)
		return fmt.Errorf("failed to sync with external system: %w", err)
	}

	s.syncTracker.RecordSuccess(syncKey)
	utils.SetSyncSuccessCondition(binding)
	return nil
}
