package resources

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// aggregationMutexes contains mutexes for synchronizing aggregation operations
// This prevents race conditions when multiple RuleS2S operations affect the same aggregation groups
var aggregationMutexes = sync.Map{}

// AggregationKey uniquely identifies an aggregated rule for synchronization
type AggregationKey struct {
	Traffic      string
	LocalAGName  string
	TargetAGName string
	Protocol     string
}

// getAggregationMutex returns a mutex for a specific aggregation key
func getAggregationMutex(key AggregationKey) *sync.Mutex {
	mutexKey := fmt.Sprintf("%s-%s-%s-%s",
		key.Traffic, key.LocalAGName, key.TargetAGName, key.Protocol)

	mutex, _ := aggregationMutexes.LoadOrStore(mutexKey, &sync.Mutex{})
	return mutex.(*sync.Mutex)
}

// ContributingRule represents a RuleS2S that contributes to an IEAgAgRule aggregation
// This is the core structure for cross-RuleS2S port aggregation logic
type ContributingRule struct {
	RuleS2S *models.RuleS2S // The RuleS2S that contributes ports
	Ports   []string        // The ports this rule contributes to the aggregated IEAgAg rule
}

// RuleS2SResourceService handles RuleS2S and IEAgAgRule operations with complex rule generation logic
type RuleS2SResourceService struct {
	registry         ports.Registry
	syncManager      interfaces.SyncManager
	conditionManager ConditionManager // Interface for condition management
}

// ConditionManager interface for handling resource conditions
type ConditionManager interface {
	ProcessRuleS2SConditions(ctx context.Context, rule *models.RuleS2S) error
	ProcessIEAgAgRuleConditions(ctx context.Context, rule *models.IEAgAgRule) error
}

// NewRuleS2SResourceService creates a new RuleS2SResourceService
func NewRuleS2SResourceService(registry ports.Registry, syncManager interfaces.SyncManager, conditionManager ConditionManager) *RuleS2SResourceService {
	return &RuleS2SResourceService{
		registry:         registry,
		syncManager:      syncManager,
		conditionManager: conditionManager,
	}
}

// =============================================================================
// RuleS2S Operations
// =============================================================================

// GetRuleS2S returns all RuleS2S within scope
func (s *RuleS2SResourceService) GetRuleS2S(ctx context.Context, scope ports.Scope) ([]models.RuleS2S, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var rules []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		rules = append(rules, rule)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list RuleS2S")
	}
	return rules, nil
}

// GetRuleS2SByID returns RuleS2S by ID
func (s *RuleS2SResourceService) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return reader.GetRuleS2SByID(ctx, id)
}

// GetRuleS2SByIDs returns multiple RuleS2S by IDs
func (s *RuleS2SResourceService) GetRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.RuleS2S, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var rules []models.RuleS2S
	for _, id := range ids {
		rule, err := reader.GetRuleS2SByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Skip not found rules
			}
			return nil, errors.Wrapf(err, "failed to get RuleS2S %s", id.Key())
		}
		rules = append(rules, *rule)
	}
	return rules, nil
}

// CreateRuleS2S creates a new RuleS2S with IEAgAgRule generation
func (s *RuleS2SResourceService) CreateRuleS2S(ctx context.Context, rule models.RuleS2S) error {

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	if err := ruleValidator.ValidateForCreation(ctx, rule); err != nil {
		log.Printf("CreateRuleS2S: Validation failed for RuleS2S %s: %v", rule.Key(), err)
		return err
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			log.Printf("CreateRuleS2S: Aborting transaction for RuleS2S %s due to error: %v", rule.Key(), err)
			writer.Abort()
		}
	}()

	if err = s.syncRuleS2S(ctx, writer, []models.RuleS2S{rule}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to create rule s2s")
	}

	if err = writer.Commit(); err != nil {
		log.Printf("CreateRuleS2S: Failed to commit transaction for RuleS2S %s: %v", rule.Key(), err)
		return errors.Wrap(err, "failed to commit")
	}

	if err := s.triggerPostCreationIEAgAgRuleGeneration(ctx, rule); err != nil {
		log.Printf("⚠️ CreateRuleS2S: Failed to trigger post-creation IEAgAgRule regeneration for %s: %v", rule.Key(), err)
	}

	// Process conditions
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessRuleS2SConditions(ctx, &rule); err != nil {
			return errors.Wrap(err, "failed to process rule s2s conditions")
		}
	}

	return nil
}

// UpdateRuleS2S updates an existing RuleS2S
func (s *RuleS2SResourceService) UpdateRuleS2S(ctx context.Context, rule models.RuleS2S) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	existingRule, err := reader.GetRuleS2SByID(ctx, rule.ResourceIdentifier)
	if err != nil {
		return errors.Wrap(err, "failed to get existing RuleS2S")
	}

	// Preserve existing conditions in the update rule to prevent overwrite
	if len(existingRule.Meta.Conditions) > 0 {
		rule.Meta.Conditions = existingRule.Meta.Conditions
		rule.Meta.Generation = existingRule.Meta.Generation // Preserve generation too
	} else {
		log.Printf("ℹ️ UPDATE_CONDITIONS: No existing conditions to preserve for %s", rule.Key())
	}

	// Validate rule for update
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	if err := ruleValidator.ValidateForUpdate(ctx, *existingRule, rule); err != nil {
		log.Printf("UpdateRuleS2S: Validation failed for RuleS2S %s: %v", rule.Key(), err)
		return err
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			log.Printf("UpdateRuleS2S: Aborting transaction for RuleS2S %s due to error: %v", rule.Key(), err)
			writer.Abort()
		}
	}()

	if err = s.syncRuleS2S(ctx, writer, []models.RuleS2S{rule}, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to update rule s2s")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessRuleS2SConditions(ctx, &rule); err != nil {
			return errors.Wrap(err, "failed to process rule s2s conditions")
		}
	} else {
		log.Printf("⚠️ UPDATE_CONDITIONS: conditionManager is NIL for RuleS2S %s - conditions will NOT be processed!", rule.Key())
	}

	return nil
}

// SyncRuleS2S synchronizes multiple RuleS2S
func (s *RuleS2SResourceService) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope, syncOp models.SyncOp) error {
	if syncOp == models.SyncOpDelete {
		var idsToDelete []models.ResourceIdentifier
		for _, rule := range rules {
			idsToDelete = append(idsToDelete, rule.ResourceIdentifier)
		}

		// Use our enhanced deletion method with targeted cleanup
		return s.DeleteRuleS2SByIDs(ctx, idsToDelete)
	}

	// For non-DELETE operations, use the original sync logic
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.syncRuleS2S(ctx, writer, rules, syncOp); err != nil {
		return errors.Wrap(err, "failed to sync RuleS2S")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	if syncOp != models.SyncOpDelete {
		klog.Infof("🔄 SyncRuleS2S: Triggering post-sync IEAgAgRule regeneration check for %d rules", len(rules))
		for _, rule := range rules {
			if err := s.triggerPostCreationIEAgAgRuleGeneration(ctx, rule); err != nil {
				klog.Errorf("⚠️ SyncRuleS2S: Failed to trigger post-sync IEAgAgRule regeneration for %s: %v", rule.Key(), err)
				// Don't fail the entire sync, but log the issue
			}
		}
	}

	// Process conditions after successful commit for each RuleS2S
	klog.Infof("🔄 SyncRuleS2S: Processing conditions for %d RuleS2S, conditionManager=%v", len(rules), s.conditionManager != nil)
	successCount := 0
	failureCount := 0

	if s.conditionManager != nil {
		for i := range rules {
			if err := s.conditionManager.ProcessRuleS2SConditions(ctx, &rules[i]); err != nil {
				failureCount++
				// Don't fail the operation if condition processing fails, but track it
			} else {
				successCount++
			}
		}
	}

	return nil
}

// DeleteRuleS2SByIDs deletes RuleS2S by IDs and triggers targeted IEAgAg rule cleanup
func (s *RuleS2SResourceService) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for validation")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	ruleS2SValidator := validator.GetRuleS2SValidator()

	for _, id := range ids {
		if err := ruleS2SValidator.CheckDependencies(ctx, id); err != nil {
			return errors.Wrapf(err, "cannot delete RuleS2S %s", id.Key())
		}
	}

	var referencedIEAgAgRules []models.ResourceIdentifier
	for _, id := range ids {
		rule, err := reader.GetRuleS2SByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				klog.Infof("🔍 RULES2S_DELETE: RuleS2S %s already deleted, skipping", id.Key())
				continue
			}
			return errors.Wrapf(err, "failed to get RuleS2S %s for cleanup", id.Key())
		}

		// Collect all IEAgAgRule references from this RuleS2S
		if len(rule.IEAgAgRuleRefs) > 0 {
			// Primary method: Use saved references from RuleS2S
			for _, ieagagRef := range rule.IEAgAgRuleRefs {
				refID := models.ResourceIdentifier{
					Namespace: ieagagRef.Namespace,
					Name:      ieagagRef.Name,
				}
				referencedIEAgAgRules = append(referencedIEAgAgRules, refID)
			}
		} else {
			expectedRules, err := s.GenerateIEAgAgRulesFromRuleS2SWithReader(ctx, reader, *rule)
			if err != nil {
				continue
			}

			// Check which expected rules actually exist in database
			for _, expectedRule := range expectedRules {
				existingRule, err := reader.GetIEAgAgRuleByID(ctx, expectedRule.ResourceIdentifier)
				if err != nil {
					if errors.Is(err, ports.ErrNotFound) {
						klog.Infof("  📋 FALLBACK_CLEANUP: Expected IEAgAgRule %s not found, skipping", expectedRule.Key())
						continue
					}
					continue
				}

				if existingRule != nil {
					referencedIEAgAgRules = append(referencedIEAgAgRules, expectedRule.ResourceIdentifier)
				}
			}
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

	// Step 1: Delete the RuleS2S resources
	if err = writer.DeleteRuleS2SByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete RuleS2S")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	if len(referencedIEAgAgRules) > 0 {
		reason := fmt.Sprintf("rules2s-deletion-cleanup-%d-rules", len(ids))
		if err := s.RecalculateTargetedIEAgAgRules(ctx, referencedIEAgAgRules, reason); err != nil {
			klog.Errorf("⚠️ RULES2S_DELETE: Targeted cleanup failed after deletion: %v", err)
			// Don't fail the deletion for recalculation errors, just log them
		}
	}

	return nil
}

// =============================================================================
// IEAgAgRule Operations
// =============================================================================

// GetIEAgAgRules returns all IEAgAgRules within scope
func (s *RuleS2SResourceService) GetIEAgAgRules(ctx context.Context, scope ports.Scope) ([]models.IEAgAgRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var rules []models.IEAgAgRule
	err = reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
		rules = append(rules, rule)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list IEAgAgRules")
	}
	return rules, nil
}

// GetIEAgAgRuleByID returns IEAgAgRule by ID
func (s *RuleS2SResourceService) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return reader.GetIEAgAgRuleByID(ctx, id)
}

// GetIEAgAgRulesByIDs returns multiple IEAgAgRules by IDs
func (s *RuleS2SResourceService) GetIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.IEAgAgRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var rules []models.IEAgAgRule
	for _, id := range ids {
		rule, err := reader.GetIEAgAgRuleByID(ctx, id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				continue // Skip not found rules
			}
			return nil, errors.Wrapf(err, "failed to get IEAgAgRule %s", id.Key())
		}
		rules = append(rules, *rule)
	}
	return rules, nil
}

// SyncIEAgAgRules synchronizes multiple IEAgAgRules
func (s *RuleS2SResourceService) SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = s.syncIEAgAgRules(ctx, writer, rules, models.SyncOpUpsert); err != nil {
		return errors.Wrap(err, "failed to sync IEAgAgRules")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	// Process conditions after successful commit for each IEAgAgRule
	klog.Infof("🔄 SYNC_CONDITION_DEBUG: Processing conditions for %d IEAgAgRules, conditionManager nil? %v", len(rules), s.conditionManager == nil)
	if s.conditionManager != nil {
		for i := range rules {
			if err := s.conditionManager.ProcessIEAgAgRuleConditions(ctx, &rules[i]); err != nil {
				klog.Errorf("❌ SYNC_CONDITION_DEBUG: Failed to process IEAgAgRule conditions for %s/%s: %v",
					rules[i].Namespace, rules[i].Name, err)
				// Don't fail the operation if condition processing fails
			}
		}
	} else {
		klog.Warningf("⚠️ SYNC_CONDITION_DEBUG: conditionManager is NIL in SyncIEAgAgRules - no conditions will be processed for %d IEAgAgRules", len(rules))
	}

	return nil
}

// DeleteIEAgAgRulesByIDs deletes IEAgAgRules by IDs WITH external sync
func (s *RuleS2SResourceService) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for external sync preparation")
	}
	defer reader.Close()

	var rulesToDelete []models.IEAgAgRule
	for _, id := range ids {
		rule, err := reader.GetIEAgAgRuleByID(ctx, id)
		if err != nil {
			klog.Warningf("⚠️ IEAGAG_DELETE: Rule %s not found for deletion (may already be deleted): %v", id.Key(), err)
			continue // Continue with other rules
		}
		rulesToDelete = append(rulesToDelete, *rule)
	}

	var writer ports.Writer
	if registryWithDeletes, ok := s.registry.(interface {
		WriterForDeletes(context.Context) (ports.Writer, error)
	}); ok {
		writer, err = registryWithDeletes.WriterForDeletes(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get delete writer with ReadCommitted isolation")
		}
	} else {
		writer, err = s.registry.Writer(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get writer")
		}
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.DeleteIEAgAgRulesByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete IEAgAgRules from backend")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit backend deletion transaction")
	}

	if s.syncManager != nil {
		for _, rule := range rulesToDelete {
			if syncErr := s.syncManager.SyncEntity(ctx, &rule, types.SyncOperationDelete); syncErr != nil {
				klog.Errorf("⚠️ IEAGAG_DELETE: Failed to sync deletion of rule %s to external systems: %v", rule.SelfRef.Key(), syncErr)
				// Don't fail the entire deletion for external sync errors, but log them
			}
		}
	} else {
		klog.Warningf("⚠️ IEAGAG_DELETE: syncManager is nil - external sync SKIPPED for %d deleted rules", len(rulesToDelete))
	}

	return nil
}

