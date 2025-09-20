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
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// HostBindingConditionManagerInterface provides condition processing for host bindings
type HostBindingConditionManagerInterface interface {
	ProcessHostBindingConditions(ctx context.Context, hostBinding *models.HostBinding, syncResult error) error
}

// HostBindingResourceService provides business logic for HostBinding resources
type HostBindingResourceService struct {
	repo                        ports.Registry
	hostResourceService         *HostResourceService
	addressGroupResourceService *AddressGroupResourceService
	syncTracker                 *utils.SyncTracker
	retryConfig                 utils.RetryConfig
	syncManager                 interfaces.SyncManager
	conditionManager            HostBindingConditionManagerInterface
}

// NewHostBindingResourceService creates a new HostBindingResourceService
func NewHostBindingResourceService(
	repo ports.Registry,
	hostResourceService *HostResourceService,
	addressGroupResourceService *AddressGroupResourceService,
	syncManager interfaces.SyncManager,
	conditionManager HostBindingConditionManagerInterface,
) *HostBindingResourceService {
	return &HostBindingResourceService{
		repo:                        repo,
		hostResourceService:         hostResourceService,
		addressGroupResourceService: addressGroupResourceService,
		syncTracker:                 utils.NewSyncTracker(1 * time.Second),
		retryConfig:                 utils.DefaultRetryConfig(),
		syncManager:                 syncManager,
		conditionManager:            conditionManager,
	}
}

// CreateHostBinding creates a new HostBinding with business logic validation
func (s *HostBindingResourceService) CreateHostBinding(ctx context.Context, hostBinding *models.HostBinding) error {
	hostRef := models.ResourceIdentifier{Name: hostBinding.HostRef.Name, Namespace: hostBinding.HostRef.Namespace}
	addressGroupRef := models.ResourceIdentifier{Name: hostBinding.AddressGroupRef.Name, Namespace: hostBinding.AddressGroupRef.Namespace}

	// Initialize metadata
	hostBinding.GetMeta().TouchOnCreate()

	// Create the host binding
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

	// Validate that the referenced Host exists and is not already bound
	bindingID := models.ResourceIdentifier{Name: hostBinding.Name, Namespace: hostBinding.Namespace}
	if err := s.validateHostBindingWithReader(ctx, reader, hostRef, bindingID); err != nil {
		return fmt.Errorf("host validation failed: %w", err)
	}

	// Validate that the referenced AddressGroup exists
	if err := s.validateAddressGroupWithReader(ctx, reader, addressGroupRef); err != nil {
		return fmt.Errorf("address group validation failed: %w", err)
	}

	// Check if HostBinding already exists
	existing, err := s.getHostBindingByIDWithReader(ctx, reader, hostBinding.Key())
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("failed to check existing host binding: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("host binding already exists: %s", hostBinding.Key())
	}

	// Convert to slice for sync
	bindings := []models.HostBinding{*hostBinding}
	if err := writer.SyncHostBindings(ctx, bindings, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to sync host bindings: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit host binding creation: %w", err)
	}

	if err := s.hostResourceService.UpdateHostBinding(ctx, hostRef, bindingID, addressGroupRef); err != nil {
		log.Printf("❌ DEBUG: Failed to update host binding status for %s: %v", hostRef.Key(), err)
	}

	// Process conditions after binding operations
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessHostBindingConditions(ctx, hostBinding, nil); err != nil {
			log.Printf("⚠️ DEBUG: Failed to process conditions for host binding %s: %v", hostBinding.Key(), err)
		}
	}

	return nil
}

