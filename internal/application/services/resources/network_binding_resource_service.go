package resources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/application/utils"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// NetworkBindingConditionManagerInterface provides condition processing for network bindings
type NetworkBindingConditionManagerInterface interface {
	ProcessNetworkBindingConditions(ctx context.Context, binding *models.NetworkBinding) error
}

// NetworkBindingResourceService provides business logic for NetworkBinding resources
type NetworkBindingResourceService struct {
	repo                   ports.Registry
	networkResourceService *NetworkResourceService
	syncTracker            *utils.SyncTracker
	retryConfig            utils.RetryConfig
	syncManager            interfaces.SyncManager
	conditionManager       NetworkBindingConditionManagerInterface
}

// NewNetworkBindingResourceService creates a new NetworkBindingResourceService
func NewNetworkBindingResourceService(
	repo ports.Registry,
	networkResourceService *NetworkResourceService,
	syncManager interfaces.SyncManager,
	conditionManager NetworkBindingConditionManagerInterface,
) *NetworkBindingResourceService {
	return &NetworkBindingResourceService{
		repo:                   repo,
		networkResourceService: networkResourceService,
		syncTracker:            utils.NewSyncTracker(1 * time.Second),
		retryConfig:            utils.DefaultRetryConfig(),
		syncManager:            syncManager,
		conditionManager:       conditionManager,
	}
}

// CreateNetworkBinding creates a new NetworkBinding with business logic validation
func (s *NetworkBindingResourceService) CreateNetworkBinding(ctx context.Context, binding *models.NetworkBinding) error {
	// Convert ObjectReference to ResourceIdentifier for validation
	networkRef := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
	addressGroupRef := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.Namespace}

	// Initialize metadata
	binding.GetMeta().TouchOnCreate()

	// Create the network binding
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Get reader from writer to ensure same session/transaction visibility
	reader, err := s.repo.ReaderFromWriter(ctx, writer)
	if err != nil {
		return fmt.Errorf("failed to get reader from writer: %w", err)
	}
	defer reader.Close()

	// Validate that the referenced Network exists and is not already bound
	bindingID := models.ResourceIdentifier{Name: binding.Name, Namespace: binding.Namespace}
	if err := s.networkResourceService.ValidateNetworkBindingWithReader(ctx, reader, networkRef, bindingID); err != nil {
		return fmt.Errorf("network validation failed: %w", err)
	}

	// Validate that the referenced AddressGroup exists
	if err := s.validateAddressGroupWithReader(ctx, reader, addressGroupRef); err != nil {
		return fmt.Errorf("address group validation failed: %w", err)
	}

	// Check if NetworkBinding already exists
	existing, err := s.getNetworkBindingByIDWithReader(ctx, reader, binding.Key())
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("failed to check existing network binding: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("network binding already exists: %s", binding.Key())
	}

	// Convert to slice for sync
	bindings := []models.NetworkBinding{*binding}
	if err := writer.SyncNetworkBindings(ctx, bindings, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync network bindings: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network binding creation: %w", err)
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessNetworkBindingConditions(ctx, binding); err != nil {
			klog.Errorf("Failed to process network binding conditions for %s/%s: %v",
				binding.Namespace, binding.Name, err)
			// Don't fail the operation if condition processing fails
		} else {
			// Save the processed conditions back to storage
			if err := s.saveNetworkBindingConditions(ctx, binding); err != nil {
				klog.Errorf("Failed to save network binding conditions for %s/%s: %v",
					binding.Namespace, binding.Name, err)
			}
		}
	}

	// Update the Network to mark it as bound
	if err := s.networkResourceService.UpdateNetworkBinding(ctx, networkRef, bindingID, addressGroupRef); err != nil {
		return fmt.Errorf("failed to update network binding: %w", err)
	}

	// Update AddressGroup.Networks.Items to include the Network
	if err := s.updateAddressGroupNetworks(ctx, addressGroupRef, networkRef, binding, true); err != nil {
		return fmt.Errorf("failed to update address group networks: %w", err)
	}

	if err := s.forceSyncAddressGroupWithSGroups(ctx, addressGroupRef); err != nil {
		log.Printf("❌ Failed to force sync AddressGroup %s with sgroups: %v", addressGroupRef.Key(), err)
		// Don't fail the operation - AddressGroup was updated successfully in database
	}

	return s.syncNetworkBindingWithExternal(ctx, binding, "create")
}