// =============================================================================
// Complex Rule Generation Methods
// =============================================================================

// GenerateIEAgAgRulesFromRuleS2S generates IEAgAgRules from a RuleS2S
func (s *RuleS2SResourceService) GenerateIEAgAgRulesFromRuleS2S(ctx context.Context, ruleS2S models.RuleS2S) ([]models.IEAgAgRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return s.GenerateIEAgAgRulesFromRuleS2SWithReader(ctx, reader, ruleS2S)
}

// GenerateIEAgAgRulesFromRuleS2SWithReader generates IEAgAgRules using existing reader
func (s *RuleS2SResourceService) GenerateIEAgAgRulesFromRuleS2SWithReader(ctx context.Context, reader ports.Reader, ruleS2S models.RuleS2S) ([]models.IEAgAgRule, error) {
	log.Printf("🔨 GenerateIEAgAgRulesFromRuleS2S: Starting generation for RuleS2S %s", ruleS2S.Key())

	localServiceID := models.ResourceIdentifier{
		Name:      ruleS2S.ServiceLocalRef.Name,
		Namespace: ruleS2S.ServiceLocalRef.Namespace,
	}
	localService, err := reader.GetServiceByID(ctx, localServiceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get local service %s", ruleS2S.ServiceLocalRef.Name)
	}

	targetServiceID := models.ResourceIdentifier{
		Name:      ruleS2S.ServiceRef.Name,
		Namespace: ruleS2S.ServiceRef.Namespace,
	}
	targetService, err := reader.GetServiceByID(ctx, targetServiceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get target service %s", ruleS2S.ServiceRef.Name)
	}

	// Extract ports based on traffic direction
	var portsSource *models.Service
	if ruleS2S.Traffic == models.INGRESS {
		portsSource = localService
	} else {
		portsSource = targetService
	}

	allPorts := s.extractPortsFromService(*portsSource)
	var generatedRules []models.IEAgAgRule
	for _, localAG := range localService.GetAggregatedAddressGroups() {
		for _, targetAG := range targetService.GetAggregatedAddressGroups() {
			for _, protocol := range []models.TransportProtocol{models.TCP, models.UDP} {
				var protocolPorts []models.IngressPort
				for _, port := range allPorts {
					if port.Protocol == protocol {
						protocolPorts = append(protocolPorts, port)
					}
				}

				if len(protocolPorts) == 0 {
					log.Printf("    ⏭️ Skipping protocol %s (no ports)", protocol)
					continue
				}

				// Create aggregated IEAgAgRule with UUID-based name
				ieAgAgRule := models.IEAgAgRule{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      s.generateRuleNameForRuleS2S(ruleS2S, localAG.Ref, targetAG.Ref, protocol),
							Namespace: ruleS2S.Namespace,
						},
					},
					Traffic:           ruleS2S.Traffic,
					Transport:         protocol, // Set the transport protocol
					AddressGroupLocal: localAG.Ref,
					AddressGroup:      targetAG.Ref,
					Ports:             s.convertIngressPortsToPortSpecs(protocolPorts),
					Action:            models.ActionAccept, // Default action for generated rules
					Logs:              false,               // Logs disabled by default
					Trace:             ruleS2S.Trace,       // Preserve trace setting
					Priority:          100,                 // Default priority for generated rules
				}

				generatedRules = append(generatedRules, ieAgAgRule)
			}
		}
	}

	return generatedRules, nil
}

// =============================================================================
// Service/Rule Relationship Methods
// =============================================================================

// FindRuleS2SForServices finds all RuleS2S that reference given services
func (s *RuleS2SResourceService) FindRuleS2SForServices(ctx context.Context, serviceIDs []models.ResourceIdentifier) ([]models.RuleS2S, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return s.FindRuleS2SForServicesWithReader(ctx, reader, serviceIDs)
}

// FindRuleS2SForServicesWithReader finds RuleS2S using existing reader
func (s *RuleS2SResourceService) FindRuleS2SForServicesWithReader(ctx context.Context, reader ports.Reader, serviceIDs []models.ResourceIdentifier) ([]models.RuleS2S, error) {
	var relatedRules []models.RuleS2S

	for _, serviceID := range serviceIDs {
		rules, err := s.findAllRelatedRuleS2S(ctx, reader, serviceID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to find rules for service %s", serviceID.Key())
		}
		relatedRules = append(relatedRules, rules...)
	}

	// Remove duplicates
	uniqueRules := make(map[string]models.RuleS2S)
	for _, rule := range relatedRules {
		uniqueRules[rule.Key()] = rule
	}

	var result []models.RuleS2S
	for _, rule := range uniqueRules {
		result = append(result, rule)
	}

	return result, nil
}

// =============================================================================
// Complex Rule Update Methods
// =============================================================================

// UpdateIEAgAgRulesForRuleS2S updates IEAgAgRules for given RuleS2S
func (s *RuleS2SResourceService) UpdateIEAgAgRulesForRuleS2S(ctx context.Context, writer ports.Writer, rules []models.RuleS2S, syncOp models.SyncOp) error {
	reader, err := s.registry.ReaderFromWriter(ctx, writer)
	if err != nil {
		return errors.Wrap(err, "failed to get reader from writer")
	}
	defer reader.Close()

	return s.UpdateIEAgAgRulesForRuleS2SWithReaderAndExclusions(ctx, writer, reader, rules, syncOp, nil)
}

// UpdateIEAgAgRulesForRuleS2SWithReaderAndExclusions updates IEAgAgRules using existing reader with exclusions
func (s *RuleS2SResourceService) UpdateIEAgAgRulesForRuleS2SWithReaderAndExclusions(ctx context.Context, writer ports.Writer, reader ports.Reader, rules []models.RuleS2S, syncOp models.SyncOp, excludeRuleIDs []models.ResourceIdentifier) error {
	affectedServices := make(map[string]models.ResourceIdentifier)
	for _, rule := range rules {
		localServiceID := models.ResourceIdentifier{
			Name:      rule.ServiceLocalRef.Name,
			Namespace: rule.ServiceLocalRef.Namespace,
		}

		targetServiceID := models.ResourceIdentifier{
			Name:      rule.ServiceRef.Name,
			Namespace: rule.ServiceRef.Namespace,
		}

		if _, err := reader.GetServiceByID(ctx, localServiceID); err != nil {
			log.Printf("❌ UpdateIEAgAgRulesForRuleS2S: Failed to get local service %s: %v", localServiceID.Key(), err)
			continue
		}

		if _, err := reader.GetServiceByID(ctx, targetServiceID); err != nil {
			log.Printf("❌ UpdateIEAgAgRulesForRuleS2S: Failed to get target service %s: %v", targetServiceID.Key(), err)
			continue
		}

		affectedServices[localServiceID.Key()] = localServiceID
		affectedServices[targetServiceID.Key()] = targetServiceID
	}

	return s.UpdateIEAgAgRulesForAffectedServicesWithExclusions(ctx, writer, reader, affectedServices, syncOp, excludeRuleIDs)
}

// UpdateIEAgAgRulesForRuleS2SWithReader updates IEAgAgRules using existing reader
func (s *RuleS2SResourceService) UpdateIEAgAgRulesForRuleS2SWithReader(ctx context.Context, writer ports.Writer, reader ports.Reader, rules []models.RuleS2S, syncOp models.SyncOp) error {
	affectedServices := make(map[string]models.ResourceIdentifier)
	for _, rule := range rules {
		localServiceID := models.ResourceIdentifier{
			Name:      rule.ServiceLocalRef.Name,
			Namespace: rule.ServiceLocalRef.Namespace,
		}
		targetServiceID := models.ResourceIdentifier{
			Name:      rule.ServiceRef.Name,
			Namespace: rule.ServiceRef.Namespace,
		}

		if _, err := reader.GetServiceByID(ctx, localServiceID); err != nil {
			log.Printf("❌ UpdateIEAgAgRulesForRuleS2S: Failed to get local service %s: %v", localServiceID.Key(), err)
			continue
		}

		if _, err := reader.GetServiceByID(ctx, targetServiceID); err != nil {
			log.Printf("❌ UpdateIEAgAgRulesForRuleS2S: Failed to get target service %s: %v", targetServiceID.Key(), err)
			continue
		}

		affectedServices[localServiceID.Key()] = localServiceID
		affectedServices[targetServiceID.Key()] = targetServiceID
	}

	return s.UpdateIEAgAgRulesForAffectedServicesWithExclusions(ctx, writer, reader, affectedServices, syncOp, nil)
}

// UpdateIEAgAgRulesForAffectedServicesWithExclusions updates IEAgAgRules for services affected by changes with exclusions
func (s *RuleS2SResourceService) UpdateIEAgAgRulesForAffectedServicesWithExclusions(ctx context.Context, writer ports.Writer, reader ports.Reader, affectedServices map[string]models.ResourceIdentifier, syncOp models.SyncOp, excludeRuleIDs []models.ResourceIdentifier) error {
	var allAffectedRules []models.RuleS2S
	for _, serviceID := range affectedServices {
		rules, err := s.findAllRelatedRuleS2S(ctx, reader, serviceID)
		if err != nil {
			log.Printf("❌ UpdateIEAgAgRulesForAffectedServices: Failed to find rules for service %s: %v", serviceID.Key(), err)
			continue
		}
		allAffectedRules = append(allAffectedRules, rules...)
	}

	// Remove duplicates
	uniqueRules := make(map[string]models.RuleS2S)
	for _, rule := range allAffectedRules {
		uniqueRules[rule.Key()] = rule
	}

	var rulesToProcess []models.RuleS2S
	for _, rule := range uniqueRules {
		rulesToProcess = append(rulesToProcess, rule)
	}

	_, newIEAgAgRules, err := s.generateAggregatedIEAgAgRules(ctx, reader, rulesToProcess, excludeRuleIDs...)
	if err != nil {
		return errors.Wrap(err, "failed to generate aggregated IEAgAgRules")
	}

	if len(newIEAgAgRules) > 0 {
		if err := s.syncIEAgAgRulesWithReader(ctx, writer, reader, newIEAgAgRules, syncOp); err != nil {
			return errors.Wrap(err, "failed to sync generated IEAgAgRules")
		}
	}

	return nil
}

// UpdateIEAgAgRulesForAffectedServices updates IEAgAgRules for services affected by changes
func (s *RuleS2SResourceService) UpdateIEAgAgRulesForAffectedServices(ctx context.Context, writer ports.Writer, reader ports.Reader, affectedServices map[string]models.ResourceIdentifier, syncOp models.SyncOp) error {
	var allAffectedRules []models.RuleS2S

	for _, serviceID := range affectedServices {
		rules, err := s.findAllRelatedRuleS2S(ctx, reader, serviceID)
		if err != nil {
			log.Printf("❌ UpdateIEAgAgRulesForAffectedServices: Failed to find rules for service %s: %v", serviceID.Key(), err)
			continue
		}
		allAffectedRules = append(allAffectedRules, rules...)
	}

	// Remove duplicates
	uniqueRules := make(map[string]models.RuleS2S)
	for _, rule := range allAffectedRules {
		uniqueRules[rule.Key()] = rule
	}

	var rulesToProcess []models.RuleS2S
	for _, rule := range uniqueRules {
		rulesToProcess = append(rulesToProcess, rule)
	}

	_, newIEAgAgRules, err := s.generateAggregatedIEAgAgRules(ctx, reader, rulesToProcess)
	if err != nil {
		return errors.Wrap(err, "failed to generate aggregated IEAgAgRules")
	}

	// Update IEAgAgRules
	if len(newIEAgAgRules) > 0 {
		if err := s.syncIEAgAgRulesWithReader(ctx, writer, reader, newIEAgAgRules, syncOp); err != nil {
			return errors.Wrap(err, "failed to sync generated IEAgAgRules")
		}
	}

	return nil
}

// =============================================================================
// Private Helper Methods (extracted from original NetguardService)
// =============================================================================

// syncRuleS2S handles the actual RuleS2S synchronization logic with IEAgAgRule generation
func (s *RuleS2SResourceService) syncRuleS2S(ctx context.Context, writer ports.Writer, rules []models.RuleS2S, syncOp models.SyncOp) error {
	reader, err := s.registry.ReaderFromWriter(ctx, writer)
	if err != nil {
		return errors.Wrap(err, "failed to get reader from writer")
	}
	defer reader.Close()

	// Validation based on operation
	if syncOp != models.SyncOpDelete {
		validator := validation.NewDependencyValidator(reader)
		ruleValidator := validator.GetRuleS2SValidator()

		for _, rule := range rules {
			existingRule, err := reader.GetRuleS2SByID(ctx, rule.ResourceIdentifier)
			if err == nil {
				// Rule exists - use ValidateForUpdate
				if err := ruleValidator.ValidateForUpdate(ctx, *existingRule, rule); err != nil {
					return err
				}
			} else if errors.Is(err, ports.ErrNotFound) {
				// Rule is new - use ValidateForCreation
				if err := ruleValidator.ValidateForCreation(ctx, rule); err != nil {
					return err
				}
			} else if err != nil && !errors.Is(err, ports.ErrNotFound) {
				// Other error occurred
				return errors.Wrap(err, "failed to get RuleS2S")
			}
		}
	}

	// Sync RuleS2S first
	if err := writer.SyncRuleS2S(ctx, rules, ports.EmptyScope{}, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync RuleS2S in storage")
	}

	var excludeRuleIDs []models.ResourceIdentifier
	if syncOp == models.SyncOpDelete {
		for _, rule := range rules {
			excludeRuleIDs = append(excludeRuleIDs, rule.ResourceIdentifier)
		}
	}

	if err := s.UpdateIEAgAgRulesForRuleS2SWithReaderAndExclusions(ctx, writer, reader, rules, syncOp, excludeRuleIDs); err != nil {
		return errors.Wrap(err, "failed to update IEAgAgRules for RuleS2S")
	}

	return nil
}