// UpdateHostBinding updates an existing HostBinding
func (s *HostBindingResourceService) UpdateHostBinding(ctx context.Context, hostBinding *models.HostBinding) error {

	// Validate host binding
	if hostBinding == nil {
		return errors.New("host binding cannot be nil")
	}

	if err := s.validateHostBinding(hostBinding); err != nil {
		log.Printf("❌ DEBUG: HostBinding validation failed for %s: %v", hostBinding.Key(), err)
		return fmt.Errorf("host binding validation failed: %w", err)
	}

	// Get reader to validate existing host binding
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	// Check if HostBinding exists
	existing, err := s.getHostBindingByIDWithReader(ctx, reader, hostBinding.Key())
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("host binding not found: %s", hostBinding.Key())
		}
		return fmt.Errorf("failed to get existing host binding: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("host binding not found: %s", hostBinding.Key())
	}

	// Convert ResourceIdentifiers for validation
	hostRef := models.ResourceIdentifier{Name: hostBinding.HostRef.Name, Namespace: hostBinding.HostRef.Namespace}
	addressGroupRef := models.ResourceIdentifier{Name: hostBinding.AddressGroupRef.Name, Namespace: hostBinding.AddressGroupRef.Namespace}
	bindingID := models.ResourceIdentifier{Name: hostBinding.Name, Namespace: hostBinding.Namespace}

	// Validate that the referenced Host exists (allow same binding)
	if err := s.validateHostBindingWithReader(ctx, reader, hostRef, bindingID); err != nil {
		return fmt.Errorf("host validation failed: %w", err)
	}

	// Validate that the referenced AddressGroup exists
	if err := s.validateAddressGroupWithReader(ctx, reader, addressGroupRef); err != nil {
		return fmt.Errorf("address group validation failed: %w", err)
	}

	// Update metadata
	hostBinding.GetMeta().TouchOnWrite(fmt.Sprintf("%d", time.Now().UnixNano()))

	// Get writer and perform update
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Convert to slice for sync
	bindings := []models.HostBinding{*hostBinding}
	if err := writer.SyncHostBindings(ctx, bindings, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return fmt.Errorf("failed to update host binding: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Process conditions if condition manager is available
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessHostBindingConditions(ctx, hostBinding, nil); err != nil {
			log.Printf("⚠️ DEBUG: Failed to process conditions for host binding %s: %v", hostBinding.Key(), err)
			// Don't fail the update due to condition processing errors
		}
	}

	return nil
}

// DeleteHostBinding deletes a HostBinding by resource identifier
func (s *HostBindingResourceService) DeleteHostBinding(ctx context.Context, id models.ResourceIdentifier) error {
	writer, err := s.repo.Writer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get writer: %w", err)
	}
	defer writer.Abort()

	// Check if HostBinding exists before deletion
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	existingBinding, err := s.getHostBindingByIDWithReader(ctx, reader, id.Key())
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("host binding not found: %s", id.Key())
		}
		return fmt.Errorf("failed to get existing host binding: %w", err)
	}
	if existingBinding == nil {
		return fmt.Errorf("host binding not found: %s", id.Key())
	}

	// Delete the HostBinding using the SyncHostBindings with DELETE operation
	if err := writer.SyncHostBindings(ctx, []models.HostBinding{*existingBinding}, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpDelete)); err != nil {
		return fmt.Errorf("failed to delete host binding: %w", err)
	}

	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit deletion transaction: %w", err)
	}

	log.Printf("✅ DEBUG: HostBinding %s deleted successfully from storage", id.Key())

	hostID := models.ResourceIdentifier{
		Namespace: existingBinding.HostRef.Namespace,
		Name:      existingBinding.HostRef.Name,
	}

	// Get reader to load the Host
	readerForHost, err := s.repo.Reader(ctx)
	if err != nil {
		log.Printf("⚠️ DEBUG: Failed to get reader for Host update: %v", err)
	} else {
		defer readerForHost.Close()

		host, err := readerForHost.GetHostByID(ctx, hostID)
		if err != nil {
			log.Printf("⚠️ DEBUG: Failed to load Host %s for update: %v", hostID.Key(), err)
		} else {
			host.IsBound = false
			host.BindingRef = nil
			host.AddressGroupRef = nil
			host.AddressGroupName = ""

			// Save updated Host to storage
			writerForHost, err := s.repo.Writer(ctx)
			if err != nil {
				log.Printf("⚠️ DEBUG: Failed to get writer for Host update: %v", err)
			} else {
				defer writerForHost.Abort()

				// Use SyncHosts with UPSERT to update the Host
				if err := writerForHost.SyncHosts(ctx, []models.Host{*host}, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
					log.Printf("⚠️ DEBUG: Failed to save updated Host %s: %v", host.Key(), err)
				} else {
					if err := writerForHost.Commit(); err != nil {
						log.Printf("⚠️ DEBUG: Failed to commit Host update: %v", err)
					} else {
						if err := s.hostResourceService.syncManager.SyncEntity(ctx, host, types.SyncOperationUpsert); err != nil {
							log.Printf("⚠️ DEBUG: Failed to sync updated Host %s with SGROUP: %v", host.Key(), err)
						}
					}
				}
			}
		}
	}

	// Get the affected AddressGroup
	addressGroupID := models.ResourceIdentifier{
		Namespace: existingBinding.AddressGroupRef.Namespace,
		Name:      existingBinding.AddressGroupRef.Name,
	}

	// Get reader to load the AddressGroup
	readerForAG, err := s.repo.Reader(ctx)
	if err != nil {
		log.Printf("⚠️ DEBUG: Failed to get reader for AddressGroup sync: %v", err)
		// Don't fail the entire operation, just log the warning
	} else {
		defer readerForAG.Close()

		// Load the AddressGroup
		addressGroup, err := readerForAG.GetAddressGroupByID(ctx, addressGroupID)
		if err != nil {
			log.Printf("⚠️ DEBUG: Failed to load AddressGroup %s for sync: %v", addressGroupID.Key(), err)
		} else {
			if err := s.addressGroupResourceService.syncManager.SyncEntity(ctx, addressGroup, types.SyncOperationUpsert); err != nil {
				log.Printf("⚠️ DEBUG: Failed to sync AddressGroup %s after HostBinding deletion: %v", addressGroup.Key(), err)
			}
		}
	}

	return nil
}

