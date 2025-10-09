package resources

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/application/utils"
	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// ServiceConditionManagerInterface provides condition processing for services and related resources
type ServiceConditionManagerInterface interface {
	ProcessServiceConditions(ctx context.Context, service *models.Service) error
	ProcessServiceAliasConditions(ctx context.Context, alias *models.ServiceAlias) error
	ProcessAddressGroupBindingConditions(ctx context.Context, binding *models.AddressGroupBinding) error
}

// AddressGroupPortMappingRegenerator provides the ability to regenerate port mappings when service ports change
type AddressGroupPortMappingRegenerator interface {
	RegeneratePortMappingsForService(ctx context.Context, serviceID models.ResourceIdentifier) error
	RegeneratePortMappingsForAddressGroup(ctx context.Context, addressGroupID models.ResourceIdentifier) error
}

// RuleS2SRegenerator interface is now defined in interfaces.go to avoid circular dependencies

// ServiceResourceService handles Service and ServiceAlias operations
type ServiceResourceService struct {
	registry               ports.Registry
	syncManager            interfaces.SyncManager
	conditionManager       ServiceConditionManagerInterface
	portMappingRegenerator AddressGroupPortMappingRegenerator // Optional - for port mapping updates
	ruleS2SRegenerator     RuleS2SRegenerator                 // Optional - for IEAgAg rule updates
	syncTracker            *utils.SyncTracker
	retryConfig            utils.RetryConfig
}

// NewServiceResourceService creates a new ServiceResourceService
func NewServiceResourceService(
	registry ports.Registry,
	syncManager interfaces.SyncManager,
	conditionManager ServiceConditionManagerInterface,
) *ServiceResourceService {
	return &ServiceResourceService{
		registry:               registry,
		syncManager:            syncManager,
		conditionManager:       conditionManager,
		portMappingRegenerator: nil, // Will be set later via SetPortMappingRegenerator
		ruleS2SRegenerator:     nil, // Will be set later via SetRuleS2SRegenerator
		syncTracker:            utils.NewSyncTracker(1 * time.Second),
		retryConfig:            utils.DefaultRetryConfig(),
	}
}

// SetPortMappingRegenerator sets the port mapping regenerator (used to avoid circular dependencies)
func (s *ServiceResourceService) SetPortMappingRegenerator(regenerator AddressGroupPortMappingRegenerator) {
	s.portMappingRegenerator = regenerator
}

// SetRuleS2SRegenerator sets the RuleS2S regenerator (used to avoid circular dependencies)
func (s *ServiceResourceService) SetRuleS2SRegenerator(regenerator RuleS2SRegenerator) {
	s.ruleS2SRegenerator = regenerator
}

// GetServices returns all services within scope
func (s *ServiceResourceService) GetServices(ctx context.Context, scope ports.Scope) ([]models.Service, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	services := make([]models.Service, 0)
	err = reader.ListServices(ctx, func(service models.Service) error {
		services = append(services, service)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list services")
	}
	return services, nil
}

// GetServiceByID returns service by ID
func (s *ServiceResourceService) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return reader.GetServiceByID(ctx, id)
}

// GetServicesByIDs returns multiple services by IDs
func (s *ServiceResourceService) GetServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.Service, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var services []models.Service
	for _, id := range ids {
		service, err := reader.GetServiceByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Skip not found services
			}
			return nil, errors.Wrapf(err, "failed to get service %s", id.Key())
		}
		services = append(services, *service)
	}
	return services, nil
}