// syncIEAgAgRules handles the actual IEAgAgRule synchronization logic
func (s *RuleS2SResourceService) syncIEAgAgRules(ctx context.Context, writer ports.Writer, rules []models.IEAgAgRule, syncOp models.SyncOp) error {
	return s.syncIEAgAgRulesWithReader(ctx, writer, nil, rules, syncOp)
}

// syncIEAgAgRulesWithReader handles IEAgAgRule synchronization with existing reader
func (s *RuleS2SResourceService) syncIEAgAgRulesWithReader(ctx context.Context, writer ports.Writer, reader ports.Reader, rules []models.IEAgAgRule, syncOp models.SyncOp) error {
	if reader == nil {
		var err error
		reader, err = s.registry.ReaderFromWriter(ctx, writer)
		if err != nil {
			return errors.Wrap(err, "failed to get reader from writer")
		}
		defer reader.Close()
	}

	// Sync IEAgAgRules with external systems using efficient batch sync
	if s.syncManager != nil && len(rules) > 0 {
		operation := types.SyncOperationUpsert
		if syncOp == models.SyncOpDelete {
			operation = types.SyncOperationDelete
		}

		// Convert rules to SyncableEntity slice for batch sync
		var syncableEntities []interfaces.SyncableEntity
		var unsyncableKeys []string

		for _, rule := range rules {
			// Create a copy to avoid pointer issues
			ruleCopy := rule
			if syncableEntity, ok := interface{}(&ruleCopy).(interfaces.SyncableEntity); ok {
				syncableEntities = append(syncableEntities, syncableEntity)
			} else {
				unsyncableKeys = append(unsyncableKeys, rule.Key())
			}
		}

		// Log any unsyncable rules
		if len(unsyncableKeys) > 0 {
			log.Printf("syncIEAgAgRulesWithReader: Skipping sync for %d non-syncable IEAgAgRules: %v", len(unsyncableKeys), unsyncableKeys)
		}

		// Perform batch sync for all syncable rules
		if len(syncableEntities) > 0 {
			log.Printf("syncIEAgAgRulesWithReader: Batch syncing %d IEAgAgRules to sgroups with operation %s", len(syncableEntities), operation)
			if err := s.syncManager.SyncBatch(ctx, syncableEntities, operation); err != nil {
				log.Printf("syncIEAgAgRulesWithReader: Warning - failed to batch sync %d IEAgAgRules to sgroups: %v", len(syncableEntities), err)
			}
		}
	}

	var rulesWithConditions, rulesWithoutConditions int
	for _, rule := range rules {
		if len(rule.Meta.Conditions) > 0 {
			rulesWithConditions++
		} else {
			rulesWithoutConditions++
		}
	}

	if err := s.applyTimingFixConditionsOnly(ctx, rules); err != nil {
		return errors.Wrap(err, "failed to apply conditions-only timing fix to IEAgAg rules in PATH_B")
	}

	if err := writer.SyncIEAgAgRules(ctx, rules, ports.EmptyScope{}, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync IEAgAgRules in storage")
	}

	if s.conditionManager != nil {
		for i := range rules {
			if err := s.conditionManager.ProcessIEAgAgRuleConditions(ctx, &rules[i]); err != nil {
				log.Printf("❌ POST_SYNC_CONDITION_FIX: Failed to process IEAgAgRule conditions for %s/%s: %v", rules[i].Namespace, rules[i].Name, err)
				// Don't fail the operation if condition processing fails
			}
		}
	} else {
		log.Printf("⚠️ POST_SYNC_CONDITION_FIX: conditionManager is NIL - no conditions will be processed for %d IEAgAgRules", len(rules))
	}

	return nil
}

// RuleS2SRegenerator interface implementation for reactive IEAgAg rule updates

// RegenerateIEAgAgRulesForService regenerates all IEAgAg rules that depend on a specific Service
func (s *RuleS2SResourceService) RegenerateIEAgAgRulesForService(ctx context.Context, serviceID models.ResourceIdentifier) error {

	// Get reader to find affected RuleS2S
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Get the service to understand current state
	changedService, err := reader.GetServiceByID(ctx, serviceID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			log.Printf("⚠️ RegenerateIEAgAgRulesForService: Service %s not found (deleted) - using universal recalculation", serviceID.Key())
			return s.RecalculateAllAffectedIEAgAgRules(ctx, fmt.Sprintf("service %s deleted", serviceID.Key()))
		}
		return errors.Wrap(err, "failed to get changed service")
	}

	affectedRulesMap := make(map[string]models.RuleS2S)

	directRuleCount := 0
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		if (rule.ServiceRef.Name == serviceID.Name && rule.ServiceRef.Namespace == serviceID.Namespace) ||
			(rule.ServiceLocalRef.Name == serviceID.Name && rule.ServiceLocalRef.Namespace == serviceID.Namespace) {
			if _, exists := affectedRulesMap[rule.Key()]; !exists {
				affectedRulesMap[rule.Key()] = rule
				directRuleCount++
			}
		}
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		return errors.Wrap(err, "failed to find rules referencing service")
	}

	// 2. Find rules via aggregation group relationships
	aggregationRuleCount := 0
	for _, ag := range changedService.GetAggregatedAddressGroups() {
		rules, err := s.findRuleS2SByAddressGroupInteraction(ctx, reader, ag.Ref)
		if err != nil {
			log.Printf("❌ Error finding rules for AddressGroup %s: %v", ag.Ref.Name, err)
			continue
		}
		for _, rule := range rules {
			if _, exists := affectedRulesMap[rule.Key()]; !exists {
				affectedRulesMap[rule.Key()] = rule
				aggregationRuleCount++
			}
		}
	}

	// Convert to slice
	var affectedRules []models.RuleS2S
	for _, rule := range affectedRulesMap {
		affectedRules = append(affectedRules, rule)
	}

	if len(affectedRules) == 0 {
		log.Printf("⚠️ No affected RuleS2S found for Service %s - no regeneration needed", serviceID.Key())
		return nil
	}

	reason := fmt.Sprintf("service %s changed (AddressGroups: %d)", serviceID.Key(), len(changedService.GetAggregatedAddressGroups()))
	return s.RecalculateIEAgAgRulesForAffectedRuleS2S(ctx, affectedRules, reason)
}

// RegenerateIEAgAgRulesForServiceAlias regenerates all IEAgAg rules that depend on a specific ServiceAlias
func (s *RuleS2SResourceService) RegenerateIEAgAgRulesForServiceAlias(ctx context.Context, serviceAliasID models.ResourceIdentifier) error {

	// Get reader
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Find all RuleS2S that directly reference this ServiceAlias
	var affectedRules []models.RuleS2S

	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		// Check if rule's ServiceLocalRef or ServiceRef reference the ServiceAlias
		if rule.ServiceLocalRef.Name == serviceAliasID.Name && rule.ServiceLocalRef.Namespace == serviceAliasID.Namespace {
			affectedRules = append(affectedRules, rule)
			log.Printf("RegenerateIEAgAgRulesForServiceAlias: Found affected RuleS2S %s (ServiceLocalRef)", rule.Key())
			return nil
		}
		if rule.ServiceRef.Name == serviceAliasID.Name && rule.ServiceRef.Namespace == serviceAliasID.Namespace {
			affectedRules = append(affectedRules, rule)
			log.Printf("RegenerateIEAgAgRulesForServiceAlias: Found affected RuleS2S %s (ServiceRef)", rule.Key())
			return nil
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to list RuleS2S")
	}

	if len(affectedRules) == 0 {
		log.Printf("RegenerateIEAgAgRulesForServiceAlias: No affected RuleS2S found for ServiceAlias %s", serviceAliasID.Key())
		return nil
	}

	return s.regenerateIEAgAgRulesForRuleS2SList(ctx, affectedRules)
}

// RegenerateIEAgAgRulesForAddressGroupBinding regenerates IEAgAg rules affected by AddressGroupBinding changes
func (s *RuleS2SResourceService) RegenerateIEAgAgRulesForAddressGroupBinding(ctx context.Context, bindingID models.ResourceIdentifier) error {
	log.Printf("RegenerateIEAgAgRulesForAddressGroupBinding: Starting regeneration for AddressGroupBinding %s", bindingID.Key())

	// Get reader
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Get the AddressGroupBinding to understand what Service it affects
	binding, err := reader.GetAddressGroupBindingByID(ctx, bindingID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			log.Printf("RegenerateIEAgAgRulesForAddressGroupBinding: AddressGroupBinding %s not found, may have been deleted", bindingID.Key())
			// If binding is deleted, we still need to regenerate affected rules
		} else {
			return errors.Wrap(err, "failed to get AddressGroupBinding")
		}
	}

	// Find all RuleS2S that might be affected by this AddressGroupBinding change
	// This is complex because we need to find rules that reference the service that this binding affects
	var affectedRules []models.RuleS2S

	if binding != nil {
		// If binding exists, find rules that reference the service this binding affects
		serviceRef := binding.ServiceRef
		serviceID := models.ResourceIdentifier{Name: serviceRef.Name, Namespace: serviceRef.Namespace}

		err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
			// Check if ServiceLocalRef or ServiceRef reference the service affected by this binding
			if rule.ServiceLocalRef.Name == serviceID.Name && rule.ServiceLocalRef.Namespace == serviceID.Namespace {
				affectedRules = append(affectedRules, rule)
				log.Printf("RegenerateIEAgAgRulesForAddressGroupBinding: Found affected RuleS2S %s via service %s (ServiceLocalRef)", rule.Key(), serviceID.Key())
				return nil
			}
			if rule.ServiceRef.Name == serviceID.Name && rule.ServiceRef.Namespace == serviceID.Namespace {
				affectedRules = append(affectedRules, rule)
				log.Printf("RegenerateIEAgAgRulesForAddressGroupBinding: Found affected RuleS2S %s via service %s (ServiceRef)", rule.Key(), serviceID.Key())
				return nil
			}
			return nil
		}, ports.EmptyScope{})

		if err != nil {
			return errors.Wrap(err, "failed to list RuleS2S")
		}
	} else {
		// If binding was deleted, we need to regenerate all rules to be safe
		// This is less efficient but ensures correctness
		log.Printf("RegenerateIEAgAgRulesForAddressGroupBinding: Binding was deleted, regenerating all rules")

		err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
			affectedRules = append(affectedRules, rule)
			return nil
		}, ports.EmptyScope{})

		if err != nil {
			return errors.Wrap(err, "failed to list all RuleS2S")
		}
	}

	if len(affectedRules) == 0 {
		log.Printf("RegenerateIEAgAgRulesForAddressGroupBinding: No affected RuleS2S found for AddressGroupBinding %s", bindingID.Key())
		return nil
	}

	log.Printf("RegenerateIEAgAgRulesForAddressGroupBinding: Found %d affected RuleS2S for AddressGroupBinding %s", len(affectedRules), bindingID.Key())

	// Regenerate IEAgAg rules for affected RuleS2S
	return s.regenerateIEAgAgRulesForRuleS2SList(ctx, affectedRules)
}

func (s *RuleS2SResourceService) NotifyServiceAddressGroupsChanged(ctx context.Context, serviceID models.ResourceIdentifier) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Log current service state for debugging
	service, serviceErr := reader.GetServiceByID(ctx, serviceID)
	if serviceErr == nil {
		agRefs := make([]string, len(service.GetAggregatedAddressGroups()))
		for i, agRef := range service.GetAggregatedAddressGroups() {
			agRefs[i] = fmt.Sprintf("%s/%s", agRef.Ref.Namespace, agRef.Ref.Name)
		}
		log.Printf("🔍 SERVICE_CURRENT_STATE: Service %s currently has %d AddressGroups: [%s]",
			service.Key(), len(service.GetAggregatedAddressGroups()), strings.Join(agRefs, ", "))
	} else {
		log.Printf("❌ SERVICE_LOOKUP_ERROR: Failed to get current state of Service %s: %v", serviceID.Key(), serviceErr)
	}

	var affectedRules []models.RuleS2S

	// Find all RuleS2S that reference this Service directly (ServiceAlias layer removed)
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		// Check direct Service references
		if rule.ServiceLocalRef.Name == serviceID.Name && rule.ServiceLocalRef.Namespace == serviceID.Namespace {
			affectedRules = append(affectedRules, rule)
			return nil
		}

		if rule.ServiceRef.Name == serviceID.Name && rule.ServiceRef.Namespace == serviceID.Namespace {
			affectedRules = append(affectedRules, rule)
			return nil
		}

		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to list RuleS2S")
	}

	if len(affectedRules) == 0 {
		log.Printf("🔔 NO_AFFECTED_RULES: No RuleS2S found that reference Service %s - no regeneration needed", serviceID.Key())
		return nil
	}

	existingRulesCount := 0
	reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
		existingRulesCount++
		return nil
	}, ports.EmptyScope{})

	err = s.RegenerateIEAgAgRulesForService(ctx, serviceID)
	if err != nil {
		return errors.Wrapf(err, "failed to regenerate IEAgAgRules for service %s", serviceID.Key())
	}

	newRulesCount := 0
	reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
		newRulesCount++
		return nil
	}, ports.EmptyScope{})

	for _, rule := range affectedRules {
		if s.conditionManager != nil {
			if err := s.conditionManager.ProcessRuleS2SConditions(ctx, &rule); err != nil {
				log.Printf("⚠️ CONDITION_ERROR: Failed to re-evaluate conditions for RuleS2S %s: %v", rule.Key(), err)
				// Continue processing other rules
			}
		} else {
			log.Printf("⚠️ NO_CONDITION_MANAGER: Cannot re-evaluate conditions for RuleS2S %s", rule.Key())
		}
	}

	return nil
}