// GetHostBinding retrieves a HostBinding by resource identifier
func (s *HostBindingResourceService) GetHostBinding(ctx context.Context, id models.ResourceIdentifier) (*models.HostBinding, error) {

	// Get reader to retrieve host binding
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	// Use the helper method to get host binding by ID
	hostBinding, err := s.getHostBindingByIDWithReader(ctx, reader, id.Key())
	if err != nil {
		return nil, err
	}

	if hostBinding == nil {
		return nil, ports.ErrNotFound
	}

	return hostBinding, nil
}

// ListHostBindings retrieves all HostBindings within a scope
func (s *HostBindingResourceService) ListHostBindings(ctx context.Context, scope ports.Scope) ([]models.HostBinding, error) {

	// Get reader to list host bindings
	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	// List to collect host bindings
	var hostBindings []models.HostBinding

	// Use the repository's ListHostBindings method with a consume function
	err = reader.ListHostBindings(ctx, func(binding models.HostBinding) error {
		hostBindings = append(hostBindings, binding)
		return nil
	}, scope)

	if err != nil {
		return nil, fmt.Errorf("failed to list host bindings: %w", err)
	}

	return hostBindings, nil
}

// SyncHostBindings synchronizes multiple host bindings with the specified operation
func (s *HostBindingResourceService) SyncHostBindings(ctx context.Context, hostBindings []models.HostBinding, scope ports.Scope, syncOp models.SyncOp) error {
	log.Printf("🔥 DEBUG: HostBindingResourceService.SyncHostBindings called with %d host bindings, syncOp=%v", len(hostBindings), syncOp)

	switch syncOp {
	case models.SyncOpFullSync:
		return s.fullSyncHostBindings(ctx, hostBindings, scope)
	case models.SyncOpUpsert:
		return s.upsertHostBindings(ctx, hostBindings)
	case models.SyncOpDelete:
		return s.deleteHostBindings(ctx, hostBindings)
	default:
		return fmt.Errorf("unsupported sync operation: %v", syncOp)
	}
}