// CreateService creates a new service
func (s *ServiceResourceService) CreateService(ctx context.Context, service models.Service) error {

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Validate service for creation
	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	if err := serviceValidator.ValidateForCreation(ctx, service); err != nil {
		return err
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.syncServices(ctx, writer, []models.Service{service}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to create service")
	}

	readerFromWriter, err := s.registry.ReaderFromWriter(ctx, writer)
	if err != nil {
		return errors.Wrap(err, "failed to get reader from writer for service re-read")
	}
	defer readerFromWriter.Close()

	createdService, err := readerFromWriter.GetServiceByID(ctx, service.ResourceIdentifier)
	if err != nil {
		return errors.Wrapf(err, "failed to re-read service %s after creation", service.Key())
	}

	if err = s.syncServiceWithExternal(ctx, createdService, types.SyncOperationUpsert, false); err != nil {
		// Sync failed - abort transaction and return error
		return fmt.Errorf("SGROUP sync failed, transaction aborted: %w", err)
	}

	// Commit only after successful SGROUP sync
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit (use createdService with correct AggregatedAddressGroups)
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessServiceConditions(ctx, createdService); err != nil {
			klog.Errorf("Failed to process service conditions for %s/%s: %v",
				createdService.Namespace, createdService.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	// Sync port mappings for AddressGroups in spec (use createdService)
	if err := s.syncPortMappingsForServiceSpecAGs(ctx, createdService); err != nil {
		return errors.Wrap(err, "failed to sync port mappings after service creation")
	}

	return nil
}

// UpdateService updates an existing service
func (s *ServiceResourceService) UpdateService(ctx context.Context, service models.Service) error {

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Get existing service for validation
	existingService, err := reader.GetServiceByID(ctx, service.ResourceIdentifier)
	if err != nil {
		return errors.Wrap(err, "failed to get existing service")
	}

	// Validate service for update
	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	if err := serviceValidator.ValidateForUpdate(ctx, *existingService, service); err != nil {
		return err
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Check if service ports changed - this affects rule generation
	portsChanged := s.servicePortsChanged(*existingService, service)

	// Sync service (this will update it)
	if err = s.syncServices(ctx, writer, []models.Service{service}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to update service")
	}

	readerFromWriter, err := s.registry.ReaderFromWriter(ctx, writer)
	if err != nil {
		return errors.Wrap(err, "failed to get reader from writer for service re-read")
	}
	defer readerFromWriter.Close()

	updatedService, err := readerFromWriter.GetServiceByID(ctx, service.ResourceIdentifier)
	if err != nil {
		return errors.Wrapf(err, "failed to re-read service %s after update", service.Key())
	}

	if err = s.syncServiceWithExternal(ctx, updatedService, types.SyncOperationUpsert, false); err != nil {
		// Sync failed - abort transaction and return error
		return fmt.Errorf("SGROUP sync failed, transaction aborted: %w", err)
	}

	// Commit only after successful SGROUP sync
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit (use updatedService with correct AggregatedAddressGroups)
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessServiceConditions(ctx, updatedService); err != nil {
			klog.Errorf("Failed to process service conditions for %s/%s: %v",
				updatedService.Namespace, updatedService.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	// If ports changed, regenerate AddressGroupPortMappings that reference this service
	if portsChanged {

		if s.portMappingRegenerator != nil {
			serviceID := models.ResourceIdentifier{Name: updatedService.Name, Namespace: updatedService.Namespace}
			if err := s.portMappingRegenerator.RegeneratePortMappingsForService(ctx, serviceID); err != nil {
				klog.Errorf("Failed to regenerate AddressGroupPortMappings for service %s: %v",
					updatedService.Key(), err)
				// Don't fail the operation if port mapping regeneration fails
				// The service update succeeded, and mappings can be manually regenerated
			} else {
			}
		} else {
			klog.Warningf("⚠️ UpdateService: Service %s ports changed but no port mapping regenerator available", updatedService.Key())
		}

		if s.ruleS2SRegenerator != nil {
			serviceID := models.ResourceIdentifier{Name: updatedService.Name, Namespace: updatedService.Namespace}
			if err := s.ruleS2SRegenerator.RegenerateIEAgAgRulesForService(ctx, serviceID); err != nil {
				klog.Errorf("Failed to regenerate IEAgAg rules for service %s: %v",
					updatedService.Key(), err)
				// Don't fail the operation if IEAgAg rule regeneration fails
				// The service update succeeded, and rules can be manually regenerated
			} else {
			}
		} else {
			klog.Warningf("⚠️ UpdateService: Service %s ports changed but no RuleS2S regenerator available", updatedService.Key())
		}
	}

	// Check if AddressGroups or ports changed
	addressGroupsChanged := !reflect.DeepEqual(existingService.AddressGroups, service.AddressGroups)
	portsChanged = !reflect.DeepEqual(existingService.IngressPorts, service.IngressPorts)

	if addressGroupsChanged || portsChanged {
		// Sync port mappings for current AddressGroups (use updatedService)
		if err := s.syncPortMappingsForServiceSpecAGs(ctx, updatedService); err != nil {
			return errors.Wrap(err, "failed to sync port mappings after service update")
		}

		// If AddressGroups changed, also regenerate for removed AGs
		if addressGroupsChanged {
			// Find removed AGs
			oldAGKeys := make(map[string]bool)
			for _, ag := range existingService.AddressGroups {
				oldAGKeys[fmt.Sprintf("%s/%s", ag.Namespace, ag.Name)] = true
			}

			newAGKeys := make(map[string]bool)
			for _, ag := range service.AddressGroups {
				newAGKeys[fmt.Sprintf("%s/%s", ag.Namespace, ag.Name)] = true
			}

			// Regenerate for removed AGs
			if s.portMappingRegenerator != nil {
				for _, ag := range existingService.AddressGroups {
					key := fmt.Sprintf("%s/%s", ag.Namespace, ag.Name)
					if !newAGKeys[key] {
						agID := models.NewResourceIdentifier(ag.Name, models.WithNamespace(ag.Namespace))
						if err := s.portMappingRegenerator.RegeneratePortMappingsForAddressGroup(ctx, agID); err != nil {
							return errors.Wrapf(err, "failed to regenerate port mappings for removed address group %s", agID.Key())
						}
					}
				}
			}
		}
	}

	return nil
}

// SyncServices synchronizes multiple services
func (s *ServiceResourceService) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, syncOp models.SyncOp) error {

	// Before syncing, check for port changes to trigger port mapping regeneration
	var servicesWithPortChanges []models.ResourceIdentifier
	var removedAddressGroups []models.ResourceIdentifier // Track removed AGs for cleanup

	if s.portMappingRegenerator != nil {
		reader, readerErr := s.registry.Reader(ctx)
		if readerErr == nil {
			defer reader.Close()

			// Check each service for port changes OR new services with spec.addressGroups
			for _, newService := range services {
				serviceID := models.ResourceIdentifier{
					Name:      newService.Name,
					Namespace: newService.Namespace,
				}

				existingService, getErr := reader.GetServiceByID(ctx, serviceID)
				if getErr == nil && existingService != nil {
					// Service exists, check for port changes OR addressGroups changes
					portsChanged := s.servicePortsChanged(*existingService, newService)
					addressGroupsChanged := !reflect.DeepEqual(existingService.AddressGroups, newService.AddressGroups)

					if portsChanged || addressGroupsChanged {
						servicesWithPortChanges = append(servicesWithPortChanges, serviceID)

						// If AddressGroups changed, collect removed AGs for cleanup
						if addressGroupsChanged {
							newAGKeys := make(map[string]bool)
							for _, ag := range newService.AddressGroups {
								newAGKeys[fmt.Sprintf("%s/%s", ag.Namespace, ag.Name)] = true
							}

							for _, ag := range existingService.AddressGroups {
								key := fmt.Sprintf("%s/%s", ag.Namespace, ag.Name)
								if !newAGKeys[key] {
									agID := models.NewResourceIdentifier(ag.Name, models.WithNamespace(ag.Namespace))
									removedAddressGroups = append(removedAddressGroups, agID)
								}
							}
						}
					}
				} else {
					// New service - check if it has spec.addressGroups that need port mapping creation
					if len(newService.AddressGroups) > 0 {
						servicesWithPortChanges = append(servicesWithPortChanges, serviceID)
					}
				}
			}
		} else {
			klog.Warningf("SyncServices: Failed to get reader for port change detection: %v", readerErr)
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.syncServices(ctx, writer, services, syncOp); err != nil {
		return errors.Wrap(err, "failed to sync services")
	}

	if syncOp != models.SyncOpDelete {
		reader, readerErr := s.registry.Reader(ctx)
		if readerErr != nil {
			writer.Abort()
			return errors.Wrap(readerErr, "failed to get reader for pre-commit validation")
		}
		defer reader.Close()

		validator := validation.NewDependencyValidator(reader)
		serviceValidator := validator.GetServiceValidator()

		for _, service := range services {
			serviceID := service.ResourceIdentifier

			// Check if service exists to determine validation type
			existingService, getErr := reader.GetServiceByID(ctx, serviceID)

			if getErr == nil && existingService != nil {
				// Service exists - this is an UPDATE operation
				// Use ValidateForUpdate to check port conflicts with proper context
				if err := serviceValidator.ValidateForUpdate(ctx, *existingService, service); err != nil {
					writer.Abort()
					return errors.Wrapf(err, "pre-commit validation failed for service %s", service.Key())
				}
			} else {
				// Service does not exist - this is a CREATE operation
				// Use ValidateWithoutDuplicateCheck (creation validation without entity existence check)
				if err := serviceValidator.ValidateWithoutDuplicateCheck(ctx, service); err != nil {
					writer.Abort()
					return errors.Wrapf(err, "pre-commit validation failed for service %s", service.Key())
				}
			}
		}
	}

	readerFromWriter, readerErr := s.registry.ReaderFromWriter(ctx, writer)
	if readerErr != nil {
		writer.Abort()
		return errors.Wrap(readerErr, "failed to get reader from writer for services re-read")
	}
	defer readerFromWriter.Close()

	var updatedServices []models.Service
	for _, service := range services {
		updatedService, getErr := readerFromWriter.GetServiceByID(ctx, service.ResourceIdentifier)
		if getErr != nil {
			if errors.Is(getErr, ports.ErrNotFound) && syncOp == models.SyncOpDelete {
				updatedServices = append(updatedServices, service)
				continue
			}
			writer.Abort()
			return errors.Wrapf(getErr, "failed to re-read service %s after sync", service.Key())
		}
		updatedServices = append(updatedServices, *updatedService)
	}

	if s.syncManager != nil {
		for i := range updatedServices {
			var operation types.SyncOperation
			if syncOp == models.SyncOpDelete {
				operation = types.SyncOperationDelete
			} else {
				operation = types.SyncOperationUpsert
			}

			// Sync without retry (transactional sync)
			if err = s.syncServiceWithExternal(ctx, &updatedServices[i], operation, false); err != nil {
				// Sync failed - abort transaction and return error
				return fmt.Errorf("SGROUP sync failed for service %s, transaction aborted: %w", updatedServices[i].Key(), err)
			}
		}
	}

	// Commit only after all successful SGROUP syncs
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Always regenerate dependent resources for services with dependencies, but skip condition processing for DELETE
	if syncOp != models.SyncOpDelete {
		// Process conditions after successful commit (use updatedServices with correct AggregatedAddressGroups)
		if s.conditionManager != nil {
			for i := range updatedServices {
				if err := s.conditionManager.ProcessServiceConditions(ctx, &updatedServices[i]); err != nil {
					klog.Errorf("Failed to process service conditions for %s/%s: %v",
						updatedServices[i].Namespace, updatedServices[i].Name, err)
					// Don't fail the operation if condition processing fails
				}
			}
		}
	}

	if s.portMappingRegenerator != nil && len(servicesWithPortChanges) > 0 {

		for _, serviceID := range servicesWithPortChanges {
			if err := s.portMappingRegenerator.RegeneratePortMappingsForService(ctx, serviceID); err != nil {
				klog.Errorf("SyncServices: Failed to regenerate AddressGroupPortMappings for service %s: %v",
					serviceID.Key(), err)
				// Don't fail the operation if port mapping regeneration fails
			} else {
			}
		}
	}

	// Regenerate port mappings for removed AddressGroups to clean up stale service references
	if s.portMappingRegenerator != nil && len(removedAddressGroups) > 0 {

		for _, agID := range removedAddressGroups {
			if err := s.portMappingRegenerator.RegeneratePortMappingsForAddressGroup(ctx, agID); err != nil {
				klog.Errorf("SyncServices: Failed to regenerate port mapping for removed AddressGroup %s: %v",
					agID.Key(), err)
				// Don't fail the operation if cleanup fails
			} else {
			}
		}
	}

	if s.ruleS2SRegenerator != nil && len(servicesWithPortChanges) > 0 {

		for _, serviceID := range servicesWithPortChanges {
			if err := s.ruleS2SRegenerator.RegenerateIEAgAgRulesForService(ctx, serviceID); err != nil {
				klog.Errorf("SyncServices: Failed to regenerate IEAgAg rules for service %s: %v",
					serviceID.Key(), err)
				// Don't fail the operation if IEAgAg rule regeneration fails
			} else {
			}
		}
	}

	// For DELETE operations, trigger condition re-processing for dependent resources to detect broken references
	if syncOp == models.SyncOpDelete {

		for _, service := range services {
			serviceID := models.ResourceIdentifier{Name: service.Name, Namespace: service.Namespace}
			if err := s.reprocessDependentResourceConditions(ctx, serviceID); err != nil {
				klog.Errorf("SyncServices: Failed to reprocess dependent resource conditions for service %s: %v",
					serviceID.Key(), err)
				// Don't fail the operation if condition reprocessing fails
			} else {
			}
		}
	}

	return nil
}

// DeleteServicesByIDs deletes services by IDs with dependency validation
func (s *ServiceResourceService) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	// 1. Get reader for validation
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for validation")
	}
	defer reader.Close()

	// 2. Validate dependencies for each service
	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	for _, id := range ids {
		if err := serviceValidator.CheckDependencies(ctx, id); err != nil {
			return errors.Wrapf(err, "cannot delete Service %s", id.Key())
		}
	}

	// 3. For each service to be deleted, get it for external sync and regenerate port mappings
	var servicesToDelete []models.Service
	for _, id := range ids {
		service, err := reader.GetServiceByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Service doesn't exist, skip
			}
			return errors.Wrapf(err, "failed to get service %s before deletion", id.Key())
		}

		servicesToDelete = append(servicesToDelete, *service)

		// Regenerate port mappings for all AddressGroups to remove this service
		if err := s.syncPortMappingsForServiceSpecAGs(ctx, service); err != nil {
			return errors.Wrapf(err, "failed to sync port mappings before deleting service %s", id.Key())
		}
	}

	// 4. Proceed with deletion
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteServicesByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete services")
	}

	if s.syncManager != nil {
		for _, service := range servicesToDelete {
			if err = s.syncServiceWithExternal(ctx, &service, types.SyncOperationDelete, false); err != nil {
				// Sync failed - abort transaction and return error
				return fmt.Errorf("SGROUP deletion sync failed for service %s, transaction aborted: %w", service.Key(), err)
			}
		}
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}

// ServiceAlias methods

// GetServiceAliases returns all service aliases within scope
func (s *ServiceResourceService) GetServiceAliases(ctx context.Context, scope ports.Scope) ([]models.ServiceAlias, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	aliases := make([]models.ServiceAlias, 0)
	err = reader.ListServiceAliases(ctx, func(alias models.ServiceAlias) error {
		aliases = append(aliases, alias)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list service aliases")
	}
	return aliases, nil
}

// GetServiceAliasByID returns service alias by ID
func (s *ServiceResourceService) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return reader.GetServiceAliasByID(ctx, id)
}

// GetServiceAliasesByIDs returns multiple service aliases by IDs
func (s *ServiceResourceService) GetServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.ServiceAlias, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var aliases []models.ServiceAlias
	for _, id := range ids {
		alias, err := reader.GetServiceAliasByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Skip not found aliases
			}
			return nil, errors.Wrapf(err, "failed to get service alias %s", id.Key())
		}
		aliases = append(aliases, *alias)
	}
	return aliases, nil
}

