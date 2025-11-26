package resources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"netguard-pg-backend/internal/application/utils"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/interfaces"

	"k8s.io/klog/v2"
)

// NetworkBindingConditionManagerInterface provides condition processing for network bindings
type NetworkBindingConditionManagerInterface interface {
	ProcessNetworkBindingConditions(ctx context.Context, binding *models.NetworkBinding) error
}

// NetworkBindingResourceService provides business logic for NetworkBinding resources
type NetworkBindingResourceService struct {
	repo                        ports.Registry
	networkResourceService      *NetworkResourceService
	addressGroupResourceService *AddressGroupResourceService
	syncTracker                 *utils.SyncTracker
	retryConfig                 utils.RetryConfig
	syncManager                 interfaces.SyncManager
	conditionManager            NetworkBindingConditionManagerInterface
}

// NewNetworkBindingResourceService creates a new NetworkBindingResourceService
func NewNetworkBindingResourceService(
	repo ports.Registry,
	networkResourceService *NetworkResourceService,
	addressGroupResourceService *AddressGroupResourceService,
	syncManager interfaces.SyncManager,
	conditionManager NetworkBindingConditionManagerInterface,
) *NetworkBindingResourceService {
	return &NetworkBindingResourceService{
		repo:                        repo,
		networkResourceService:      networkResourceService,
		addressGroupResourceService: addressGroupResourceService,
		syncTracker:                 utils.NewSyncTracker(1 * time.Second),
		retryConfig:                 utils.DefaultRetryConfig(),
		syncManager:                 syncManager,
		conditionManager:            conditionManager,
	}
}