// fullSyncHostBindings performs a full synchronization of host bindings
func (s *HostBindingResourceService) fullSyncHostBindings(ctx context.Context, hostBindings []models.HostBinding, scope ports.Scope) error {
	log.Printf("🔥 DEBUG: Starting full sync of %d host bindings", len(hostBindings))

	return s.upsertHostBindings(ctx, hostBindings)
}

// upsertHostBindings creates or updates multiple host bindings
func (s *HostBindingResourceService) upsertHostBindings(ctx context.Context, hostBindings []models.HostBinding) error {
	log.Printf("🔥 DEBUG: Upserting %d host bindings", len(hostBindings))

	for _, hostBinding := range hostBindings {
		if err := s.CreateHostBinding(ctx, &hostBinding); err != nil {
			return fmt.Errorf("failed to create host binding %s: %w", hostBinding.Key(), err)
		}
	}

	return nil
}

// deleteHostBindings deletes multiple host bindings
func (s *HostBindingResourceService) deleteHostBindings(ctx context.Context, hostBindings []models.HostBinding) error {
	for _, hostBinding := range hostBindings {
		if err := s.DeleteHostBinding(ctx, hostBinding.SelfRef.ResourceIdentifier); err != nil {
			return fmt.Errorf("failed to delete host binding %s: %w", hostBinding.Key(), err)
		}
	}

	return nil
}

// validateHostBinding performs business logic validation on a host binding
func (s *HostBindingResourceService) validateHostBinding(hostBinding *models.HostBinding) error {
	if hostBinding == nil {
		return errors.New("host binding cannot be nil")
	}

	// Validate resource identifier
	if err := s.validateResourceIdentifier(hostBinding.SelfRef.ResourceIdentifier); err != nil {
		return fmt.Errorf("invalid resource identifier: %w", err)
	}

	// Validate host reference
	if err := s.validateNamespacedObjectReference(hostBinding.HostRef, "Host"); err != nil {
		return fmt.Errorf("invalid host reference: %w", err)
	}

	// Validate address group reference
	if err := s.validateNamespacedObjectReference(hostBinding.AddressGroupRef, "AddressGroup"); err != nil {
		return fmt.Errorf("invalid address group reference: %w", err)
	}

	// Additional business logic validation can be added here

	return nil
}

func (s *HostBindingResourceService) validateResourceIdentifier(id models.ResourceIdentifier) error {
	if id.Name == "" {
		return errors.New("resource name cannot be empty")
	}

	if id.Namespace == "" {
		return errors.New("resource namespace cannot be empty")
	}

	return nil
}

func (s *HostBindingResourceService) validateNamespacedObjectReference(ref v1beta1.NamespacedObjectReference, expectedKind string) error {
	if ref.Name == "" {
		return fmt.Errorf("%s reference name cannot be empty", expectedKind)
	}

	if ref.Namespace == "" {
		return fmt.Errorf("%s reference namespace cannot be empty", expectedKind)
	}

	if ref.Kind != expectedKind {
		return fmt.Errorf("expected %s reference kind to be %s, got %s", expectedKind, expectedKind, ref.Kind)
	}

	return nil
}

// SyncStatusUpdate handles sync status updates for host bindings
func (s *HostBindingResourceService) SyncStatusUpdate(ctx context.Context, resourceType string, status interface{}) error {
	if resourceType != "HostBinding" {
		return nil
	}

	log.Printf("🔥 DEBUG: HostBindingResourceService received sync status update: %v", status)

	// TODO: Implement sync status update when interface is clarified
	log.Printf("⚠️ DEBUG: Sync status update not yet implemented for host bindings")

	return nil
}