// CreateServiceAlias creates a new service alias
func (s *ServiceResourceService) CreateServiceAlias(ctx context.Context, alias models.ServiceAlias) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.syncServiceAliases(ctx, writer, []models.ServiceAlias{alias}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to create service alias")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessServiceAliasConditions(ctx, &alias); err != nil {
			klog.Errorf("Failed to process service alias conditions for %s/%s: %v",
				alias.Namespace, alias.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	return nil
}

// UpdateServiceAlias updates an existing service alias
func (s *ServiceResourceService) UpdateServiceAlias(ctx context.Context, alias models.ServiceAlias) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.syncServiceAliases(ctx, writer, []models.ServiceAlias{alias}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to update service alias")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessServiceAliasConditions(ctx, &alias); err != nil {
			klog.Errorf("Failed to process service alias conditions for %s/%s: %v",
				alias.Namespace, alias.Name, err)
			// Don't fail the operation if condition processing fails
		}
	}

	// Regenerate IEAgAg rules that depend on this ServiceAlias

	if s.ruleS2SRegenerator != nil {
		serviceAliasID := models.ResourceIdentifier{Name: alias.Name, Namespace: alias.Namespace}
		if err := s.ruleS2SRegenerator.RegenerateIEAgAgRulesForServiceAlias(ctx, serviceAliasID); err != nil {
			klog.Errorf("Failed to regenerate IEAgAg rules for ServiceAlias %s: %v",
				alias.Key(), err)
			// Don't fail the operation if IEAgAg rule regeneration fails
		} else {
		}
	} else {
		klog.Warningf("⚠️ UpdateServiceAlias: ServiceAlias %s updated but no RuleS2S regenerator available", alias.Key())
	}

	return nil
}