// CreateNetworkBinding creates a new NetworkBinding with business logic validation
func (s *NetworkBindingResourceService) CreateNetworkBinding(ctx context.Context, binding *models.NetworkBinding) error {
	networkRef := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
	addressGroupRef := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.Namespace}

	// Don't call TouchOnCreate() here - let upsertNetworkBinding handle UID generation
	// This ensures UID comes from database (uuid_generate_v4()) and matches Outbox entry
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	reader, err := s.repo.ReaderFromWriter(ctx, writer)
	if err != nil {
		return fmt.Errorf("failed to get reader from writer: %w", err)
	}
	defer reader.Close()

	bindingID := models.ResourceIdentifier{Name: binding.Name, Namespace: binding.Namespace}
	if err := s.networkResourceService.ValidateNetworkBindingWithReader(ctx, reader, networkRef, bindingID); err != nil {
		return fmt.Errorf("network validation failed: %w", err)
	}

	if err := s.validateAddressGroupWithReader(ctx, reader, addressGroupRef); err != nil {
		return fmt.Errorf("address group validation failed: %w", err)
	}

	existing, err := s.getNetworkBindingByIDWithReader(ctx, reader, binding.Key())
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("failed to check existing network binding: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("network binding already exists: %s", binding.Key())
	}

	log.Printf("[NetworkBindingService] create start binding=%s network=%s addressGroup=%s",
		binding.Key(), networkRef.Key(), addressGroupRef.Key())
	bindings := []models.NetworkBinding{*binding}
	if err := writer.SyncNetworkBindings(ctx, bindings, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync network bindings: %w", err)
	}

	if err := s.updateAddressGroupNetworks(ctx, writer, addressGroupRef, networkRef, true); err != nil {
		return fmt.Errorf("failed to refresh address group networks: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network binding creation: %w", err)
	}

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

	log.Printf("[NetworkBindingService] create update-network binding=%s network=%s addressGroup=%s",
		binding.Key(), networkRef.Key(), addressGroupRef.Key())
	if err := s.networkResourceService.UpdateNetworkBinding(ctx, networkRef, bindingID, addressGroupRef); err != nil {
		return fmt.Errorf("failed to update network binding: %w", err)
	}

	if err := s.forceSyncAddressGroupWithSGroups(ctx, addressGroupRef); err != nil {
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

	networkRef := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
	addressGroupRef := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.Namespace}
	if err := s.validateNetwork(ctx, networkRef); err != nil {
		return fmt.Errorf("network validation failed: %w", err)
	}

	if err := s.validateAddressGroup(ctx, addressGroupRef); err != nil {
		return fmt.Errorf("address group validation failed: %w", err)
	}

	if existing.NetworkRef.Name != binding.NetworkRef.Name || existing.AddressGroupRef.Name != binding.AddressGroupRef.Name {
		existingNetworkRef := models.ResourceIdentifier{Name: existing.NetworkRef.Name, Namespace: existing.Namespace}
		existingAddressGroupRef := models.ResourceIdentifier{Name: existing.AddressGroupRef.Name, Namespace: existing.Namespace}

		log.Printf("[NetworkBindingService] update removing old binding=%s oldNetwork=%s oldAddressGroup=%s",
			binding.Key(), existingNetworkRef.Key(), existingAddressGroupRef.Key())
		if err := s.networkResourceService.RemoveNetworkBinding(ctx, existingNetworkRef); err != nil {
			return fmt.Errorf("failed to remove old network binding: %w", err)
		}
		if err := s.updateAddressGroupNetworks(ctx, nil, existingAddressGroupRef, existingNetworkRef, false); err != nil {
			return fmt.Errorf("failed to refresh previous address group networks: %w", err)
		}

		bindingID := models.ResourceIdentifier{Name: binding.Name, Namespace: binding.Namespace}
		if err := s.networkResourceService.ValidateNetworkBinding(ctx, networkRef, bindingID); err != nil {
			return fmt.Errorf("new network validation failed: %w", err)
		}

		log.Printf("[NetworkBindingService] update new association binding=%s network=%s addressGroup=%s",
			binding.Key(), networkRef.Key(), addressGroupRef.Key())
		if err := s.networkResourceService.UpdateNetworkBinding(ctx, networkRef, bindingID, addressGroupRef); err != nil {
			return fmt.Errorf("failed to update new network binding: %w", err)
		}

		if err := s.updateAddressGroupNetworks(ctx, nil, addressGroupRef, networkRef, true); err != nil {
			return fmt.Errorf("failed to refresh new address group networks: %w", err)
		}

		if err := s.forceSyncAddressGroupWithSGroups(ctx, existingAddressGroupRef); err != nil {
		}
		if err := s.forceSyncAddressGroupWithSGroups(ctx, addressGroupRef); err != nil {
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

	// Check if NetworkBinding exists
	existing, err := s.getNetworkBindingByID(ctx, id.Key())
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("failed to get network binding: %w", err)
	}
	if existing == nil || errors.Is(err, ports.ErrNotFound) {
		// Network binding doesn't exist - delete is idempotent, so this is success
		return nil
	}

	// Convert ObjectReference to ResourceIdentifier
	networkRef := models.ResourceIdentifier{Name: existing.NetworkRef.Name, Namespace: existing.Namespace}
	addressGroupRef := models.ResourceIdentifier{Name: existing.AddressGroupRef.Name, Namespace: existing.Namespace}

	// Remove binding from Network
	log.Printf("[NetworkBindingService] delete binding=%s network=%s addressGroup=%s",
		id.Key(), networkRef.Key(), addressGroupRef.Key())
	if err := s.networkResourceService.RemoveNetworkBinding(ctx, networkRef); err != nil {
		return fmt.Errorf("failed to remove network binding: %w", err)
	}

	// Remove network reference from AddressGroup within the same transaction
	// that will delete the binding record to keep watch events consistent.
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	if err := s.updateAddressGroupNetworks(ctx, writer, addressGroupRef, networkRef, false); err != nil {
		return fmt.Errorf("failed to refresh address group networks: %w", err)
	}

	if err := writer.DeleteNetworkBindingsByIDs(ctx, []models.ResourceIdentifier{id}); err != nil {
		return fmt.Errorf("failed to delete network binding: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit network binding deletion: %w", err)
	}

	if err := s.forceSyncAddressGroupWithSGroups(ctx, addressGroupRef); err != nil {
	}

	// Sync deletion with external systems
	err = s.syncNetworkBindingWithExternal(ctx, existing, "delete")
	if err != nil {
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

// SyncNetworkBindings synchronizes multiple network bindings with the specified operation
func (s *NetworkBindingResourceService) SyncNetworkBindings(ctx context.Context, bindings []models.NetworkBinding, scope ports.Scope, syncOp models.SyncOp) error {
	// Get writer from registry
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Call writer.SyncNetworkBindings directly with the bindings and syncOp
	if err = writer.SyncNetworkBindings(ctx, bindings, scope, ports.WithSyncOp(syncOp)); err != nil {
		return fmt.Errorf("failed to sync network bindings: %w", err)
	}

	// Commit transaction
	if err = writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	for i := range bindings {
		binding := &bindings[i]

		networkRef := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
		addressGroupRef := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.Namespace}

		if syncOp == models.SyncOpDelete {
			if err := s.networkResourceService.RemoveNetworkBinding(ctx, networkRef); err != nil {
				klog.Errorf("Failed to remove Network binding for %s: %v", networkRef.Key(), err)
			}

			if err := s.forceSyncAddressGroupWithSGroups(ctx, addressGroupRef); err != nil {
				klog.Errorf("Failed to sync AddressGroup %s after deletion: %v", addressGroupRef.Key(), err)
			}

			continue
		}

		if err := s.forceSyncNetworkWithSGroups(ctx, networkRef); err != nil {
			klog.Errorf("Failed to sync Network %s with SGROUP: %v", networkRef.Key(), err)
		}

		if err := s.networkResourceService.UpdateNetworkBinding(ctx, networkRef, binding.ResourceIdentifier, addressGroupRef); err != nil {
			klog.Errorf("Failed to update Network binding info for %s: %v", networkRef.Key(), err)
		}

		if err := s.forceSyncAddressGroupWithSGroups(ctx, addressGroupRef); err != nil {
			klog.Errorf("Failed to sync AddressGroup %s with SGROUP: %v", addressGroupRef.Key(), err)
		}

		if s.conditionManager != nil {
			if err := s.conditionManager.ProcessNetworkBindingConditions(ctx, binding); err != nil {
				klog.Errorf("Failed to process network binding conditions for %s/%s: %v",
					binding.Namespace, binding.Name, err)
			} else {
				if err := s.saveNetworkBindingConditions(ctx, binding); err != nil {
					klog.Errorf("Failed to save network binding conditions for %s/%s: %v",
						binding.Namespace, binding.Name, err)
				}
			}
		}
	}

	return nil
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

	if !s.isAddressGroupReady(addressGroup) {
		return fmt.Errorf("address group is not ready yet (Ready=False): %s - binding can only be created between Ready resources", addressGroupRef.Key())
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

func (s *NetworkBindingResourceService) updateAddressGroupNetworks(ctx context.Context, txWriter ports.Writer, addressGroupRef, networkRef models.ResourceIdentifier, add bool) error {
	if addressGroupRef.Name == "" {
		return nil
	}

	log.Printf("[NetworkBindingService] refresh addressGroup=%s network=%s add=%t reuseWriter=%t",
		addressGroupRef.Key(), networkRef.Key(), add, txWriter != nil)

	writer := txWriter
	ownWriter := false
	if writer == nil {
		var err error
		writer, err = s.repo.Writer(ctx)
		if err != nil {
			return fmt.Errorf("failed to get writer for address group touch: %w", err)
		}
		ownWriter = true
		defer writer.Abort()
	}

	reader, err := s.repo.ReaderFromWriter(ctx, writer)
	if err != nil {
		return fmt.Errorf("failed to get reader from writer: %w", err)
	}
	defer reader.Close()

	addressGroup, err := reader.GetAddressGroupByID(ctx, addressGroupRef)
	if err != nil {
		return fmt.Errorf("failed to load address group %s: %w", addressGroupRef.Key(), err)
	}
	if addressGroup == nil {
		return fmt.Errorf("address group not found: %s", addressGroupRef.Key())
	}

	updatedNetworks, changed, err := s.applyNetworkMembershipChange(ctx, reader, addressGroup.Networks, addressGroupRef, networkRef, add)
	if err != nil {
		return err
	}
	if !changed {
		log.Printf("[NetworkBindingService] address group unchanged addressGroup=%s network=%s add=%t",
			addressGroupRef.Key(), networkRef.Key(), add)
		if ownWriter {
			writer.Abort()
		}
		return nil
	}
	addressGroup.Networks = updatedNetworks
	addressGroup.Meta.TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	if err := writer.SyncAddressGroups(ctx, []models.AddressGroup{*addressGroup}, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync address group %s: %w", addressGroupRef.Key(), err)
	}
	log.Printf("[NetworkBindingService] address group updated addressGroup=%s network=%s add=%t networkCount=%d",
		addressGroupRef.Key(), networkRef.Key(), add, len(addressGroup.Networks))

	if ownWriter {
		if err := writer.Commit(); err != nil {
			return fmt.Errorf("failed to commit address group touch for %s: %w", addressGroupRef.Key(), err)
		}
	}

	log.Printf("[NetworkBindingService] address group touch completed addressGroup=%s network=%s add=%t",
		addressGroupRef.Key(), networkRef.Key(), add)
	return nil
}

func (s *NetworkBindingResourceService) applyNetworkMembershipChange(
	ctx context.Context,
	reader ports.Reader,
	current []models.NetworkItem,
	addressGroupRef models.ResourceIdentifier,
	networkRef models.ResourceIdentifier,
	add bool,
) ([]models.NetworkItem, bool, error) {
	if add {
		network, err := reader.GetNetworkByID(ctx, networkRef)
		if err != nil {
			return nil, false, fmt.Errorf("failed to load network %s: %w", networkRef.Key(), err)
		}
		if network == nil {
			return nil, false, fmt.Errorf("network not found: %s", networkRef.Key())
		}

		item := models.NetworkItem{
			Name:       fmt.Sprintf("%s/%s", network.Namespace, network.Name),
			CIDR:       network.CIDR,
			ApiVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Network",
			Namespace:  network.Namespace,
		}

		for _, existing := range current {
			if existing.Name == item.Name {
				return current, false, nil
			}
		}
		return append(current, item), true, nil
	}

	targetName := fmt.Sprintf("%s/%s", networkRef.Namespace, networkRef.Name)
	var updated []models.NetworkItem
	changed := false
	for _, existing := range current {
		if existing.Name == targetName {
			changed = true
			continue
		}
		updated = append(updated, existing)
	}
	if !changed {
		return current, false, nil
	}
	return updated, true, nil
}

func (s *NetworkBindingResourceService) syncNetworkBindingWithExternal(ctx context.Context, binding *models.NetworkBinding, operation string) error {
	syncKey := fmt.Sprintf("%s-%s", operation, binding.GetID())

	// Check debouncing
	if !s.syncTracker.ShouldSync(syncKey) {
		return nil // Skip sync due to debouncing
	}

	// Execute sync with retry
	err := utils.ExecuteWithRetry(ctx, s.retryConfig, func() error {
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
	if operation != "delete" {
		utils.SetSyncSuccessCondition(binding)
	} else {
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

	scope := ports.EmptyScope{}

	// Sync the NetworkBinding with updated conditions (CONDITION-ONLY update - no Outbox creation)
	if err := writer.SyncNetworkBindings(ctx, []models.NetworkBinding{*binding}, scope, ports.ConditionOnlyOperation{}); err != nil {
		return fmt.Errorf("failed to sync NetworkBinding with conditions: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit NetworkBinding conditions: %w", err)
	}

	return nil
}

func (s *NetworkBindingResourceService) forceSyncNetworkWithSGroups(ctx context.Context, networkRef models.ResourceIdentifier) error {
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return fmt.Errorf("failed to get reader: %w", err)
	}

	freshNetwork, err := reader.GetNetworkByID(ctx, networkRef)
	if err != nil {
		return fmt.Errorf("failed to get Network %s: %w", networkRef.Key(), err)
	}

	_ = freshNetwork

	return nil
}

func (s *NetworkBindingResourceService) forceSyncAddressGroupWithSGroups(ctx context.Context, addressGroupRef models.ResourceIdentifier) error {
	if addressGroupRef.Name == "" {
		return nil
	}

	// Prefer to reuse the shared AddressGroup service so that we bump resourceVersion the same
	// way HostBinding flows do (ConditionOnly sync inside a writer transaction).
	if s.addressGroupResourceService != nil {
		writer, err := s.repo.Writer(ctx)
		if err != nil {
			return fmt.Errorf("failed to get writer for address group touch: %w", err)
		}
		defer writer.Abort()

		if err := s.addressGroupResourceService.TouchAddressGroupBindingMembership(ctx, writer, addressGroupRef); err != nil {
			return fmt.Errorf("failed to touch address group %s: %w", addressGroupRef.Key(), err)
		}

		if err := writer.Commit(); err != nil {
			return fmt.Errorf("failed to commit address group touch for %s: %w", addressGroupRef.Key(), err)
		}
		return nil
	}

	// Fallback: ensure the row exists so triggers such as ensure_resource_version_bump()
	// still have a chance to run via other updates.
	if _, err := s.getFreshAddressGroupFromDatabase(ctx, addressGroupRef); err != nil {
		return fmt.Errorf("failed to get AddressGroup %s: %w", addressGroupRef.Key(), err)
	}

	return nil
}

// isAddressGroupReady checks if AddressGroup has Ready=True condition
func (s *NetworkBindingResourceService) isAddressGroupReady(ag *models.AddressGroup) bool {
	if ag == nil || ag.Meta.Conditions == nil {
		return false
	}
	for _, cond := range ag.Meta.Conditions {
		if cond.Type == "Ready" && cond.Status == "True" {
			return true
		}
	}
	return false
}