// regenerateIEAgAgRulesForRuleS2SList is a helper method to regenerate IEAgAg rules for a list of RuleS2S
func (s *RuleS2SResourceService) regenerateIEAgAgRulesForRuleS2SList(ctx context.Context, ruleS2SList []models.RuleS2S) error {
	if len(ruleS2SList) == 0 {
		log.Printf("🔄 REGENERATION_EMPTY: No RuleS2S provided for regeneration - skipping")
		return nil
	}

	// Get writer for transaction management
	log.Printf("💾 TRANSACTION_START: Creating writer for IEAgAgRule regeneration")
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		log.Printf("❌ TRANSACTION_ERROR: Failed to get writer: %v", err)
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			log.Printf("❌ TRANSACTION_ABORT: Aborting transaction due to error: %v", err)
			writer.Abort()
		}
	}()

	reader, err := s.registry.ReaderFromWriter(ctx, writer)
	if err != nil {
		log.Printf("❌ READER_ERROR: Failed to get reader from writer: %v", err)
		return errors.Wrap(err, "failed to get reader from writer")
	}
	defer reader.Close()

	err = s.updateIEAgAgRulesForRuleS2SWithReader(ctx, writer, reader, ruleS2SList)
	if err != nil {
		log.Printf("❌ REGENERATION_ERROR: updateIEAgAgRulesForRuleS2SWithReader failed: %v", err)
		return err
	}

	return nil
}

// updateIEAgAgRulesForRuleS2SWithReader regenerates IEAgAg rules for given RuleS2S (similar to old service logic)
func (s *RuleS2SResourceService) updateIEAgAgRulesForRuleS2SWithReader(ctx context.Context, writer ports.Writer, reader ports.Reader, rules []models.RuleS2S) error {
	if len(rules) == 0 {
		return nil
	}

	// PHASE 0: Collect all aggregation keys that will be affected and acquire locks
	affectedKeys := make(map[string]AggregationKey)
	mutexes := make(map[string]*sync.Mutex)

	// First pass: identify all aggregation groups that will be affected
	for _, rule := range rules {
		groups, err := s.extractAggregationGroupsFromRuleS2S(ctx, reader, rule)
		if err != nil {
			log.Printf("❌ Error extracting aggregation groups from rule %s: %v", rule.Key(), err)
			continue
		}

		for _, group := range groups {
			// Create aggregation keys for both TCP and UDP protocols
			for _, protocol := range []string{"TCP", "UDP"} {
				key := AggregationKey{
					Traffic:      string(group.Traffic),
					LocalAGName:  group.LocalAG.Name,
					TargetAGName: group.TargetAG.Name,
					Protocol:     protocol,
				}
				keyStr := fmt.Sprintf("%s-%s-%s-%s", key.Traffic, key.LocalAGName, key.TargetAGName, key.Protocol)
				affectedKeys[keyStr] = key
				mutexes[keyStr] = getAggregationMutex(key)
			}
		}
	}

	// Acquire all locks in deterministic order to prevent deadlock
	keyStrs := make([]string, 0, len(affectedKeys))
	for keyStr := range affectedKeys {
		keyStrs = append(keyStrs, keyStr)
	}
	sort.Strings(keyStrs) // Sort for deterministic lock order

	for _, keyStr := range keyStrs {
		mutex := mutexes[keyStr]
		mutex.Lock()
	}

	// Ensure all locks are released
	defer func() {
		for _, keyStr := range keyStrs {
			mutex := mutexes[keyStr]
			mutex.Unlock()
			log.Printf("🔓 SYNC_AGGREGATION: Released lock for aggregation key: %s", keyStr)
		}
	}()

	// PHASE 1: Find all aggregation groups affected by the input rules (MUST do this first!)
	affectedGroups := make(map[string]AggregationGroup)
	for _, rule := range rules {
		groups, err := s.extractAggregationGroupsFromRuleS2S(ctx, reader, rule)
		if err != nil {
			log.Printf("❌ Error extracting aggregation groups from rule %s: %v", rule.Key(), err)
			continue
		}
		for _, group := range groups {
			affectedGroups[group.Key()] = group
		}
	}
	existingRules := make(map[string]models.IEAgAgRule)

	// Build targeted scope for only the aggregation groups we're regenerating
	affectedNamespaces := make(map[string]bool)
	affectedLocalAGs := make(map[string]bool)
	affectedTargetAGs := make(map[string]bool)

	for _, group := range affectedGroups {
		affectedNamespaces[group.LocalAG.Namespace] = true
		affectedNamespaces[group.TargetAG.Namespace] = true
		affectedLocalAGs[models.AddressGroupRefKey(group.LocalAG)] = true
		affectedTargetAGs[models.AddressGroupRefKey(group.TargetAG)] = true
	}

	// Helper function to get key from NamespacedObjectReference
	getAGKey := func(ref v1beta1.NamespacedObjectReference) string {
		return fmt.Sprintf("%s/%s", ref.Namespace, ref.Name)
	}

	// Get only rules that match our affected aggregation groups
	err := reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
		// Only include rules that are actually related to our affected aggregation groups
		localAGKey := getAGKey(rule.AddressGroupLocal)
		targetAGKey := getAGKey(rule.AddressGroup)

		if affectedLocalAGs[localAGKey] || affectedTargetAGs[targetAGKey] {
			existingRules[rule.Key()] = rule
			log.Printf("🔧 MASS_DELETION_FIX: Including existing rule %s (localAG:%s, targetAG:%s)",
				rule.Key(), localAGKey, targetAGKey)
		} else {
			log.Printf("🔧 MASS_DELETION_FIX: EXCLUDING rule %s (not related to affected groups)", rule.Key())
		}
		return nil
	}, ports.EmptyScope{}) // Still use EmptyScope for listing, but filter in the callback

	if err != nil {
		return errors.Wrap(err, "failed to list existing IEAgAg rules")
	}

	// PHASE 2: For each affected group, find ALL RuleS2S that contribute (proper aggregation per controller reference)
	// This is CORRECT behavior - aggregation requires finding all contributors, not just input rules
	var allContributingRules []models.RuleS2S
	ruleKeysProcessed := make(map[string]bool)

	for _, group := range affectedGroups {
		groupRules, err := s.FindAllRuleS2SForAggregationGroup(ctx, reader, group)
		if err != nil {
			log.Printf("❌ Error finding rules for aggregation group %s: %v", group.Key(), err)
			continue
		}

		for _, rule := range groupRules {
			if !ruleKeysProcessed[rule.Key()] {
				allContributingRules = append(allContributingRules, rule)
				ruleKeysProcessed[rule.Key()] = true
			}
		}
	}

	// PHASE 3: Use aggregated generation on the complete set for proper port aggregation
	expectedRulesSet, allNewRules, err := s.generateAggregatedIEAgAgRules(ctx, reader, allContributingRules)
	if err != nil {
		return errors.Wrap(err, "failed to generate aggregated IEAgAg rules")
	}

	// Sync new/updated IEAgAg rules
	if len(allNewRules) > 0 {
		if err = s.syncIEAgAgRulesWithReader(ctx, writer, reader, allNewRules, models.SyncOpUpsert); err != nil {
			return errors.Wrap(err, "failed to sync new IEAgAg rules")
		}
	}

	var obsoleteRules []models.IEAgAgRule
	for existingKey, existingRule := range existingRules {
		if !expectedRulesSet[existingKey] {
			// This rule is no longer needed within the affected aggregation groups
			obsoleteRules = append(obsoleteRules, existingRule)
			log.Printf("🔧 MASS_DELETION_FIX: Marking rule as obsolete: %s (was in affected groups but not in new expected set)", existingKey)
		}
	}

	if len(obsoleteRules) > 0 {
		totalSystemRuleCount := 0
		reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
			totalSystemRuleCount++
			return nil
		}, ports.EmptyScope{})

		deletionRatio := float64(len(obsoleteRules)) / float64(totalSystemRuleCount)
		if deletionRatio > 0.8 && totalSystemRuleCount > 10 {
			return errors.Errorf("safety check: refusing to delete %d IEAgAg rules (%.1f%% of total %d rules) - likely mass deletion bug",
				len(obsoleteRules), deletionRatio*100, totalSystemRuleCount)
		}

		// Extract IDs for deletion
		var obsoleteRuleIDs []models.ResourceIdentifier
		for _, rule := range obsoleteRules {
			obsoleteRuleIDs = append(obsoleteRuleIDs, rule.ResourceIdentifier)
		}

		if err = writer.DeleteIEAgAgRulesByIDs(ctx, obsoleteRuleIDs); err != nil {
			return errors.Wrap(err, "failed to delete obsolete IEAgAg rules")
		}
	}

	if err := s.applyTimingFixToIEAgAgRules(ctx, writer, allNewRules); err != nil {
		return errors.Wrap(err, "failed to apply universal timing fix to IEAgAg rules")
	}

	// Commit transaction (now includes rules WITH conditions)
	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit IEAgAg rule updates")
	}

	if s.conditionManager != nil {
		for i := range allContributingRules {
			if err := s.conditionManager.ProcessRuleS2SConditions(ctx, &allContributingRules[i]); err != nil {
			}
		}
	}

	return nil
}

func (s *RuleS2SResourceService) generateAggregatedIEAgAgRules(ctx context.Context, reader ports.Reader, rules []models.RuleS2S, excludeRuleIDs ...models.ResourceIdentifier) (map[string]bool, []models.IEAgAgRule, error) {
	excludeMap := make(map[string]bool)
	for _, id := range excludeRuleIDs {
		excludeMap[id.Key()] = true
	}

	expectedRules := make(map[string]bool)
	var newRules []models.IEAgAgRule
	processedCombinations := make(map[string]bool) // Track processed AG+Protocol combinations

	for _, currentRule := range rules {
		if excludeMap[currentRule.ResourceIdentifier.Key()] {
			continue
		}

		if !currentRule.Meta.IsReady() {
			continue
		}

		localService, targetService, err := s.getServicesForRuleWithReader(ctx, reader, &currentRule)
		if err != nil {
			continue
		}

		for _, localAG := range localService.GetAggregatedAddressGroups() {
			for _, targetAG := range targetService.GetAggregatedAddressGroups() {
				var portsSource *models.Service
				if currentRule.Traffic == models.INGRESS {
					portsSource = localService
				} else {
					portsSource = targetService
				}

				// Collect protocols that actually have ports
				protocolsWithPorts := make(map[models.TransportProtocol]bool)
				for _, port := range portsSource.IngressPorts {
					protocolsWithPorts[port.Protocol] = true
				}

				// Only process protocols that actually have ports
				for protocol := range protocolsWithPorts {
					// Create unique combination key
					combinationKey := fmt.Sprintf("%s|%s|%s|%s",
						currentRule.Traffic,
						s.addressGroupRefKey(localAG.Ref),
						s.addressGroupRefKey(targetAG.Ref),
						protocol)

					// Skip if we already processed this combination
					if processedCombinations[combinationKey] {
						continue
					}
					processedCombinations[combinationKey] = true

					contributingRules, err := s.findContributingRuleS2S(ctx, &currentRule, localService, targetService, excludeMap, protocol)
					if err != nil {
						continue
					}

					aggregatedPorts := s.aggregatePortsWithProtocol(ctx, reader, contributingRules, protocol)

					ruleS2SList := make([]models.RuleS2S, len(contributingRules))
					for i, cr := range contributingRules {
						ruleS2SList[i] = *cr.RuleS2S
					}
					aggregatedTrace := s.aggregateTraceValue(ruleS2SList)

					if len(aggregatedPorts) == 0 {
						ruleName := s.generateRuleName(string(currentRule.Traffic), localAG.Ref.Name, targetAG.Ref.Name, string(protocol))
						err := s.cleanupOrphanedIEAgAgRule(ctx, reader, ruleName, currentRule.Namespace, combinationKey)
						if err != nil {
							klog.Errorf("    ❌ CROSS_AGGREGATION: Failed to cleanup orphaned rule %s: %v", ruleName, err)
						}
						continue
					}

					ruleName := s.generateRuleName(string(currentRule.Traffic), localAG.Ref.Name, targetAG.Ref.Name, string(protocol))

					var ruleNamespace string
					if currentRule.Traffic == models.INGRESS {
						ruleNamespace = localAG.Ref.Namespace
					} else {
						ruleNamespace = targetAG.Ref.Namespace
					}

					ieRule := models.IEAgAgRule{
						SelfRef: models.SelfRef{
							ResourceIdentifier: models.ResourceIdentifier{
								Name:      ruleName,
								Namespace: ruleNamespace,
							},
						},
						Transport:         protocol,
						Traffic:           currentRule.Traffic,
						AddressGroupLocal: localAG.Ref,
						AddressGroup:      targetAG.Ref,
						Ports: []models.PortSpec{
							{
								Destination: strings.Join(aggregatedPorts, ","), // Single aggregated port string
							},
						},
						Action:   models.ActionAccept,
						Logs:     true,
						Trace:    aggregatedTrace,
						Priority: int32(100),
					}

					expectedRules[ieRule.Key()] = true
					newRules = append(newRules, ieRule)
				}
			}
		}
	}

	return expectedRules, newRules, nil
}

func (s *RuleS2SResourceService) cleanupOrphanedIEAgAgRule(ctx context.Context, reader ports.Reader, ruleName, namespace string, combinationKey string) error {
	ruleID := models.ResourceIdentifier{
		Name:      ruleName,
		Namespace: namespace,
	}

	existingRule, err := reader.GetIEAgAgRuleByID(ctx, ruleID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil // Rule doesn't exist, nothing to clean up
		}
		return errors.Wrapf(err, "failed to check if rule %s/%s exists", namespace, ruleName)
	}

	// Get a writer to delete the rule
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer for cleanup")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	err = writer.DeleteIEAgAgRulesByIDs(ctx, []models.ResourceIdentifier{ruleID})
	if err != nil {
		return errors.Wrapf(err, "failed to delete orphaned rule %s/%s", namespace, ruleName)
	}

	// Commit the deletion
	if err = writer.Commit(); err != nil {
		return errors.Wrapf(err, "failed to commit deletion of rule %s/%s", namespace, ruleName)
	}

	if s.syncManager != nil {
		err = s.syncManager.SyncEntity(ctx, existingRule, types.SyncOperationDelete)
		if err != nil {
		}
	}

	return nil
}

// =============================================================================
// Universal Recalculation Engine
// =============================================================================