// SyncServiceAliases synchronizes multiple service aliases
func (s *ServiceResourceService) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope, syncOp models.SyncOp) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.syncServiceAliases(ctx, writer, aliases, syncOp); err != nil {
		return errors.Wrap(err, "failed to sync service aliases")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Always regenerate IEAgAg rules for service aliases, but skip condition processing for DELETE
	if syncOp != models.SyncOpDelete {
		// Process conditions after successful commit for each service alias (only for non-DELETE operations)
		klog.Infof("🔄 SyncServiceAliases: Processing conditions for %d service aliases, conditionManager=%v", len(aliases), s.conditionManager != nil)
		if s.conditionManager != nil {
			for i := range aliases {
				klog.Infof("🔄 SyncServiceAliases: Processing conditions for service alias %s/%s", aliases[i].Namespace, aliases[i].Name)
				if err := s.conditionManager.ProcessServiceAliasConditions(ctx, &aliases[i]); err != nil {
					klog.Errorf("Failed to process service alias conditions for %s/%s: %v",
						aliases[i].Namespace, aliases[i].Name, err)
					// Don't fail the operation if condition processing fails
				}
			}
		} else {
			klog.Warningf("⚠️ SyncServiceAliases: conditionManager is nil, skipping condition processing for %d service aliases", len(aliases))
		}
	} else {
	}

	if syncOp != models.SyncOpDelete {
		// Regenerate IEAgAg rules for service aliases (only for non-DELETE operations)
		if s.ruleS2SRegenerator != nil && len(aliases) > 0 {

			for i := range aliases {
				serviceAliasID := models.ResourceIdentifier{Name: aliases[i].Name, Namespace: aliases[i].Namespace}
				if err := s.ruleS2SRegenerator.RegenerateIEAgAgRulesForServiceAlias(ctx, serviceAliasID); err != nil {
					klog.Errorf("SyncServiceAliases: Failed to regenerate IEAgAg rules for ServiceAlias %s: %v",
						aliases[i].Key(), err)
					// Don't fail the operation if IEAgAg rule regeneration fails
				} else {
				}
			}
		}
	} else {
	}

	return nil
}