// validateHostBindingWithReader validates that a Host can be bound and is not already bound to another binding
func (s *HostBindingResourceService) validateHostBindingWithReader(ctx context.Context, reader ports.Reader, hostID models.ResourceIdentifier, bindingID models.ResourceIdentifier) error {
	// Check if Host exists using provided reader
	host, err := reader.GetHostByID(ctx, hostID)
	if err != nil {
		return fmt.Errorf("failed to get host: %w", err)
	}
	if host == nil {
		return fmt.Errorf("host not found: %s", hostID.Key())
	}

	// Check if Host is already bound to a different binding
	if host.IsBound {
		// If bound to the same binding, that's valid
		if host.BindingRef != nil {
			expectedName := bindingID.Name
			actualName := host.BindingRef.Name
			log.Printf("🔍 HostBinding validation: comparing expectedName='%s' with actualName='%s'", expectedName, actualName)

			if actualName == expectedName {
				return nil
			}
			log.Printf("❌ HostBinding validation: Host is bound to DIFFERENT binding - expectedName='%s' vs actualName='%s'", expectedName, actualName)
			return fmt.Errorf("host is already bound to another binding (expected: %s, actual: %s)", bindingID.Name, actualName)
		} else {
			// Host is bound but BindingRef is nil - means it's bound via AddressGroup.spec.hosts
			log.Printf("❌ HostBinding validation: Host is bound via AddressGroup.spec.hosts - cannot create HostBinding")
			return fmt.Errorf("host is already bound to AddressGroup via spec.hosts - cannot create HostBinding")
		}
	}

	return nil
}

// validateAddressGroupWithReader validates that an AddressGroup exists
func (s *HostBindingResourceService) validateAddressGroupWithReader(ctx context.Context, reader ports.Reader, addressGroupID models.ResourceIdentifier) error {
	// Check if AddressGroup exists using provided reader
	addressGroup, err := reader.GetAddressGroupByID(ctx, addressGroupID)
	if err != nil {
		return fmt.Errorf("failed to get address group: %w", err)
	}
	if addressGroup == nil {
		return fmt.Errorf("address group not found: %s", addressGroupID.Key())
	}

	return nil
}

// getHostBindingByIDWithReader retrieves a host binding by its key using provided reader
func (s *HostBindingResourceService) getHostBindingByIDWithReader(ctx context.Context, reader ports.Reader, id string) (*models.HostBinding, error) {

	// Parse namespace/name from id (format: "namespace/name")
	parts := strings.Split(id, "/")
	var resourceID models.ResourceIdentifier
	if len(parts) == 2 {
		resourceID = models.ResourceIdentifier{Namespace: parts[0], Name: parts[1]}
		log.Printf("🔍 DEBUG: getHostBindingByIDWithReader parsed resourceID: namespace=%s, name=%s", parts[0], parts[1])
	} else {
		resourceID = models.ResourceIdentifier{Name: id}
		log.Printf("🔍 DEBUG: getHostBindingByIDWithReader using single name: %s", id)
	}

	hostBinding, err := reader.GetHostBindingByID(ctx, resourceID)
	return hostBinding, err
}

// getHostBindingByHostID finds a HostBinding that binds the specified Host
func (s *HostBindingResourceService) getHostBindingByHostID(ctx context.Context, hostID models.ResourceIdentifier) (*models.HostBinding, error) {
	log.Printf("🔍 DEBUG: getHostBindingByHostID called for host %s", hostID.Key())

	reader, err := s.repo.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	defer reader.Close()

	// Use ListHostBindings to find the binding for this host
	var foundBinding *models.HostBinding
	err = reader.ListHostBindings(ctx, func(hostBinding models.HostBinding) error {
		// Check if this binding binds our target host
		if hostBinding.HostRef.Namespace == hostID.Namespace && hostBinding.HostRef.Name == hostID.Name {
			foundBinding = &hostBinding
			return nil // Found it, continue to collect (though there should only be one due to UNIQUE constraint)
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		log.Printf("❌ DEBUG: Failed to list host bindings: %v", err)
		return nil, fmt.Errorf("failed to list host bindings: %w", err)
	}

	if foundBinding == nil {
		log.Printf("ℹ️ DEBUG: No HostBinding found for host %s", hostID.Key())
		return nil, ports.ErrNotFound
	}

	log.Printf("✅ DEBUG: Successfully found HostBinding %s for host %s", foundBinding.Key(), hostID.Key())
	return foundBinding, nil
}