// RecalculateAllAffectedIEAgAgRules provides universal recalculation/cleanup for ALL scenarios
// This implements the complete reference architecture pattern where ANY change that affects
// IEAgAg rules triggers the same comprehensive recalculation logic
func (s *RuleS2SResourceService) RecalculateAllAffectedIEAgAgRules(ctx context.Context, reason string) error {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for universal recalculation")
	}
	defer reader.Close()

	// Phase 1: Get ALL existing IEAgAg rules
	var existingRules []models.IEAgAgRule
	err = reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
		existingRules = append(existingRules, rule)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		return errors.Wrap(err, "failed to list existing IEAgAg rules")
	}

	var allRuleS2S []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		allRuleS2S = append(allRuleS2S, rule)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		return errors.Wrap(err, "failed to list all RuleS2S")
	}

	_, freshRules, err := s.generateAggregatedIEAgAgRules(ctx, reader, allRuleS2S)
	if err != nil {
		return errors.Wrap(err, "failed to generate fresh aggregated IEAgAg rules")
	}

	operations := s.calculateRuleOperations(existingRules, freshRules)

	if err := s.executeRuleOperations(ctx, operations, reason); err != nil {
		return errors.Wrapf(err, "failed to execute rule operations for reason: %s", reason)
	}

	return nil
}

// RecalculateIEAgAgRulesForAffectedRuleS2S provides efficient scoped recalculation for specific affected RuleS2S
// This method is optimized for scenarios where we know exactly which RuleS2S are affected (e.g., Service AddressGroup changes)
// It only processes IEAgAg rules belonging to the affected RuleS2S while maintaining cross-RuleS2S aggregation accuracy
func (s *RuleS2SResourceService) RecalculateIEAgAgRulesForAffectedRuleS2S(ctx context.Context, affectedRules []models.RuleS2S, reason string) error {
	if len(affectedRules) == 0 {
		klog.Infof("🔄 SCOPED_RECALC: No affected RuleS2S provided for recalculation - skipping (reason: %s)", reason)
		return nil
	}

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for scoped recalculation")
	}
	defer reader.Close()

	var existingRules []models.IEAgAgRule
	affectedServices := make(map[string]bool)

	for _, rule := range affectedRules {
		localService, targetService, err := s.getServicesForRuleWithReader(ctx, reader, &rule)
		if err != nil {
			klog.Errorf("  ⚠️ SCOPED_RECALC: Failed to get services for affected RuleS2S %s: %v", rule.Key(), err)
			continue
		}

		affectedServices[localService.Key()] = true
		affectedServices[targetService.Key()] = true
	}

	err = reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
		for serviceKey := range affectedServices {
			serviceNamespace := strings.Split(serviceKey, "/")[0]
			if rule.AddressGroupLocal.Namespace == serviceNamespace ||
				rule.AddressGroup.Namespace == serviceNamespace {
				existingRules = append(existingRules, rule)
				break
			}
		}
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		return errors.Wrap(err, "failed to list existing IEAgAg rules for affected services")
	}

	var allRuleS2S []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		allRuleS2S = append(allRuleS2S, rule)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		return errors.Wrap(err, "failed to list all RuleS2S for cross-aggregation")
	}

	_, freshRules, err := s.generateAggregatedIEAgAgRules(ctx, reader, allRuleS2S)
	if err != nil {
		return errors.Wrap(err, "failed to generate fresh aggregated IEAgAg rules for scoped recalculation")
	}

	operations := s.calculateRuleOperations(existingRules, freshRules)
	if err := s.executeRuleOperations(ctx, operations, reason); err != nil {
		return errors.Wrapf(err, "failed to execute scoped rule operations for reason: %s", reason)
	}

	return nil
}

// RecalculateTargetedIEAgAgRules provides targeted recalculation for specific IEAgAgRules
func (s *RuleS2SResourceService) RecalculateTargetedIEAgAgRules(ctx context.Context, targetedIEAgAgRuleIDs []models.ResourceIdentifier, reason string) error {
	if len(targetedIEAgAgRuleIDs) == 0 {
		klog.Infof("🎯 TARGETED_RECALC: No targeted IEAgAgRules provided for recalculation - skipping (reason: %s)", reason)
		return nil
	}

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for targeted recalculation")
	}
	defer reader.Close()

	var existingTargetedRules []models.IEAgAgRule
	for _, ruleID := range targetedIEAgAgRuleIDs {
		existingRule, err := reader.GetIEAgAgRuleByID(ctx, ruleID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				klog.Infof("  📋 TARGETED_RECALC: IEAgAgRule %s already deleted, skipping", ruleID.Key())
				continue
			}
			return errors.Wrapf(err, "failed to get existing IEAgAgRule %s", ruleID.Key())
		}
		existingTargetedRules = append(existingTargetedRules, *existingRule)
	}

	var allRemainingRuleS2S []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		allRemainingRuleS2S = append(allRemainingRuleS2S, rule)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		return errors.Wrap(err, "failed to list remaining RuleS2S")
	}

	_, allFreshRules, err := s.generateAggregatedIEAgAgRules(ctx, reader, allRemainingRuleS2S)
	if err != nil {
		return errors.Wrap(err, "failed to generate fresh aggregated IEAgAg rules for targeted recalculation")
	}

	freshTargetedRules := make([]models.IEAgAgRule, 0)
	targetedPatterns := make(map[string]bool)

	for _, existingRule := range existingTargetedRules {
		pattern := fmt.Sprintf("%s:%s:%s:%s",
			existingRule.Traffic,
			existingRule.AddressGroupLocal.Name,
			existingRule.AddressGroup.Name,
			existingRule.Transport)
		targetedPatterns[pattern] = true
	}

	for _, freshRule := range allFreshRules {
		pattern := fmt.Sprintf("%s:%s:%s:%s",
			freshRule.Traffic,
			freshRule.AddressGroupLocal.Name,
			freshRule.AddressGroup.Name,
			freshRule.Transport)
		if targetedPatterns[pattern] {
			freshTargetedRules = append(freshTargetedRules, freshRule)
		}
	}

	operations := s.calculateRuleOperations(existingTargetedRules, freshTargetedRules)
	if err := s.executeRuleOperations(ctx, operations, reason); err != nil {
		return errors.Wrapf(err, "failed to execute targeted rule operations for reason: %s", reason)
	}

	return nil
}

// RuleOperations represents the operations needed to sync existing rules with fresh calculations
type RuleOperations struct {
	toCreate []models.IEAgAgRule
	toUpdate []models.IEAgAgRule
	toDelete []models.IEAgAgRule
}

// calculateRuleOperations determines what operations are needed by comparing existing and fresh rules
func (s *RuleS2SResourceService) calculateRuleOperations(existing []models.IEAgAgRule, fresh []models.IEAgAgRule) *RuleOperations {
	operations := &RuleOperations{
		toCreate: []models.IEAgAgRule{},
		toUpdate: []models.IEAgAgRule{},
		toDelete: []models.IEAgAgRule{},
	}

	// Create lookup maps for efficient comparison
	existingMap := make(map[string]*models.IEAgAgRule)
	for i := range existing {
		key := existing[i].SelfRef.Key()
		existingMap[key] = &existing[i]
	}

	freshMap := make(map[string]*models.IEAgAgRule)
	for i := range fresh {
		key := fresh[i].SelfRef.Key()
		freshMap[key] = &fresh[i]
	}

	// Find rules to create or update
	for key, freshRule := range freshMap {
		if existingRule, exists := existingMap[key]; exists {
			if s.needsUpdate(existingRule, freshRule) {
				klog.Infof("    🔄 UNIVERSAL_RECALC: Rule %s needs UPDATE (ports changed)", key)
				operations.toUpdate = append(operations.toUpdate, *freshRule)
			}
		} else {
			operations.toCreate = append(operations.toCreate, *freshRule)
		}
	}

	for key, existingRule := range existingMap {
		if _, exists := freshMap[key]; !exists {
			operations.toDelete = append(operations.toDelete, *existingRule)
		}
	}

	return operations
}

// needsUpdate determines if an existing rule needs to be updated based on fresh calculations
func (s *RuleS2SResourceService) needsUpdate(existing *models.IEAgAgRule, fresh *models.IEAgAgRule) bool {
	// Compare ports (the main thing that changes in aggregation)
	if len(existing.Ports) != len(fresh.Ports) {
		return true
	}

	for i, existingPort := range existing.Ports {
		if i >= len(fresh.Ports) || existingPort.Destination != fresh.Ports[i].Destination {
			return true
		}
	}

	// Could add other field comparisons here if needed (action, transport, etc.)
	return false
}

// executeRuleOperations performs the calculated operations with proper external sync
func (s *RuleS2SResourceService) executeRuleOperations(ctx context.Context, operations *RuleOperations, reason string) error {
	if len(operations.toCreate) == 0 && len(operations.toUpdate) == 0 && len(operations.toDelete) == 0 {
		klog.Infof("  ✅ UNIVERSAL_RECALC: No operations needed (reason: %s)", reason)
		return nil
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer for operations")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Execute deletions first
	if len(operations.toDelete) > 0 {
		deleteIDs := make([]models.ResourceIdentifier, len(operations.toDelete))
		for i, rule := range operations.toDelete {
			deleteIDs[i] = rule.SelfRef.ResourceIdentifier
		}

		klog.Infof("  🗑️ UNIVERSAL_RECALC: Deleting %d orphaned rules", len(deleteIDs))
		if err := writer.DeleteIEAgAgRulesByIDs(ctx, deleteIDs); err != nil {
			return errors.Wrap(err, "failed to delete orphaned rules")
		}

		klog.Infof("  🔄 EXTERNAL_SYNC_DELETE: Syncing deletion of %d orphaned rules to external systems (reason: %s)", len(operations.toDelete), reason)
		for _, rule := range operations.toDelete {
			if s.syncManager != nil {
				if syncErr := s.syncManager.SyncEntity(ctx, &rule, types.SyncOperationDelete); syncErr != nil {
					klog.Errorf("  ⚠️ UNIVERSAL_RECALC: Failed to sync deletion of rule %s: %v", rule.SelfRef.Key(), syncErr)
					// Don't fail the entire operation for external sync errors, but log them
				} else {
					klog.Infof("  ✅ UNIVERSAL_RECALC: Successfully synced deletion of rule %s to external systems", rule.SelfRef.Key())
				}
			} else {
				klog.Warningf("  ⚠️ UNIVERSAL_RECALC: syncManager is nil - cannot sync deletion of rule %s", rule.SelfRef.Key())
			}
		}
	}

	// Execute creates and updates
	allChanges := append(operations.toCreate, operations.toUpdate...)
	if len(allChanges) > 0 {
		if s.conditionManager != nil {
			for i := range allChanges {
				if err := s.processIEAgAgRuleConditionsInMemory(ctx, &allChanges[i]); err != nil {
					klog.Errorf("  ❌ UNIVERSAL_RECALC_CONDITIONS: Failed to process IEAgAgRule conditions for %s/%s: %v", allChanges[i].Namespace, allChanges[i].Name, err)
					// Don't fail the operation if condition processing fails
				} else {
					klog.Infof("  ✅ UNIVERSAL_RECALC_CONDITIONS: Successfully processed conditions for %s", allChanges[i].Key())
				}
			}
		} else {
			klog.Warningf("  ⚠️ UNIVERSAL_RECALC_CONDITIONS: conditionManager is NIL - no conditions will be processed for %d IEAgAgRules", len(allChanges))
		}

		if err := writer.SyncIEAgAgRules(ctx, allChanges, ports.EmptyScope{}); err != nil {
			return errors.Wrap(err, "failed to sync rule changes")
		}

		// Sync creates/updates to external systems
		for _, rule := range allChanges {
			if s.syncManager != nil {
				if syncErr := s.syncManager.SyncEntity(ctx, &rule, types.SyncOperationUpsert); syncErr != nil {
					klog.Errorf("  ⚠️ UNIVERSAL_RECALC: Failed to sync rule %s: %v", rule.SelfRef.Key(), syncErr)
				}
			}
		}
	}

	// Commit all operations
	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit universal recalculation operations")
	}

	return nil
}