// DeleteServiceAliasesByIDs deletes service aliases by IDs with dependency validation
func (s *ServiceResourceService) DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	// First validate that all ServiceAliases can be safely deleted
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for dependency validation")
	}
	defer reader.Close()

	// Validate dependencies for each ServiceAlias before deletion
	validator := validation.NewDependencyValidator(reader)
	aliasValidator := validator.GetServiceAliasValidator()

	for _, id := range ids {

		if err := aliasValidator.CheckDependencies(ctx, id); err != nil {
			return errors.Wrapf(err, "cannot delete ServiceAlias %s", id.Key())
		}
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteServiceAliasesByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete service aliases")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}

// Private helper methods (extracted from original NetguardService)

// syncServices handles the actual synchronization logic
func (s *ServiceResourceService) syncServices(ctx context.Context, writer ports.Writer, services []models.Service, syncOp models.SyncOp) error {
	if err := writer.SyncServices(ctx, services, ports.EmptyScope{}, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync services in storage")
	}

	return nil
}

// syncServiceAliases handles the actual service alias synchronization logic
func (s *ServiceResourceService) syncServiceAliases(ctx context.Context, writer ports.Writer, aliases []models.ServiceAlias, syncOp models.SyncOp) error {

	// Use passed syncOp to handle service aliases operations correctly
	if err := writer.SyncServiceAliases(ctx, aliases, ports.EmptyScope{}, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync service aliases in storage")
	}

	return nil
}

// isTransientError determines if an error is transient (network/timeout) and should be retried
// Returns false for validation/business logic errors that should fail immediately
func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's a gRPC error with a specific code
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		// Transient errors - retry these
		case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted:
			return true
		// Permanent errors - don't retry these
		case codes.InvalidArgument, codes.AlreadyExists, codes.FailedPrecondition,
			codes.NotFound, codes.PermissionDenied, codes.Unauthenticated:
			return false
		}
	}

	// Fallback: check error message for common transient error patterns
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "connection") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "unavailable") ||
		strings.Contains(errMsg, "deadline")
}