// UpdateNetworkBinding updates an existing NetworkBinding with business logic validation
func (s *NetworkBindingResourceService) UpdateNetworkBinding(ctx context.Context, binding *models.NetworkBinding) error {
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
		if err := s.networkResourceService.RemoveNetworkBinding(ctx, existingNetworkRef); err != nil {
			return fmt.Errorf("failed to remove old network binding: %w", err)
		}

		// Remove Network from old AddressGroup
		if err := s.updateAddressGroupNetworks(ctx, existingAddressGroupRef, existingNetworkRef, existing, false); err != nil {
			return fmt.Errorf("failed to remove network from old address group: %w", err)
		}

		// Validate that the new Network is not already bound
		bindingID := models.ResourceIdentifier{Name: binding.Name, Namespace: binding.Namespace}
		if err := s.networkResourceService.ValidateNetworkBinding(ctx, networkRef, bindingID); err != nil {
			return fmt.Errorf("new network validation failed: %w", err)
		}

		// Update the new Network to mark it as bound
		if err := s.networkResourceService.UpdateNetworkBinding(ctx, networkRef, bindingID, addressGroupRef); err != nil {
			return fmt.Errorf("failed to update new network binding: %w", err)
		}

		// Add Network to new AddressGroup
		if err := s.updateAddressGroupNetworks(ctx, addressGroupRef, networkRef, binding, true); err != nil {
			return fmt.Errorf("failed to add network to new address group: %w", err)
		}

		// FORCE SYNC: Immediately sync new AddressGroup with sgroups after Networks update
		if err := s.forceSyncAddressGroupWithSGroups(ctx, addressGroupRef); err != nil {
			log.Printf("❌ Failed to force sync new AddressGroup %s with sgroups: %v", addressGroupRef.Key(), err)
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
	if err := writer.SyncNetworkBindings(ctx, bindings, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync network bindings: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network binding update: %w", err)
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessNetworkBindingConditions(ctx, binding); err != nil {
			klog.Errorf("Failed to process network binding conditions for %s/%s: %v",
				binding.Namespace, binding.Name, err)
			// Don't fail the operation if condition processing fails
		} else {
			// Save the processed conditions back to storage
			if err := s.saveNetworkBindingConditions(ctx, binding); err != nil {
				klog.Errorf("Failed to save network binding conditions for %s/%s: %v",
					binding.Namespace, binding.Name, err)
			}
		}
	}

	// Sync with external systems
	return s.syncNetworkBindingWithExternal(ctx, binding, "update")
}

// DeleteNetworkBinding deletes a NetworkBinding with cleanup logic
func (s *NetworkBindingResourceService) DeleteNetworkBinding(ctx context.Context, id models.ResourceIdentifier) error {
	existing, err := s.getNetworkBindingByID(ctx, id.Key())
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		log.Printf("❌ DEBUG: Failed to get network binding for deletion: %v", err)
		return fmt.Errorf("failed to get network binding: %w", err)
	}
	if existing == nil || errors.Is(err, ports.ErrNotFound) {
		log.Printf("⚠️  DEBUG: Network binding %s doesn't exist - delete is idempotent", id.Key())
		return nil
	}

	networkRef := models.ResourceIdentifier{Name: existing.NetworkRef.Name, Namespace: existing.Namespace}
	addressGroupRef := models.ResourceIdentifier{Name: existing.AddressGroupRef.Name, Namespace: existing.Namespace}

	if err := s.networkResourceService.RemoveNetworkBinding(ctx, networkRef); err != nil {
		return fmt.Errorf("failed to remove network binding: %w", err)
	}
	// Remove Network from AddressGroup
	if err := s.updateAddressGroupNetworks(ctx, addressGroupRef, networkRef, existing, false); err != nil {
		log.Printf("❌ DEBUG: Failed to remove network from AddressGroup: %v", err)
		return fmt.Errorf("failed to remove network from address group: %w", err)
	}
	if err := s.forceSyncAddressGroupWithSGroups(ctx, addressGroupRef); err != nil {
		log.Printf("❌ Failed to force sync AddressGroup %s with sgroups after deletion: %v", addressGroupRef.Key(), err)
	}

	writer, err := s.repo.Writer(ctx)
	if err != nil {
		log.Printf("❌ DEBUG: Failed to get repository writer: %v", err)
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	if err := writer.DeleteNetworkBindingsByIDs(ctx, []models.ResourceIdentifier{id}); err != nil {
		log.Printf("❌ DEBUG: writer.DeleteNetworkBindingsByIDs failed: %v", err)
		return fmt.Errorf("failed to delete network binding: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network binding deletion: %w", err)
	}

	err = s.syncNetworkBindingWithExternal(ctx, existing, "delete")
	if err != nil {
		log.Printf("❌ DEBUG: Failed to sync with external systems: %v", err)
		return err
	}
	return nil
}

// GetNetworkBinding retrieves a NetworkBinding by ID
func (s *NetworkBindingResourceService) GetNetworkBinding(ctx context.Context, id models.ResourceIdentifier) (*models.NetworkBinding, error) {
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
func (s *NetworkBindingResourceService) ListNetworkBindings(ctx context.Context, scope ports.Scope) ([]models.NetworkBinding, error) {
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

func (s *NetworkBindingResourceService) getNetworkBindingByID(ctx context.Context, id string) (*models.NetworkBinding, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Parse resource identifier format: namespace/name
	parts := strings.Split(id, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid resource identifier format: %s (expected: namespace/name)", id)
	}
	resourceID := models.ResourceIdentifier{
		Namespace: parts[0],
		Name:      parts[1],
	}
	return reader.GetNetworkBindingByID(ctx, resourceID)
}

func (s *NetworkBindingResourceService) validateNetwork(ctx context.Context, networkRef models.ResourceIdentifier) error {
	network, err := s.networkResourceService.GetNetwork(ctx, networkRef)
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if network == nil {
		return fmt.Errorf("network not found: %s", networkRef.Key())
	}
	return nil
}

func (s *NetworkBindingResourceService) validateAddressGroup(ctx context.Context, addressGroupRef models.ResourceIdentifier) error {
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

// validateAddressGroupWithReader validates AddressGroup using provided reader (same session)
func (s *NetworkBindingResourceService) validateAddressGroupWithReader(ctx context.Context, reader ports.Reader, addressGroupRef models.ResourceIdentifier) error {
	addressGroup, err := reader.GetAddressGroupByID(ctx, addressGroupRef)
	if err != nil {
		return fmt.Errorf("failed to get address group: %w", err)
	}
	if addressGroup == nil {
		return fmt.Errorf("address group not found: %s", addressGroupRef.Key())
	}
	return nil
}

// getNetworkBindingByIDWithReader gets NetworkBinding using provided reader (same session)
func (s *NetworkBindingResourceService) getNetworkBindingByIDWithReader(ctx context.Context, reader ports.Reader, id string) (*models.NetworkBinding, error) {
	// Parse the key (namespace/name format) to extract namespace and name
	parts := strings.Split(id, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid resource identifier format: %s (expected: namespace/name)", id)
	}
	resourceID := models.ResourceIdentifier{
		Namespace: parts[0],
		Name:      parts[1],
	}
	return reader.GetNetworkBindingByID(ctx, resourceID)
}

// getFreshAddressGroupFromDatabase reads the latest AddressGroup data from database
// This ensures sgroups synchronization uses the most up-to-date Networks field
func (s *NetworkBindingResourceService) getFreshAddressGroupFromDatabase(ctx context.Context, addressGroupRef models.ResourceIdentifier) (*models.AddressGroup, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	addressGroup, err := reader.GetAddressGroupByID(ctx, addressGroupRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get fresh address group: %w", err)
	}
	if addressGroup == nil {
		return nil, fmt.Errorf("fresh address group not found: %s", addressGroupRef.Key())
	}

	return addressGroup, nil
}

// updateAddressGroupNetworks updates the Networks.Items field in AddressGroup
func (s *NetworkBindingResourceService) updateAddressGroupNetworks(ctx context.Context, addressGroupRef, networkRef models.ResourceIdentifier, binding *models.NetworkBinding, add bool) error {
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
	network, err := s.networkResourceService.GetNetwork(ctx, networkRef)
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

	// Sync the updated AddressGroup using NetworkService (this commits to database)
	log.Printf("🔧 updateAddressGroupNetworks: About to call UpdateAddressGroup for %s", addressGroupRef.Key())
	if err := s.networkResourceService.UpdateAddressGroup(ctx, addressGroup); err != nil {
		return fmt.Errorf("failed to update address group: %w", err)
	}
	log.Printf("✅ updateAddressGroupNetworks: UpdateAddressGroup completed for %s", addressGroupRef.Key())

	log.Printf("✅ updateAddressGroupNetworks: AddressGroup Networks field updated successfully")
	return nil
}

func (s *NetworkBindingResourceService) syncNetworkBindingWithExternal(ctx context.Context, binding *models.NetworkBinding, operation string) error {
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
		// Skip condition setting for delete operations to avoid validation errors
		if operation != "delete" {
			utils.SetSyncFailedCondition(binding, err)
		}
		return fmt.Errorf("failed to sync with external system: %w", err)
	}

	s.syncTracker.RecordSuccess(syncKey)
	// Skip condition setting for delete operations to avoid trying to save conditions
	// for a resource that has already been deleted from the database
	if operation != "delete" {
		utils.SetSyncSuccessCondition(binding)
		log.Printf("✅ Successfully set sync success condition for NetworkBinding %s", binding.Key())
	} else {
		log.Printf("✅ Successfully completed delete sync for NetworkBinding %s (skipped condition saving)", binding.Key())
	}
	return nil
}

// saveNetworkBindingConditions saves the processed conditions for a NetworkBinding back to storage
func (s *NetworkBindingResourceService) saveNetworkBindingConditions(ctx context.Context, binding *models.NetworkBinding) error {
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer for NetworkBinding conditions: %w", err)
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	scope := ports.NewResourceIdentifierScope(binding.ResourceIdentifier)

	// Sync the NetworkBinding with updated conditions
	if err := writer.SyncNetworkBindings(ctx, []models.NetworkBinding{*binding}, scope); err != nil {
		return fmt.Errorf("failed to sync NetworkBinding with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit NetworkBinding conditions: %w", err)
	}

	klog.Infof("💾 NetworkBindingResourceService: Successfully saved conditions for NetworkBinding %s/%s", binding.Namespace, binding.Name)
	return nil
}

// forceSyncAddressGroupWithSGroups forces immediate sync of AddressGroup with sgroups
// This bypasses debouncing using SyncEntityForced and ensures fresh database data is used
func (s *NetworkBindingResourceService) forceSyncAddressGroupWithSGroups(ctx context.Context, addressGroupRef models.ResourceIdentifier) error {
	log.Printf("🚀 FORCE SYNC: Starting immediate sgroups sync for AddressGroup %s", addressGroupRef.Key())

	// Get fresh AddressGroup data from database
	freshAddressGroup, err := s.getFreshAddressGroupFromDatabase(ctx, addressGroupRef)
	if err != nil {
		return fmt.Errorf("failed to get fresh AddressGroup %s: %w", addressGroupRef.Key(), err)
	}

	log.Printf("🔧 FORCE SYNC: Fresh AddressGroup %s has %d networks", addressGroupRef.Key(), len(freshAddressGroup.Networks))

	// Force sync with sgroups using SyncEntityForced (bypasses debouncing)
	if s.syncManager != nil {
		log.Printf("🔄 FORCE SYNC: Using SyncEntityForced to bypass debouncing for AddressGroup %s", addressGroupRef.Key())

		if err := s.syncManager.SyncEntityForced(ctx, freshAddressGroup, types.SyncOperationUpsert); err != nil {
			return fmt.Errorf("failed to force sync AddressGroup %s with sgroups: %w", addressGroupRef.Key(), err)
		}

		log.Printf("✅ FORCE SYNC: Successfully synced AddressGroup %s with sgroups (debouncing bypassed)", addressGroupRef.Key())
	} else {
		log.Printf("⚠️  FORCE SYNC: SyncManager is nil - skipping sgroups sync for AddressGroup %s", addressGroupRef.Key())
	}

	return nil
}