func (s *RuleS2SResourceService) findAllRelatedRuleS2S(ctx context.Context, reader ports.Reader, serviceID models.ResourceIdentifier) ([]models.RuleS2S, error) {
	var relatedRules []models.RuleS2S

	// Find service aliases that reference this service
	var serviceAliases []models.ServiceAlias
	err := reader.ListServiceAliases(ctx, func(alias models.ServiceAlias) error {
		if alias.ServiceRef.Name == serviceID.Name && alias.ServiceRef.Namespace == serviceID.Namespace {
			serviceAliases = append(serviceAliases, alias)
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return nil, errors.Wrap(err, "failed to find service aliases for service")
	}

	// Find all RuleS2S that reference any of these service aliases
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		for _, alias := range serviceAliases {
			if (rule.ServiceRef.Name == alias.Name && rule.ServiceRef.Namespace == alias.Namespace) ||
				(rule.ServiceLocalRef.Name == alias.Name && rule.ServiceLocalRef.Namespace == alias.Namespace) {
				relatedRules = append(relatedRules, rule)
				break
			}
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return nil, errors.Wrap(err, "failed to find RuleS2S for service aliases")
	}

	return relatedRules, nil
}

// triggerPostCreationIEAgAgRuleGeneration handles timing issues where AddressGroupBindings existed before RuleS2S creation
func (s *RuleS2SResourceService) triggerPostCreationIEAgAgRuleGeneration(ctx context.Context, rule models.RuleS2S) error {

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Get the services referenced directly by this RuleS2S to check their AddressGroup status
	localServiceID := models.ResourceIdentifier{
		Name:      rule.ServiceLocalRef.Name,
		Namespace: rule.ServiceLocalRef.Namespace,
	}
	targetServiceID := models.ResourceIdentifier{
		Name:      rule.ServiceRef.Name,
		Namespace: rule.ServiceRef.Namespace,
	}

	// Get the services directly (no ServiceAlias lookup needed since RuleS2S references Service directly)
	localService, err := reader.GetServiceByID(ctx, localServiceID)
	if err != nil {
		log.Printf("⚠️ triggerPostCreationIEAgAgRuleGeneration: Cannot get local service %s: %v", localServiceID.Key(), err)
		return nil
	}

	targetService, err := reader.GetServiceByID(ctx, targetServiceID)
	if err != nil {
		log.Printf("⚠️ triggerPostCreationIEAgAgRuleGeneration: Cannot get target service %s: %v", targetServiceID.Key(), err)
		return nil
	}

	log.Printf("🔍 triggerPostCreationIEAgAgRuleGeneration: Service AddressGroup counts - Local: %d, Target: %d",
		len(localService.GetAggregatedAddressGroups()), len(targetService.GetAggregatedAddressGroups()))

	// Check if both services have AddressGroups - if yes, we should have generated IEAgAgRules
	if len(localService.GetAggregatedAddressGroups()) > 0 && len(targetService.GetAggregatedAddressGroups()) > 0 {

		// Notify about both services to ensure complete aggregation
		if err := s.NotifyServiceAddressGroupsChanged(ctx, localServiceID); err != nil {
			log.Printf("⚠️ triggerPostCreationIEAgAgRuleGeneration: Failed to notify about local service %s: %v", localServiceID.Key(), err)
		}

		if err := s.NotifyServiceAddressGroupsChanged(ctx, targetServiceID); err != nil {
			log.Printf("⚠️ triggerPostCreationIEAgAgRuleGeneration: Failed to notify about target service %s: %v", targetServiceID.Key(), err)
		}

	} else {
		log.Printf("⚠️ triggerPostCreationIEAgAgRuleGeneration: Services don't have sufficient AddressGroups - Local: %d, Target: %d",
			len(localService.GetAggregatedAddressGroups()), len(targetService.GetAggregatedAddressGroups()))
	}

	return nil
}

func (s *RuleS2SResourceService) extractPortsFromService(service models.Service) []models.IngressPort {
	return service.IngressPorts
}

// findContributingRuleS2S finds all RuleS2S that contribute to the same IEAgAg rule aggregation
// Based on reference lines 603-654
func (s *RuleS2SResourceService) findContributingRuleS2S(
	ctx context.Context,
	currentRule *models.RuleS2S,
	localService *models.Service,
	targetService *models.Service,
	excludeMap map[string]bool,
	protocol models.TransportProtocol,
) ([]ContributingRule, error) {
	klog.Infof("🔍 CROSS_AGGREGATION: Finding contributing RuleS2S for current rule %s (local: %s, target: %s)",
		currentRule.Key(), localService.Key(), targetService.Key())

	// Get all RuleS2S for cross-rule comparison
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get registry reader")
	}

	var allRules []models.RuleS2S
	if err := reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		allRules = append(allRules, rule)
		return nil
	}, ports.EmptyScope{}); err != nil {
		return nil, errors.Wrap(err, "failed to list all RuleS2S")
	}

	var contributingRules []ContributingRule

	for _, rule := range allRules {
		if excludeMap[rule.ResourceIdentifier.Key()] {
			klog.Infof("  🚫 EXCLUSION_FILTER: Skipping excluded rule %s from contribution check", rule.Key())
			continue
		}

		if !rule.Meta.IsReady() {
			klog.Infof("  🚫 READY_FILTER: Skipping inactive (Ready=False) RuleS2S %s from contribution", rule.Key())
			continue
		}

		// No deletion checking needed in our backend implementation
		contributes, ports, err := s.checkIfRuleContributes(ctx, &rule, currentRule, localService, targetService, protocol)
		if err != nil {
			klog.Errorf("  ❌ CROSS_AGGREGATION: Error checking rule contribution for %s: %v", rule.Key(), err)
			continue
		}

		if contributes && len(ports) > 0 {
			klog.Infof("  ✅ CROSS_AGGREGATION: Found contributing rule %s (namespace: %s, ports: %s)",
				rule.Key(), rule.Namespace, strings.Join(ports, ","))

			contributingRules = append(contributingRules, ContributingRule{
				RuleS2S: &rule,
				Ports:   ports,
			})
		} else {
			klog.V(2).Infof("  ⏭️ CROSS_AGGREGATION: Rule %s does not contribute (contributes=%v, ports=%d)",
				rule.Key(), contributes, len(ports))
		}
	}

	klog.Infof("🎯 CROSS_AGGREGATION: Found %d contributing rules for current rule %s",
		len(contributingRules), currentRule.Key())

	return contributingRules, nil
}

// servicesHaveSameAddressGroups checks if two services have the same address groups
// Based on reference lines 753-775
func (s *RuleS2SResourceService) servicesHaveSameAddressGroups(
	service1 *models.Service,
	service2 *models.Service,
) bool {
	klog.V(2).Infof("🔍 COMPARE_SERVICES: Comparing AddressGroups between services %s and %s",
		service1.Key(), service2.Key())

	// First check lengths
	if len(service1.GetAggregatedAddressGroups()) != len(service2.GetAggregatedAddressGroups()) {
		klog.V(2).Infof("  ❌ COMPARE_SERVICES: Different AG counts - %s has %d, %s has %d",
			service1.Key(), len(service1.GetAggregatedAddressGroups()),
			service2.Key(), len(service2.GetAggregatedAddressGroups()))
		return false
	}

	// Create map of AddressGroup keys from service1
	agMap := make(map[string]bool)
	for _, ag := range service1.GetAggregatedAddressGroups() {
		key := s.addressGroupRefKey(ag.Ref)
		agMap[key] = true
		klog.V(2).Infof("  📍 COMPARE_SERVICES: Service1 AddressGroup: %s", key)
	}

	// Check if all AddressGroups from service2 exist in service1
	for _, ag := range service2.GetAggregatedAddressGroups() {
		key := s.addressGroupRefKey(ag.Ref)
		klog.V(2).Infof("  📍 COMPARE_SERVICES: Service2 AddressGroup: %s", key)
		if !agMap[key] {
			klog.V(2).Infof("  ❌ COMPARE_SERVICES: AddressGroup %s from %s not found in %s",
				key, service2.Key(), service1.Key())
			return false
		}
	}

	klog.V(2).Infof("  ✅ COMPARE_SERVICES: Services %s and %s have identical AddressGroups",
		service1.Key(), service2.Key())
	return true
}

// aggregatePortsWithProtocol aggregates ports from all contributing RuleS2S for a specific protocol
// Based on reference implementation lines 789-808: simple map-based deduplication without service re-fetching
func (s *RuleS2SResourceService) aggregatePortsWithProtocol(
	ctx context.Context,
	reader ports.Reader,
	contributingRules []ContributingRule,
	protocol models.TransportProtocol,
) []string {
	portSet := make(map[string]bool)
	for _, rule := range contributingRules {
		for _, port := range rule.Ports {
			portSet[port] = true
		}
	}

	// Convert set to sorted slice (same as reference)
	var aggregatedPorts []string
	for port := range portSet {
		aggregatedPorts = append(aggregatedPorts, port)
	}

	sort.Strings(aggregatedPorts)

	return aggregatedPorts
}

func (s *RuleS2SResourceService) checkIfRuleContributes(
	ctx context.Context,
	candidateRule *models.RuleS2S,
	currentRule *models.RuleS2S,
	localService *models.Service,
	targetService *models.Service,
	protocol models.TransportProtocol,
) (bool, []string, error) {
	if candidateRule.Traffic != currentRule.Traffic {
		klog.Infof("  ❌ CHECK_CONTRIBUTION: Traffic mismatch - candidate: %s, current: %s",
			candidateRule.Traffic, currentRule.Traffic)
		return false, nil, nil
	}

	// Get services for candidate rule to compare AddressGroups
	candidateLocalService, candidateTargetService, err := s.getServicesForRule(ctx, candidateRule)
	if err != nil {
		klog.Errorf("  ❌ CHECK_CONTRIBUTION: Failed to get services for candidate rule %s: %v",
			candidateRule.Key(), err)
		return false, nil, err
	}

	// Generate all AddressGroup combinations for current rule
	currentCombinations := s.generateAGCombinations(localService, targetService, currentRule.Traffic)
	candidateCombinations := s.generateAGCombinations(candidateLocalService, candidateTargetService, candidateRule.Traffic)

	hasOverlap := false
	for _, currentCombo := range currentCombinations {
		for _, candidateCombo := range candidateCombinations {
			if currentCombo == candidateCombo {
				hasOverlap = true
				break
			}
		}
		if hasOverlap {
			break
		}
	}

	if !hasOverlap {
		klog.Infof("  ❌ CHECK_CONTRIBUTION: No overlapping AG combinations for rule %s", candidateRule.Key())
		return false, nil, nil
	}

	// Extract ports based on traffic direction (same logic as reference)
	var ports []string
	if strings.ToLower(string(candidateRule.Traffic)) == "ingress" {
		ports = s.extractPortStringsFromService(*candidateLocalService, protocol)
	} else {
		ports = s.extractPortStringsFromService(*candidateTargetService, protocol)
	}

	return true, ports, nil
}

func (s *RuleS2SResourceService) getServicesForRule(
	ctx context.Context,
	rule *models.RuleS2S,
) (*models.Service, *models.Service, error) {
	reader, err := s.registry.ReaderWithReadCommitted(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get ReadCommitted registry reader")
	}
	defer reader.Close()
	return s.getServicesForRuleWithReader(ctx, reader, rule)
}

// getServicesForRuleWithReader gets services using the provided reader (same session consistency)
func (s *RuleS2SResourceService) getServicesForRuleWithReader(
	ctx context.Context,
	reader ports.Reader,
	rule *models.RuleS2S,
) (*models.Service, *models.Service, error) {
	localServiceID := models.ResourceIdentifier{
		Name:      rule.ServiceLocalRef.Name,
		Namespace: rule.ServiceLocalRef.Namespace,
	}
	localService, err := reader.GetServiceByID(ctx, localServiceID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "local service %s not found", localServiceID.Key())
	}

	targetServiceID := models.ResourceIdentifier{
		Name:      rule.ServiceRef.Name,
		Namespace: rule.ServiceRef.Namespace,
	}
	targetService, err := reader.GetServiceByID(ctx, targetServiceID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "target service %s not found", targetServiceID.Key())
	}

	localService, err = s.populateServiceAddressGroups(ctx, reader, localService)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to populate AddressGroups for local service %s", localServiceID.Key())
	}

	targetService, err = s.populateServiceAddressGroups(ctx, reader, targetService)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to populate AddressGroups for target service %s", targetServiceID.Key())
	}

	return localService, targetService, nil
}

func (s *RuleS2SResourceService) populateServiceAddressGroups(
	ctx context.Context,
	reader ports.Reader,
	service *models.Service,
) (*models.Service, error) {
	serviceCopy := *service
	//serviceCopy.XAggregatedAddressGroups = []models.AddressGroupReference{} // Reset aggregated slice
	//
	//err := reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
	//	if binding.ServiceRef.Name == service.Name && binding.ServiceRef.Namespace == service.Namespace {
	//		// Create NamespacedObjectReference for the new aggregated system
	//		agRef := v1beta1.NamespacedObjectReference{
	//			ObjectReference: v1beta1.ObjectReference{
	//				APIVersion: binding.AddressGroupRef.APIVersion,
	//				Kind:       binding.AddressGroupRef.Kind,
	//				Name:       binding.AddressGroupRef.Name,
	//			},
	//			Namespace: binding.AddressGroupRef.Namespace,
	//		}
	//
	//		// Create AddressGroupReference with binding source
	//		agReference := models.NewAddressGroupReference(agRef, v1beta1.AddressGroupSourceBinding)
	//
	//		// Add to service's XAggregatedAddressGroups
	//		serviceCopy.XAggregatedAddressGroups = append(serviceCopy.XAggregatedAddressGroups, agReference)
	//	}
	//	return nil
	//}, ports.EmptyScope{})
	//
	//if err != nil {
	//	return nil, errors.Wrapf(err, "failed to list AddressGroupBindings for service %s", service.Key())
	//}

	return &serviceCopy, nil
}

func (s *RuleS2SResourceService) extractPortStringsFromService(
	service models.Service,
	protocol models.TransportProtocol,
) []string {
	var ports []string
	for _, port := range service.IngressPorts {
		if port.Protocol == protocol {
			ports = append(ports, port.Port)
		}
	}

	return ports
}

func (s *RuleS2SResourceService) convertIngressPortsToPortSpecs(ports []models.IngressPort) []models.PortSpec {
	if len(ports) == 0 {
		return []models.PortSpec{}
	}

	// Aggregate ports into comma-separated string
	var portStrs []string
	for _, port := range ports {
		portStrs = append(portStrs, port.Port)
	}

	// Sort ports for deterministic output
	sort.Strings(portStrs)

	// Return single aggregated port spec
	return []models.PortSpec{{
		Destination: strings.Join(portStrs, ","),
		Source:      "", // Empty for destination ports
	}}
}

// generateRuleName creates a deterministic UUID-based rule name (restored from original implementation)
// This function MUST produce identical results to the original service.go.deprecated:5149-5168
func (s *RuleS2SResourceService) generateRuleName(trafficDirection, localAGName, targetAGName, protocol string) string {
	input := fmt.Sprintf("%s-%s-%s-%s",
		strings.ToLower(trafficDirection),
		localAGName,
		targetAGName,
		strings.ToLower(protocol))

	h := sha256.New()
	h.Write([]byte(input))
	hash := h.Sum(nil)

	// Format the first 16 bytes as UUID v5
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])

	// Use traffic direction prefix and UUID
	return fmt.Sprintf("%s-%s",
		strings.ToLower(trafficDirection)[:3],
		uuid)
}

// generateAggregatedRuleName generates UUID-based rule names for aggregated rules using the original logic
func (s *RuleS2SResourceService) generateAggregatedRuleName(traffic models.Traffic, localAG, targetAG models.AddressGroupRef, protocol models.TransportProtocol) string {
	return s.generateRuleName(string(traffic), localAG.Name, targetAG.Name, string(protocol))
}

// generateRuleNameForRuleS2S generates rule name for a specific RuleS2S (backward compatibility)
func (s *RuleS2SResourceService) generateRuleNameForRuleS2S(rule models.RuleS2S, localAG, targetAG models.AddressGroupRef, protocol models.TransportProtocol) string {
	return s.generateRuleName(string(rule.Traffic), localAG.Name, targetAG.Name, string(protocol))
}

func (s *RuleS2SResourceService) addressGroupRefKey(ref models.AddressGroupRef) string {
	return fmt.Sprintf("%s/%s", ref.Namespace, ref.Name)
}

// =============================================================================
// Smart Rule Discovery for Dynamic Port Aggregation
// =============================================================================