// syncServiceWithExternal syncs a Service with external systems
// If allowRetry is false, only attempts sync once (used in transactional operations)
// If allowRetry is true, retries only on transient errors (network/timeout issues)
func (s *ServiceResourceService) syncServiceWithExternal(ctx context.Context, service *models.Service, operation types.SyncOperation, allowRetry bool) error {
	syncKey := fmt.Sprintf("%s-%s", operation, service.Key())

	// Check debouncing only if retry is allowed (post-commit operations)
	if allowRetry && !s.syncTracker.ShouldSync(syncKey) {
		return nil // Skip sync due to debouncing
	}

	// Execute sync - with or without retry based on allowRetry flag
	var err error
	if allowRetry {
		// Post-commit sync: use retry with transient error detection
		err = utils.ExecuteWithRetry(ctx, s.retryConfig, func() error {
			// Sync Service with SGROUP
			if s.syncManager != nil {
				if syncErr := s.syncManager.SyncEntity(ctx, service, operation); syncErr != nil {
					// ExecuteWithRetry will check IsRetryableError automatically
					// If error is not retryable (validation, etc.), it will fail immediately
					return syncErr
				}
			}
			return nil
		})
	} else {
		// Pre-commit sync: single attempt, no retry
		if s.syncManager != nil {
			err = s.syncManager.SyncEntity(ctx, service, operation)
		}
	}

	if err != nil {
		if allowRetry {
			s.syncTracker.RecordFailure(syncKey, err)
		}
		// Set sync failed condition directly on Meta
		service.Meta.SetCondition(metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "SyncFailed",
			Message:            fmt.Sprintf("Failed to sync with external system: %v", err),
			LastTransitionTime: metav1.Now(),
		})
		return fmt.Errorf("failed to sync with external system: %w", err)
	}

	if allowRetry {
		s.syncTracker.RecordSuccess(syncKey)
	}
	// Set sync success condition directly on Meta
	service.Meta.SetCondition(metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "SyncSucceeded",
		Message:            "Successfully synced with external system",
		LastTransitionTime: metav1.Now(),
	})
	return nil
}

