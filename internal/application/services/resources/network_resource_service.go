package resources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/application/utils"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/interfaces"
)

// NetworkConditionManagerInterface provides condition processing for networks
type NetworkConditionManagerInterface interface {
	ProcessNetworkConditions(ctx context.Context, network *models.Network, syncResult error) error
}

// NetworkResourceService provides business logic for Network resources
type NetworkResourceService struct {
	repo             ports.Registry
	syncTracker      *utils.SyncTracker
	retryConfig      utils.RetryConfig
	syncManager      interfaces.SyncManager
	conditionManager NetworkConditionManagerInterface
}

// NewNetworkResourceService creates a new NetworkResourceService
func NewNetworkResourceService(
	repo ports.Registry,
	syncManager interfaces.SyncManager,
	conditionManager NetworkConditionManagerInterface,
) *NetworkResourceService {
	return &NetworkResourceService{
		repo:             repo,
		syncTracker:      utils.NewSyncTracker(1 * time.Second),
		retryConfig:      utils.DefaultRetryConfig(),
		syncManager:      syncManager,
		conditionManager: conditionManager,
	}
}

// CreateNetwork creates a new Network with business logic validation
func (s *NetworkResourceService) CreateNetwork(ctx context.Context, network *models.Network) error {
	// Validate CIDR format
	if err := s.validateCIDR(network.CIDR); err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}

	// Check if Network already exists
	existing, err := s.getNetworkByID(ctx, network.Key())
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("failed to check existing network: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("network already exists: %s", network.Key())
	}

	network.GetMeta().TouchOnCreate()

	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	networks := []models.Network{*network}
	if err := writer.SyncNetworks(ctx, networks, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync networks: %w", err)
	}

	// CLOUD-233: Removed syncNetworkWithExternal() - SGROUP sync now handled by OutboxWorker
	// Migration 027 triggers create outbox entries, OutboxWorker processes them asynchronously

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network creation: %w", err)
	}

	// ConditionManager needs the ACTUAL database state, not the in-memory object
	// Writer may have set Ready=False (PendingSGROUPSync), which must be preserved
	if s.conditionManager != nil {
		// Re-read the network from database to get Writer-applied conditions
		reader, err := s.repo.Reader(ctx)
		if err != nil {
			klog.Errorf("Failed to get reader for condition processing %s/%s: %v",
				network.Namespace, network.Name, err)
			return nil // Don't fail the operation
		}
		defer reader.Close()

		freshNetwork, err := reader.GetNetworkByID(ctx, models.ResourceIdentifier{
			Namespace: network.Namespace,
			Name:      network.Name,
		})
		if err != nil {
			klog.Errorf("Failed to re-read network for condition processing %s/%s: %v",
				network.Namespace, network.Name, err)
			return nil // Don't fail the operation
		}

		// Process conditions with the FRESH network from database
		if err := s.conditionManager.ProcessNetworkConditions(ctx, freshNetwork, nil); err != nil {
			klog.Errorf("Failed to process network conditions for %s/%s: %v",
				network.Namespace, network.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	return nil
}

// UpdateNetwork updates an existing Network with business logic validation
func (s *NetworkResourceService) UpdateNetwork(ctx context.Context, network *models.Network) error {
	// Validate CIDR format
	if err := s.validateCIDR(network.CIDR); err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}

	// Check if Network exists
	existing, err := s.getNetworkByID(ctx, network.GetID())
	if err != nil {
		return fmt.Errorf("failed to get existing network: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("network not found: %s", network.GetID())
	}

	// Check if Network is bound and prevent certain changes
	if existing.IsBound {
		// Prevent changing CIDR when bound
		if existing.CIDR != network.CIDR {
			return fmt.Errorf("cannot change CIDR when network is bound")
		}
	}

	network.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	networks := []models.Network{*network}
	if err := writer.SyncNetworks(ctx, networks, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync networks: %w", err)
	}

	// CLOUD-233: Removed syncNetworkWithExternal() - SGROUP sync now handled by OutboxWorker
	// Migration 027 triggers create outbox entries, OutboxWorker processes them asynchronously

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network update: %w", err)
	}

	// ConditionManager needs the ACTUAL database state, not the in-memory object
	// Writer may have set Ready=False (PendingSGROUPSync), which must be preserved
	if s.conditionManager != nil {
		// Re-read the network from database to get Writer-applied conditions
		reader, err := s.repo.Reader(ctx)
		if err != nil {
			klog.Errorf("Failed to get reader for condition processing %s/%s: %v",
				network.Namespace, network.Name, err)
			return nil // Don't fail the operation
		}
		defer reader.Close()

		freshNetwork, err := reader.GetNetworkByID(ctx, models.ResourceIdentifier{
			Namespace: network.Namespace,
			Name:      network.Name,
		})
		if err != nil {
			klog.Errorf("Failed to re-read network for condition processing %s/%s: %v",
				network.Namespace, network.Name, err)
			return nil // Don't fail the operation
		}

		// Process conditions with the FRESH network from database
		if err := s.conditionManager.ProcessNetworkConditions(ctx, freshNetwork, nil); err != nil {
			klog.Errorf("Failed to process network conditions for %s/%s: %v",
				network.Namespace, network.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	return nil
}

// DeleteNetwork deletes a Network with cleanup logic
func (s *NetworkResourceService) DeleteNetwork(ctx context.Context, id models.ResourceIdentifier) error {
	existing, err := s.getNetworkByID(ctx, id.Key())
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if existing == nil || errors.Is(err, ports.ErrNotFound) {
		return nil
	}

	// Check if Network is bound and handle cleanup
	if existing.IsBound {
		// Capture AddressGroupRef before clearing it for cleanup
		var addressGroupRef models.ResourceIdentifier
		if existing.AddressGroupRef != nil {
			addressGroupRef = models.ResourceIdentifier{
				Name:      existing.AddressGroupRef.Name,
				Namespace: existing.Namespace, // AddressGroup is in same namespace as Network
			}
		}

		// Remove Network from AddressGroup.Networks before deleting
		if existing.AddressGroupRef != nil {
			networkRef := models.ResourceIdentifier{Name: existing.Name, Namespace: existing.Namespace}
			if err := s.removeNetworkFromAddressGroup(ctx, addressGroupRef, networkRef); err != nil {
				return fmt.Errorf("failed to remove network from address group: %w", err)
			}
		}

		// Clear binding references
		existing.BindingRef = nil
		existing.AddressGroupRef = nil
		existing.IsBound = false

		// Update the network to clear bindings
		writer, err := s.repo.Writer(ctx)
		if err != nil {
			return fmt.Errorf("failed to get writer: %w", err)
		}
		defer writer.Abort()

		networks := []models.Network{*existing}
		if err := writer.SyncNetworks(ctx, networks, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
			return fmt.Errorf("failed to sync network cleanup: %w", err)
		}

		if err := writer.Commit(); err != nil {
			return fmt.Errorf("failed to commit network cleanup: %w", err)
		}
	}

	// Delete the network
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	if err := writer.DeleteNetworksByIDs(ctx, []models.ResourceIdentifier{id}); err != nil {
		return fmt.Errorf("failed to delete network: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network deletion: %w", err)
	}

	// CLOUD-233: Removed syncManager.SyncEntity() - SGROUP sync now handled by OutboxWorker
	// Migration 032 DELETE triggers create outbox entries, OutboxWorker processes them asynchronously

	return nil
}

// GetNetwork retrieves a Network by ID
func (s *NetworkResourceService) GetNetwork(ctx context.Context, id models.ResourceIdentifier) (*models.Network, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	network, err := reader.GetNetworkByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get network: %w", err)
	}

	return network, nil
}

// GetAddressGroup retrieves an AddressGroup by ID
func (s *NetworkResourceService) GetAddressGroup(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	addressGroup, err := reader.GetAddressGroupByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get address group: %w", err)
	}

	return addressGroup, nil
}

// UpdateAddressGroup updates an AddressGroup
func (s *NetworkResourceService) UpdateAddressGroup(ctx context.Context, addressGroup *models.AddressGroup) error {
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Convert to slice for sync
	addressGroups := []models.AddressGroup{*addressGroup}
	if err := writer.SyncAddressGroups(ctx, addressGroups, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync address groups: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit address group update: %w", err)
	}

	return nil
}

// ListNetworks retrieves all Networks
func (s *NetworkResourceService) ListNetworks(ctx context.Context, scope ports.Scope) ([]models.Network, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	var networks []models.Network
	err = reader.ListNetworks(ctx, func(network models.Network) error {
		networks = append(networks, network)
		return nil
	}, scope)

	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	return networks, nil
}

// SyncNetworks synchronizes multiple networks with the specified operation
func (s *NetworkResourceService) SyncNetworks(ctx context.Context, networks []models.Network, scope ports.Scope, syncOp models.SyncOp) error {
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	if err = writer.SyncNetworks(ctx, networks, scope, ports.WithSyncOp(syncOp)); err != nil {
		return fmt.Errorf("failed to sync networks: %w", err)
	}

	// CLOUD-233: Removed syncNetworkWithExternal() for both UPSERT and DELETE
	// Migration 027 (UPSERT) and 032 (DELETE) triggers create outbox entries
	// OutboxWorker processes them asynchronously with retry/backoff

	if err = writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// ConditionManager needs the ACTUAL database state, not the in-memory objects
	// Writer may have set Ready=False (PendingSGROUPSync), which must be preserved
	if s.conditionManager != nil && syncOp != models.SyncOpDelete {
		reader, err := s.repo.Reader(ctx)
		if err != nil {
			klog.Errorf("Failed to get reader for batch condition processing: %v", err)
			return nil // Don't fail the operation
		}
		defer reader.Close()

		// Re-read each network from database and process conditions
		for i := range networks {
			freshNetwork, err := reader.GetNetworkByID(ctx, models.ResourceIdentifier{
				Namespace: networks[i].Namespace,
				Name:      networks[i].Name,
			})
			if err != nil {
				klog.Errorf("Failed to re-read network %s/%s for condition processing: %v",
					networks[i].Namespace, networks[i].Name, err)
				continue // Skip this network but continue with others
			}

			// Process conditions with the FRESH network from database
			if err := s.conditionManager.ProcessNetworkConditions(ctx, freshNetwork, nil); err != nil {
				klog.Errorf("Failed to process conditions for %s/%s: %v",
					freshNetwork.Namespace, freshNetwork.Name, err)
				// Continue with other networks
			}
		}
	}

	return nil
}

// ValidateNetworkBinding validates that a NetworkBinding can be created for this Network
func (s *NetworkResourceService) ValidateNetworkBinding(ctx context.Context, networkID models.ResourceIdentifier, bindingID models.ResourceIdentifier) error {

	// Check if Network exists
	network, err := s.getNetworkByID(ctx, networkID.Key())
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if network == nil {
		return fmt.Errorf("network not found: %s", networkID.Key())
	}

	if network.IsBound {
		if network.BindingRef != nil {
			expectedName := bindingID.Name
			actualName := network.BindingRef.Name

			if actualName == expectedName {
				return nil // Network is bound to the same binding - valid
			}
		}
		return fmt.Errorf("network is already bound to another binding (expected: %s, actual: %s)", bindingID.Name, network.BindingRef.Name)
	}

	return nil
}

// ValidateNetworkBindingWithReader validates that a NetworkBinding can be created using provided reader (same session)
func (s *NetworkResourceService) ValidateNetworkBindingWithReader(ctx context.Context, reader ports.Reader, networkID models.ResourceIdentifier, bindingID models.ResourceIdentifier) error {
	// Check if Network exists using provided reader
	network, err := reader.GetNetworkByID(ctx, networkID)
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if network == nil {
		return fmt.Errorf("network not found: %s", networkID.Key())
	}

	// Check if Network is already bound to a different binding
	if network.IsBound {
		// If bound to the same binding, that's valid
		if network.BindingRef != nil {
			// BindingRef.Name now contains only the name part, so compare with binding name
			expectedName := bindingID.Name
			actualName := network.BindingRef.Name

			if actualName == expectedName {
				return nil // Network is bound to the same binding - valid
			}
		}
		return fmt.Errorf("network is already bound to another binding (expected: %s, actual: %s)", bindingID.Name, network.BindingRef.Name)
	}

	// ========================================
	// ========================================
	if !s.isNetworkReady(network) {
		return fmt.Errorf("network is not ready yet (Ready=False): %s - binding can only be created between Ready resources", networkID.Key())
	}

	return nil
}

// UpdateNetworkBinding updates Network status when a binding is created
func (s *NetworkResourceService) UpdateNetworkBinding(ctx context.Context, networkID models.ResourceIdentifier, bindingID models.ResourceIdentifier, addressGroupID models.ResourceIdentifier) error {
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

	// Get the network using the same session reader
	network, err := reader.GetNetworkByID(ctx, networkID)
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if network == nil {
		return fmt.Errorf("network not found: %s", networkID.Key())
	}

	// Update binding references
	network.BindingRef = &v1beta1.ObjectReference{
		APIVersion: "netguard.sgroups.io/v1beta1",
		Kind:       "NetworkBinding",
		Name:       bindingID.Name, // Store only the name part for repository consistency
	}
	network.AddressGroupRef = &v1beta1.ObjectReference{
		APIVersion: "netguard.sgroups.io/v1beta1",
		Kind:       "AddressGroup",
		Name:       addressGroupID.Name,
	}
	network.IsBound = true

	// Update metadata
	network.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	// Set success condition
	utils.SetSyncSuccessCondition(network)

	// Sync the updated network
	networks := []models.Network{*network}
	if err := writer.SyncNetworks(ctx, networks, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync network binding: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network binding: %w", err)
	}

	// CLOUD-233: Removed syncManager.SyncEntity() - SGROUP sync now handled by OutboxWorker
	// Migration 027 triggers create outbox entries, OutboxWorker processes them asynchronously

	return nil
}

// RemoveNetworkBinding removes binding references from Network
func (s *NetworkResourceService) RemoveNetworkBinding(ctx context.Context, networkID models.ResourceIdentifier) error {
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

	// Get the network using the same session reader
	network, err := reader.GetNetworkByID(ctx, networkID)
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if network == nil {
		return fmt.Errorf("network not found: %s", networkID.Key())
	}

	// Clear binding references
	network.BindingRef = nil
	network.AddressGroupRef = nil
	network.IsBound = false

	// Update metadata
	network.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	// Set success condition
	utils.SetSyncSuccessCondition(network)

	// Sync the updated network
	networks := []models.Network{*network}
	if err := writer.SyncNetworks(ctx, networks, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync network unbinding: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network unbinding: %w", err)
	}

	// CLOUD-233: Removed syncManager.SyncEntity() - SGROUP sync now handled by OutboxWorker
	// Migration 027 triggers create outbox entries, OutboxWorker processes them asynchronously

	return nil
}

// Helper methods

// removeNetworkFromAddressGroup removes a network from AddressGroup.Networks field
func (s *NetworkResourceService) removeNetworkFromAddressGroup(ctx context.Context, addressGroupRef, networkRef models.ResourceIdentifier) error {
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
		return nil // AddressGroup doesn't exist, nothing to clean up
	}

	networkName := fmt.Sprintf("%s/%s", networkRef.Namespace, networkRef.Name)

	var updatedNetworks []models.NetworkItem
	found := false
	for _, existingNetwork := range addressGroup.Networks {
		if existingNetwork.Name != networkName {
			updatedNetworks = append(updatedNetworks, existingNetwork)
		} else {
			found = true
		}
	}

	if found {
		addressGroup.Networks = updatedNetworks

		// Update metadata
		addressGroup.Meta.TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

		// Sync the updated AddressGroup (commits to database)
		if err := s.UpdateAddressGroup(ctx, addressGroup); err != nil {
			return fmt.Errorf("failed to update address group: %w", err)
		}
	}

	return nil
}

func (s *NetworkResourceService) getNetworkByID(ctx context.Context, id string) (*models.Network, error) {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Parse namespace/name from id (format: "namespace/name")
	parts := strings.Split(id, "/")
	var resourceID models.ResourceIdentifier
	if len(parts) == 2 {
		resourceID = models.ResourceIdentifier{Namespace: parts[0], Name: parts[1]}
	} else {
		resourceID = models.ResourceIdentifier{Name: id}
	}

	network, err := reader.GetNetworkByID(ctx, resourceID)
	return network, err
}

func (s *NetworkResourceService) validateCIDR(cidr string) error {
	// Basic CIDR validation - in a real implementation, you'd use a proper IP/CIDR library
	if cidr == "" {
		return fmt.Errorf("CIDR cannot be empty")
	}

	// Simple format check
	if !strings.Contains(cidr, "/") {
		return fmt.Errorf("invalid CIDR format: %s", cidr)
	}

	return nil
}

// CLOUD-233: Removed syncNetworkWithExternal() and syncAddressGroupWithExternal()
// These methods performed synchronous SGROUP sync, which is now handled by OutboxWorker
// Migration 027 (Network UPSERT), 032 (Network DELETE) triggers create outbox entries
// OutboxWorker processes them asynchronously with exponential backoff and retry

// ========================================
// ========================================

// isNetworkReady checks if Network has Ready=True condition
func (s *NetworkResourceService) isNetworkReady(network *models.Network) bool {
	if network == nil || network.Meta.Conditions == nil {
		return false
	}
	for _, cond := range network.Meta.Conditions {
		if cond.Type == "Ready" && cond.Status == "True" {
			return true
		}
	}
	return false
}