// AggregationGroup represents a group of rules that should be aggregated into a single IEAgAg rule
type AggregationGroup struct {
	Traffic   models.Traffic
	LocalAG   models.AddressGroupRef
	TargetAG  models.AddressGroupRef
	Protocol  models.TransportProtocol
	Namespace string
}

// Key returns a unique key for this aggregation group
func (ag AggregationGroup) Key() string {
	return fmt.Sprintf("%s|%s/%s|%s/%s|%s",
		ag.Traffic, ag.LocalAG.Namespace, ag.LocalAG.Name,
		ag.TargetAG.Namespace, ag.TargetAG.Name, ag.Protocol)
}

// FindAllRuleS2SForAggregationGroup finds all RuleS2S that contribute to the same aggregated IEAgAg rule
// This is critical for proper port aggregation when services change dynamically
func (s *RuleS2SResourceService) FindAllRuleS2SForAggregationGroup(ctx context.Context, reader ports.Reader, group AggregationGroup) ([]models.RuleS2S, error) {
	var matchingRules []models.RuleS2S

	// Search through all RuleS2S to find ones that would generate the same IEAgAg rule
	err := reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		// Skip rules in different namespaces
		if rule.Namespace != group.Namespace {
			return nil
		}

		// Skip rules with different traffic direction
		if rule.Traffic != group.Traffic {
			return nil
		}

		// Get services referenced by this rule to check AddressGroups
		if contributesToGroup, err := s.ruleContributesToAggregationGroup(ctx, reader, rule, group); err != nil {
			log.Printf("❌ Error checking if rule %s contributes to group %s: %v", rule.Key(), group.Key(), err)
			return nil // Continue with other rules
		} else if contributesToGroup {
			matchingRules = append(matchingRules, rule)
		}

		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return nil, errors.Wrap(err, "failed to search RuleS2S for aggregation group")
	}

	log.Printf("🔍 FindAllRuleS2SForAggregationGroup: Found %d rules for group %s", len(matchingRules), group.Key())
	return matchingRules, nil
}

// ruleContributesToAggregationGroup checks if a RuleS2S rule contributes to a specific aggregation group
func (s *RuleS2SResourceService) ruleContributesToAggregationGroup(ctx context.Context, reader ports.Reader, rule models.RuleS2S, group AggregationGroup) (bool, error) {
	// Get service IDs directly from RuleS2S references (no ServiceAlias lookup needed)
	localServiceID := models.ResourceIdentifier{
		Name:      rule.ServiceLocalRef.Name,
		Namespace: rule.ServiceLocalRef.Namespace,
	}
	targetServiceID := models.ResourceIdentifier{
		Name:      rule.ServiceRef.Name,
		Namespace: rule.ServiceRef.Namespace,
	}

	localService, err := reader.GetServiceByID(ctx, localServiceID)
	if err != nil {
		return false, nil // Skip if service not found
	}

	targetService, err := reader.GetServiceByID(ctx, targetServiceID)
	if err != nil {
		return false, nil // Skip if service not found
	}

	// Extract ports based on traffic direction
	var portsSource *models.Service
	if rule.Traffic == models.INGRESS {
		portsSource = localService
	} else {
		portsSource = targetService
	}

	// Check if this rule would generate IEAgAg rules for the same AG combinations and protocol
	for _, localAG := range localService.GetAggregatedAddressGroups() {
		for _, targetAG := range targetService.GetAggregatedAddressGroups() {
			// Check if this combination matches the aggregation group
			if s.addressGroupRefMatches(localAG.Ref, group.LocalAG) &&
				s.addressGroupRefMatches(targetAG.Ref, group.TargetAG) {

				// Check if the service has ports for the specified protocol
				hasProtocolPorts := false
				for _, port := range portsSource.IngressPorts {
					if port.Protocol == group.Protocol {
						hasProtocolPorts = true
						break
					}
				}

				if hasProtocolPorts {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// addressGroupRefMatches checks if two AddressGroupRef are equivalent
func (s *RuleS2SResourceService) addressGroupRefMatches(ref1, ref2 models.AddressGroupRef) bool {
	return ref1.Name == ref2.Name && ref1.Namespace == ref2.Namespace
}

// FindAggregationGroupsForServices finds all aggregation groups affected by service changes
// This is used when services change (ports, AddressGroups) to determine which IEAgAg rules need regeneration
func (s *RuleS2SResourceService) FindAggregationGroupsForServices(ctx context.Context, reader ports.Reader, serviceIDs []models.ResourceIdentifier) ([]AggregationGroup, error) {
	uniqueGroups := make(map[string]AggregationGroup)

	// Find all RuleS2S that reference the affected services
	affectedRules, err := s.FindRuleS2SForServicesWithReader(ctx, reader, serviceIDs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find RuleS2S for services")
	}

	// Extract aggregation groups from affected rules
	for _, rule := range affectedRules {
		groups, err := s.extractAggregationGroupsFromRuleS2S(ctx, reader, rule)
		if err != nil {
			log.Printf("❌ Error extracting aggregation groups from rule %s: %v", rule.Key(), err)
			continue
		}

		for _, group := range groups {
			uniqueGroups[group.Key()] = group
		}
	}

	var result []AggregationGroup
	for _, group := range uniqueGroups {
		result = append(result, group)
	}

	log.Printf("🔍 FindAggregationGroupsForServices: Found %d aggregation groups for %d services", len(result), len(serviceIDs))
	return result, nil
}

// extractAggregationGroupsFromRuleS2S extracts all possible aggregation groups from a single RuleS2S
func (s *RuleS2SResourceService) extractAggregationGroupsFromRuleS2S(ctx context.Context, reader ports.Reader, rule models.RuleS2S) ([]AggregationGroup, error) {
	// Get service IDs directly from RuleS2S references (no ServiceAlias lookup needed)
	localServiceID := models.ResourceIdentifier{
		Name:      rule.ServiceLocalRef.Name,
		Namespace: rule.ServiceLocalRef.Namespace,
	}
	targetServiceID := models.ResourceIdentifier{
		Name:      rule.ServiceRef.Name,
		Namespace: rule.ServiceRef.Namespace,
	}

	localService, err := reader.GetServiceByID(ctx, localServiceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get local service %s", localServiceID.Key())
	}

	targetService, err := reader.GetServiceByID(ctx, targetServiceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get target service %s", targetServiceID.Key())
	}

	// Extract ports based on traffic direction
	var portsSource *models.Service
	if rule.Traffic == models.INGRESS {
		portsSource = localService
	} else {
		portsSource = targetService
	}

	var groups []AggregationGroup

	// Generate aggregation groups for all AG combinations and protocols
	for _, localAG := range localService.GetAggregatedAddressGroups() {
		for _, targetAG := range targetService.GetAggregatedAddressGroups() {
			// Check what protocols this service supports
			protocolsSupported := make(map[models.TransportProtocol]bool)
			for _, port := range portsSource.IngressPorts {
				protocolsSupported[port.Protocol] = true
			}

			// Create aggregation groups for each protocol
			for protocol := range protocolsSupported {
				group := AggregationGroup{
					Traffic:   rule.Traffic,
					LocalAG:   localAG.Ref,
					TargetAG:  targetAG.Ref,
					Protocol:  protocol,
					Namespace: rule.Namespace,
				}
				groups = append(groups, group)
			}
		}
	}

	return groups, nil
}

// 🆕 Helper methods for aggregation-aware regeneration

// findServiceAliasesForService finds all ServiceAliases that reference a specific Service
func (s *RuleS2SResourceService) findServiceAliasesForService(ctx context.Context, reader ports.Reader, serviceID models.ResourceIdentifier) ([]models.ServiceAlias, error) {
	var serviceAliases []models.ServiceAlias

	err := reader.ListServiceAliases(ctx, func(alias models.ServiceAlias) error {
		if alias.ServiceRef.Name == serviceID.Name && alias.ServiceRef.Namespace == serviceID.Namespace {
			serviceAliases = append(serviceAliases, alias)
		}
		return nil
	}, ports.EmptyScope{})

	return serviceAliases, err
}

// findRuleS2SReferencingServiceAlias finds all RuleS2S that reference a specific ServiceAlias
func (s *RuleS2SResourceService) findRuleS2SReferencingServiceAlias(ctx context.Context, reader ports.Reader, aliasID models.ResourceIdentifier) ([]models.RuleS2S, error) {
	var rules []models.RuleS2S

	err := reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		// Check ServiceLocalRef
		if rule.ServiceLocalRef.Name == aliasID.Name && rule.ServiceLocalRef.Namespace == aliasID.Namespace {
			rules = append(rules, rule)
			return nil
		}
		// Check ServiceRef
		if rule.ServiceRef.Name == aliasID.Name && rule.ServiceRef.Namespace == aliasID.Namespace {
			rules = append(rules, rule)
		}
		return nil
	}, ports.EmptyScope{})

	return rules, err
}

// findRuleS2SByAddressGroupInteraction finds all RuleS2S where the specified AddressGroup appears in aggregation
func (s *RuleS2SResourceService) findRuleS2SByAddressGroupInteraction(ctx context.Context, reader ports.Reader, addressGroup models.AddressGroupRef) ([]models.RuleS2S, error) {
	var rules []models.RuleS2S

	err := reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		// Extract aggregation groups from this rule to see if it interacts with the AddressGroup
		groups, err := s.extractAggregationGroupsFromRuleS2S(ctx, reader, rule)
		if err != nil {
			// Skip this rule if we can't analyze it
			log.Printf("⚠️ findRuleS2SByAddressGroupInteraction: Failed to extract aggregation groups from rule %s: %v", rule.Key(), err)
			return nil
		}

		// Check if any aggregation group involves the specified AddressGroup
		for _, group := range groups {
			if (group.LocalAG.Name == addressGroup.Name && group.LocalAG.Namespace == addressGroup.Namespace) ||
				(group.TargetAG.Name == addressGroup.Name && group.TargetAG.Namespace == addressGroup.Namespace) {
				rules = append(rules, rule)
				log.Printf("🔍 findRuleS2SByAddressGroupInteraction: Rule %s interacts with AddressGroup %s/%s via aggregation", rule.Key(), addressGroup.Namespace, addressGroup.Name)
				return nil // Found interaction, no need to check more groups for this rule
			}
		}

		return nil
	}, ports.EmptyScope{})

	return rules, err
}

// regenerateAllIEAgAgRules is a safety fallback that regenerates all IEAgAg rules
func (s *RuleS2SResourceService) regenerateAllIEAgAgRules(ctx context.Context, reader ports.Reader, reason string) error {
	log.Printf("🚨 regenerateAllIEAgAgRules: FULL REGENERATION triggered - reason: %s", reason)

	// Get all RuleS2S
	var allRules []models.RuleS2S
	err := reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		allRules = append(allRules, rule)
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to list all RuleS2S for full regeneration")
	}

	log.Printf("🚨 regenerateAllIEAgAgRules: Regenerating all IEAgAg rules for %d RuleS2S", len(allRules))
	return s.regenerateIEAgAgRulesForRuleS2SList(ctx, allRules)
}

// processIEAgAgRuleConditionsInMemory processes conditions for an IEAgAgRule in memory
// without saving them to database (used before transaction commit to eliminate race conditions)
func (s *RuleS2SResourceService) processIEAgAgRuleConditionsInMemory(ctx context.Context, rule *models.IEAgAgRule) error {
	if s.conditionManager == nil {
		return fmt.Errorf("conditionManager is nil")
	}

	klog.Infof("🧠 MEMORY_CONDITION: Processing conditions in memory for IEAgAgRule %s/%s", rule.Namespace, rule.Name)

	// Clear old error conditions and update metadata (same as ConditionManager)
	rule.Meta.ClearErrorCondition()
	rule.Meta.TouchOnWrite("v1")

	// Create a reader for validation (transaction already contains the new data)
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		rule.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Failed to get reader for validation: %v", err))
		rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation unavailable")
		return err
	}

	// Set Synced condition (rule has been synced to backend)
	rule.Meta.SetSyncedCondition(metav1.ConditionTrue, models.ReasonSynced, "IEAgAgRule committed to backend successfully")

	// Validate the rule exists and has proper structure
	if _, err := reader.GetIEAgAgRuleByID(ctx, rule.ResourceIdentifier); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			// Rule doesn't exist yet (expected during creation)
			klog.Infof("🧠 MEMORY_CONDITION: Rule %s not found in reader (expected during creation)", rule.Key())
		} else {
			rule.Meta.SetErrorCondition(models.ReasonBackendError, fmt.Sprintf("Backend validation failed: %v", err))
			rule.Meta.SetReadyCondition(metav1.ConditionFalse, models.ReasonNotReady, "Backend validation failed")
			return err
		}
	}

	// Set Validated condition (rule passes validation)
	rule.Meta.SetValidatedCondition(metav1.ConditionTrue, models.ReasonValidated, "IEAgAgRule passed validation")

	// Set Ready condition (rule is ready with configured ports)
	portCount := len(rule.Ports)
	if portCount > 0 {
		rule.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, fmt.Sprintf("IEAgAgRule is ready, %d ports configured", portCount))
	} else {
		rule.Meta.SetReadyCondition(metav1.ConditionTrue, models.ReasonReady, "IEAgAgRule is ready")
	}

	klog.Infof("🧠 MEMORY_CONDITION: Successfully processed %d conditions in memory for %s", len(rule.Meta.Conditions), rule.Key())
	return nil
}