// servicePortsChanged checks if service ports have changed between old and new versions
func (s *ServiceResourceService) servicePortsChanged(oldService, newService models.Service) bool {
	if len(oldService.IngressPorts) != len(newService.IngressPorts) {
		return true
	}

	// Convert to maps for easier comparison
	oldPorts := make(map[string]models.IngressPort)
	for _, port := range oldService.IngressPorts {
		key := string(port.Protocol) + ":" + port.Port
		oldPorts[key] = port
	}

	newPorts := make(map[string]models.IngressPort)
	for _, port := range newService.IngressPorts {
		key := string(port.Protocol) + ":" + port.Port
		newPorts[key] = port
	}

	// Check if any port is different
	for key, oldPort := range oldPorts {
		newPort, exists := newPorts[key]
		if !exists || oldPort != newPort {
			return true
		}
	}

	return false
}

// syncPortMappingsForServiceSpecAGs regenerates port mappings for all AddressGroups in Service.Spec
func (s *ServiceResourceService) syncPortMappingsForServiceSpecAGs(ctx context.Context, service *models.Service) error {
	if service == nil || len(service.AddressGroups) == 0 {
		return nil
	}

	if s.portMappingRegenerator == nil {
		return nil
	}

	for _, agRef := range service.AddressGroups {
		agID := models.NewResourceIdentifier(agRef.Name, models.WithNamespace(agRef.Namespace))

		// Regenerate port mapping for this AddressGroup
		// This will include all Services (both from spec and bindings)
		if err := s.portMappingRegenerator.RegeneratePortMappingsForAddressGroup(ctx, agID); err != nil {
			return errors.Wrapf(err, "failed to regenerate port mappings for address group %s", agID.Key())
		}
	}

	return nil
}

// FindServicesForAddressGroups finds all services that are bound to given address groups
func (s *ServiceResourceService) FindServicesForAddressGroups(ctx context.Context, addressGroupIDs []models.ResourceIdentifier) ([]models.Service, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var relatedServices []models.Service
	serviceIDs := make(map[string]models.ResourceIdentifier)

	// Find all address group bindings for these address groups
	for _, agID := range addressGroupIDs {
		var bindings []models.AddressGroupBinding
		err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
			if binding.AddressGroupRef.Name == agID.Name && binding.AddressGroupRef.Namespace == agID.Namespace {
				bindings = append(bindings, binding)
			}
			return nil
		}, ports.EmptyScope{})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to find bindings for address group %s", agID.Key())
		}

		// Collect unique service IDs
		for _, binding := range bindings {
			key := binding.ServiceRef.Namespace + "/" + binding.ServiceRef.Name
			serviceIDs[key] = models.ResourceIdentifier{
				Name:      binding.ServiceRef.Name,
				Namespace: binding.ServiceRef.Namespace,
			}
		}
	}

	// Fetch all related services
	for _, serviceID := range serviceIDs {
		service, err := reader.GetServiceByID(ctx, serviceID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Service might have been deleted
			}
			return nil, errors.Wrapf(err, "failed to get service %s", serviceID.Key())
		}
		relatedServices = append(relatedServices, *service)
	}

	return relatedServices, nil
}

// reprocessDependentResourceConditions finds and re-processes conditions for resources that depend on the deleted service
// This will update their status to reflect broken references
func (s *ServiceResourceService) reprocessDependentResourceConditions(ctx context.Context, deletedServiceID models.ResourceIdentifier) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for dependent resource processing")
	}
	defer reader.Close()

	// Find ServiceAliases that reference this service
	var dependentServiceAliases []models.ServiceAlias
	err = reader.ListServiceAliases(ctx, func(alias models.ServiceAlias) error {
		if alias.ServiceRef.Name == deletedServiceID.Name && alias.ServiceRef.Namespace == deletedServiceID.Namespace {
			dependentServiceAliases = append(dependentServiceAliases, alias)
		}
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		return errors.Wrap(err, "failed to find dependent ServiceAliases")
	}

	// Find AddressGroupBindings that reference this service
	var dependentBindings []models.AddressGroupBinding
	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		if binding.ServiceRef.Name == deletedServiceID.Name && binding.ServiceRef.Namespace == deletedServiceID.Namespace {
			dependentBindings = append(dependentBindings, binding)
		}
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		return errors.Wrap(err, "failed to find dependent AddressGroupBindings")
	}

	// Re-process conditions for ServiceAliases - this will detect broken references
	if s.conditionManager != nil {
		for i := range dependentServiceAliases {

			if err := s.conditionManager.ProcessServiceAliasConditions(ctx, &dependentServiceAliases[i]); err != nil {
				klog.Errorf("Failed to reprocess ServiceAlias conditions for %s/%s: %v",
					dependentServiceAliases[i].Namespace, dependentServiceAliases[i].Name, err)
				// Continue with other resources even if one fails
			} else {
			}
		}

		// Re-process conditions for AddressGroupBindings - this will detect broken references
		for i := range dependentBindings {

			if err := s.conditionManager.ProcessAddressGroupBindingConditions(ctx, &dependentBindings[i]); err != nil {
				klog.Errorf("Failed to reprocess AddressGroupBinding conditions for %s/%s: %v",
					dependentBindings[i].Namespace, dependentBindings[i].Name, err)
				// Continue with other resources even if one fails
			} else {
			}
		}
	} else {
		klog.Warningf("⚠️ reprocessDependentResourceConditions: conditionManager is nil, cannot reprocess conditions")
	}

	return nil
}