// applyTimingFixToIEAgAgRules applies the timing fix universally to any IEAgAgRules slice
// This eliminates race conditions by processing conditions BEFORE transaction commit
func (s *RuleS2SResourceService) applyTimingFixToIEAgAgRules(ctx context.Context, writer ports.Writer, rules []models.IEAgAgRule) error {
	if len(rules) == 0 {
		return nil
	}

	klog.Infof("🚀 UNIVERSAL_TIMING_FIX: Applying timing fix to %d IEAgAgRules, conditionManager nil? %v", len(rules), s.conditionManager == nil)

	if s.conditionManager == nil {
		klog.Warningf("⚠️ UNIVERSAL_TIMING_FIX: conditionManager is NIL - conditions will NOT be processed for %d IEAgAgRules (race condition possible)", len(rules))
		return nil
	}

	// Process conditions in memory for all rules
	for i := range rules {
		klog.Infof("🚀 UNIVERSAL_TIMING_FIX: Processing conditions for IEAgAgRule %s/%s (before commit)", rules[i].Namespace, rules[i].Name)
		klog.Infof("🚀 UNIVERSAL_TIMING_FIX: Rule %s has %d current conditions: %v", rules[i].Key(), len(rules[i].Meta.Conditions), rules[i].Meta.Conditions)

		// Process conditions in memory (don't save to database yet)
		if err := s.processIEAgAgRuleConditionsInMemory(ctx, &rules[i]); err != nil {
			klog.Errorf("❌ UNIVERSAL_TIMING_FIX: Failed to process IEAgAg rule conditions for %s: %v", rules[i].Key(), err)
		} else {
			klog.Infof("✅ UNIVERSAL_TIMING_FIX: Successfully processed conditions for %s", rules[i].Key())
			klog.Infof("🚀 UNIVERSAL_TIMING_FIX: Rule %s now has %d conditions after processing: %v", rules[i].Key(), len(rules[i].Meta.Conditions), rules[i].Meta.Conditions)
		}
	}

	// Re-sync rules with updated conditions in the same transaction
	klog.Infof("🚀 UNIVERSAL_TIMING_FIX: Re-syncing %d IEAgAgRules with conditions included", len(rules))
	if err := writer.SyncIEAgAgRules(ctx, rules, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert)); err != nil {
		return errors.Wrap(err, "failed to sync IEAgAg rules with conditions via universal timing fix")
	}

	klog.Infof("✅ UNIVERSAL_TIMING_FIX: Successfully applied timing fix to %d IEAgAgRules", len(rules))
	return nil
}

// applyTimingFixConditionsOnly processes conditions in memory without syncing (for use before manual sync)
func (s *RuleS2SResourceService) applyTimingFixConditionsOnly(ctx context.Context, rules []models.IEAgAgRule) error {
	if len(rules) == 0 {
		return nil
	}

	klog.Infof("🚀 TIMING_FIX_CONDITIONS_ONLY: Processing conditions for %d IEAgAgRules, conditionManager nil? %v", len(rules), s.conditionManager == nil)

	if s.conditionManager == nil {
		klog.Warningf("⚠️ TIMING_FIX_CONDITIONS_ONLY: conditionManager is NIL - conditions will NOT be processed for %d IEAgAgRules (race condition possible)", len(rules))
		return nil
	}

	// Process conditions in memory for all rules
	for i := range rules {
		klog.Infof("🚀 TIMING_FIX_CONDITIONS_ONLY: Processing conditions for IEAgAgRule %s/%s", rules[i].Namespace, rules[i].Name)

		// Process conditions in memory (don't save to database yet)
		if err := s.processIEAgAgRuleConditionsInMemory(ctx, &rules[i]); err != nil {
			klog.Errorf("❌ TIMING_FIX_CONDITIONS_ONLY: Failed to process IEAgAg rule conditions for %s: %v", rules[i].Key(), err)
		} else {
			klog.Infof("✅ TIMING_FIX_CONDITIONS_ONLY: Successfully processed conditions for %s", rules[i].Key())
		}
	}

	klog.Infof("✅ TIMING_FIX_CONDITIONS_ONLY: Successfully processed conditions for %d IEAgAgRules (sync required separately)", len(rules))
	return nil
}

// 🚀 ADDRESS GROUP COMBINATION HELPERS: Proper aggregation logic
// These functions implement the correct AddressGroup combination matching
// based on understanding the reference implementation's actual aggregation pattern

// generateAGCombinations generates all localAG→targetAG combinations for a rule
// This matches the reference implementation's nested loop: for localAG.Ref, for targetAG
func (s *RuleS2SResourceService) generateAGCombinations(
	localService, targetService *models.Service,
	traffic models.Traffic,
) []string {
	var combinations []string

	klog.V(2).Infof("  🔧 GENERATE_COMBINATIONS: Starting for traffic=%s, local=%s, target=%s",
		traffic, localService.Key(), targetService.Key())

	// Get all protocols that have actual ports in the relevant service (for port extraction)
	var portsSource *models.Service
	if traffic == models.INGRESS {
		portsSource = localService
	} else {
		portsSource = targetService
	}

	klog.V(2).Infof("  🔧 GENERATE_COMBINATIONS: Using ports from %s service: %s",
		traffic, portsSource.Key())

	// Collect protocols that actually have ports
	protocolsWithPorts := make(map[models.TransportProtocol]bool)
	for _, port := range portsSource.IngressPorts {
		protocolsWithPorts[port.Protocol] = true
		klog.V(2).Infof("  🔧 GENERATE_COMBINATIONS: Found port %s with protocol %s",
			port.Port, port.Protocol)
	}

	klog.V(2).Infof("  🔧 GENERATE_COMBINATIONS: Detected protocols: %v",
		protocolsWithPorts)

	// Generate combinations for each localAG x targetAG x protocol
	// This mirrors reference implementation lines 427-428: for localAG.Ref, for targetAG
	for _, localAG := range localService.GetAggregatedAddressGroups() {
		for _, targetAG := range targetService.GetAggregatedAddressGroups() {
			for protocol := range protocolsWithPorts {
				// Format: traffic-localAG_namespace/name-targetAG_namespace/name-protocol
				combination := fmt.Sprintf("%s-%s/%s-%s/%s-%s",
					traffic,
					localAG.Ref.Namespace, localAG.Ref.Name,
					targetAG.Ref.Namespace, targetAG.Ref.Name,
					protocol)
				combinations = append(combinations, combination)
				klog.V(2).Infof("  🔧 GENERATE_COMBINATIONS: Generated combination: %s", combination)
			}
		}
	}

	klog.V(2).Infof("  🔧 GENERATE_COMBINATIONS: Final combinations (%d total): %v",
		len(combinations), combinations)

	return combinations
}

// =============================================================================
// IEAgAgRule Cleanup Operations (Reactive System)
// =============================================================================

// CleanupIEAgAgRulesForRuleS2S triggers aggregation regeneration when RuleS2S becomes Ready=False
// This leverages the existing aggregation system to naturally exclude not-ready RuleS2S
// ENHANCED: Now includes external sync for deleted IEAgAgRules
func (s *RuleS2SResourceService) CleanupIEAgAgRulesForRuleS2S(ctx context.Context, ruleS2S models.RuleS2S) error {
	klog.Infof("🧹 CleanupIEAgAgRulesForRuleS2S: Starting aggregation-based cleanup WITH external sync for not-ready RuleS2S %s/%s",
		ruleS2S.Namespace, ruleS2S.Name)

	// The key insight: The existing aggregation system only processes Ready=True RuleS2S
	// So when a RuleS2S becomes Ready=False, we need to regenerate the aggregation groups
	// it was participating in, which will naturally exclude it and update/delete IEAgAgRules
	//
	// ENHANCED: We now also capture existing rules before regeneration and sync deletions to sgroups

	// Step 1: Find the services this RuleS2S was connecting
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for cleanup")
	}
	defer reader.Close()

	// Get the Service references directly from the not-ready RuleS2S (no ServiceAlias lookup needed)
	localServiceID := models.ResourceIdentifier{
		Name:      ruleS2S.ServiceLocalRef.Name,
		Namespace: ruleS2S.ServiceLocalRef.Namespace,
	}
	targetServiceID := models.ResourceIdentifier{
		Name:      ruleS2S.ServiceRef.Name,
		Namespace: ruleS2S.ServiceRef.Namespace,
	}

	// Step 3: Trigger regeneration for the affected services
	// This will cause the aggregation system to regenerate all IEAgAgRules for these services,
	// naturally excluding the not-ready RuleS2S

	// Add both services to affected list (avoiding duplicates if same service)
	var affectedServices []models.ResourceIdentifier
	affectedServices = append(affectedServices, localServiceID)
	klog.Infof("🧹 CleanupIEAgAgRulesForRuleS2S: Will regenerate aggregation for local service %s", localServiceID.Key())

	// Avoid duplicate if it's the same service
	if localServiceID.Key() != targetServiceID.Key() {
		affectedServices = append(affectedServices, targetServiceID)
		klog.Infof("🧹 CleanupIEAgAgRulesForRuleS2S: Will regenerate aggregation for target service %s", targetServiceID.Key())
	}

	if len(affectedServices) == 0 {
		klog.Infof("🧹 CleanupIEAgAgRulesForRuleS2S: No services found to regenerate for RuleS2S %s/%s", ruleS2S.Namespace, ruleS2S.Name)
		return nil
	}

	// 🆕 EXTERNAL SYNC ENHANCEMENT: Step 4 - Capture existing IEAgAgRules before regeneration
	// This enables us to determine which rules were deleted and sync those deletions to sgroups
	klog.Infof("🔍 CleanupIEAgAgRulesForRuleS2S: Capturing existing IEAgAgRules before regeneration for external sync")

	var existingIEAgAgRules []models.IEAgAgRule

	// Collect all existing IEAgAgRules for affected services
	for _, serviceID := range affectedServices {
		var serviceRules []models.IEAgAgRule
		err := reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
			// Check if this rule involves the affected service (either as local or target)
			if (rule.AddressGroupLocal.Namespace == serviceID.Namespace) ||
				(rule.AddressGroup.Namespace == serviceID.Namespace) {
				serviceRules = append(serviceRules, rule)
			}
			return nil
		}, ports.EmptyScope{})

		if err != nil {
			klog.Errorf("⚠️ CleanupIEAgAgRulesForRuleS2S: Failed to list existing IEAgAgRules for service %s: %v", serviceID.Key(), err)
			// Continue with cleanup even if we can't get existing rules
		} else {
			existingIEAgAgRules = append(existingIEAgAgRules, serviceRules...)
			klog.Infof("🔍 CleanupIEAgAgRulesForRuleS2S: Found %d existing IEAgAgRules for service %s", len(serviceRules), serviceID.Key())
		}
	}

	klog.Infof("🔍 CleanupIEAgAgRulesForRuleS2S: Total existing IEAgAgRules before regeneration: %d", len(existingIEAgAgRules))

	// Step 5: Trigger the existing aggregation regeneration system for affected services
	// This will cause all IEAgAgRules involving these services to be recalculated,
	// naturally excluding the not-ready RuleS2S
	klog.Infof("🔄 CleanupIEAgAgRulesForRuleS2S: Triggering aggregation regeneration for %d affected services", len(affectedServices))

	for _, serviceID := range affectedServices {
		err := s.RegenerateIEAgAgRulesForService(ctx, serviceID)
		if err != nil {
			klog.Errorf("❌ CleanupIEAgAgRulesForRuleS2S: Failed to regenerate aggregation for service %s: %v", serviceID.Key(), err)
			// Continue with other services even if one fails
		} else {
			klog.Infof("✅ CleanupIEAgAgRulesForRuleS2S: Successfully regenerated aggregation for service %s", serviceID.Key())
		}
	}

	// 🆕 EXTERNAL SYNC ENHANCEMENT: Step 6 - Compare before/after and sync deletions to sgroups
	if len(existingIEAgAgRules) > 0 && s.syncManager != nil {
		klog.Infof("🔄 CleanupIEAgAgRulesForRuleS2S: Checking for deleted rules to sync to sgroups")

		// Get new reader to see post-regeneration state
		newReader, err := s.registry.Reader(ctx)
		if err != nil {
			klog.Errorf("⚠️ CleanupIEAgAgRulesForRuleS2S: Failed to get reader for post-regeneration sync: %v", err)
		} else {
			defer newReader.Close()

			var currentIEAgAgRules []models.IEAgAgRule

			// Collect current IEAgAgRules after regeneration
			for _, serviceID := range affectedServices {
				err := newReader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
					if (rule.AddressGroupLocal.Namespace == serviceID.Namespace) ||
						(rule.AddressGroup.Namespace == serviceID.Namespace) {
						currentIEAgAgRules = append(currentIEAgAgRules, rule)
					}
					return nil
				}, ports.EmptyScope{})

				if err != nil {
					klog.Errorf("⚠️ CleanupIEAgAgRulesForRuleS2S: Failed to list current IEAgAgRules for service %s: %v", serviceID.Key(), err)
				}
			}

			// Find rules that existed before but not after (these were deleted)
			deletedRules := s.findDeletedIEAgAgRules(existingIEAgAgRules, currentIEAgAgRules)

			if len(deletedRules) > 0 {

				// Sync each deleted rule to sgroups with DELETE operation
				for _, deletedRule := range deletedRules {
					if err := s.syncManager.SyncEntity(ctx, &deletedRule, types.SyncOperationDelete); err != nil {
						klog.Errorf("❌ CleanupIEAgAgRulesForRuleS2S: Failed to sync deleted rule %s to sgroups: %v", deletedRule.GetSyncKey(), err)
					}
				}
			}
		}
	} else if s.syncManager == nil {
		klog.Warningf("⚠️ CleanupIEAgAgRulesForRuleS2S: syncManager is nil - external sync SKIPPED")
	}

	return nil
}

// findDeletedIEAgAgRules compares two slices of IEAgAgRules and returns rules that exist in 'before' but not in 'after'
func (s *RuleS2SResourceService) findDeletedIEAgAgRules(before, after []models.IEAgAgRule) []models.IEAgAgRule {
	// Create a map of current rules for efficient lookup using namespace/name as key
	currentRulesMap := make(map[string]bool)
	for _, rule := range after {
		key := rule.SelfRef.ResourceIdentifier.Key() // Use ResourceIdentifier.Key() method
		currentRulesMap[key] = true
	}

	// Find rules that existed before but don't exist now
	var deletedRules []models.IEAgAgRule
	for _, rule := range before {
		key := rule.SelfRef.ResourceIdentifier.Key() // Use ResourceIdentifier.Key() method
		if !currentRulesMap[key] {
			deletedRules = append(deletedRules, rule)
		}
	}

	return deletedRules
}

func (s *RuleS2SResourceService) aggregateTraceValue(contributingRules []models.RuleS2S) bool {
	if len(contributingRules) == 0 {
		return false // No contributing rules means no trace
	}

	for _, rule := range contributingRules {
		if !rule.Trace {
			return false
		}
	}

	return true
}
