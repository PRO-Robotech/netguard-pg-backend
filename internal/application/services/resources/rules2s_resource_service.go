package resources

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

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
	log.Printf("CreateRuleS2S: Starting creation of RuleS2S %s", rule.Key())

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Validate rule for creation
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	log.Printf("CreateRuleS2S: Validating RuleS2S %s", rule.Key())
	if err := ruleValidator.ValidateForCreation(ctx, rule); err != nil {
		log.Printf("CreateRuleS2S: Validation failed for RuleS2S %s: %v", rule.Key(), err)
		return err
	}
	log.Printf("CreateRuleS2S: Validation passed for RuleS2S %s", rule.Key())

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

	// Use syncRuleS2S for IEAgAgRule generation and IEAgAgRuleRefs population
	log.Printf("CreateRuleS2S: Syncing RuleS2S %s with IEAgAgRule generation", rule.Key())
	if err = s.syncRuleS2S(ctx, writer, []models.RuleS2S{rule}, models.SyncOpUpsert); err != nil {
		log.Printf("CreateRuleS2S: Failed to sync RuleS2S %s: %v", rule.Key(), err)
		return errors.Wrap(err, "failed to create rule s2s")
	}

	if err = writer.Commit(); err != nil {
		log.Printf("CreateRuleS2S: Failed to commit transaction for RuleS2S %s: %v", rule.Key(), err)
		return errors.Wrap(err, "failed to commit")
	}
	log.Printf("CreateRuleS2S: Successfully committed RuleS2S %s", rule.Key())

	// 🎯 CRITICAL FIX: After successful RuleS2S creation, trigger IEAgAgRule regeneration
	// This handles the timing issue where AddressGroupBindings existed before RuleS2S creation
	// The dependency chain: AddressGroupBinding → Service.AddressGroups → RuleS2S → IEAgAgRule
	// If AddressGroupBindings were created before this RuleS2S, we need to manually trigger regeneration
	log.Printf("🔄 CreateRuleS2S: Triggering post-creation IEAgAgRule regeneration for %s", rule.Key())
	if err := s.triggerPostCreationIEAgAgRuleGeneration(ctx, rule); err != nil {
		log.Printf("⚠️ CreateRuleS2S: Failed to trigger post-creation IEAgAgRule regeneration for %s: %v", rule.Key(), err)
		// Don't fail the entire creation, but log the issue
	}

	// Process conditions
	if s.conditionManager != nil {
		log.Printf("CreateRuleS2S: Processing conditions for RuleS2S %s", rule.Key())
		if err := s.conditionManager.ProcessRuleS2SConditions(ctx, &rule); err != nil {
			log.Printf("CreateRuleS2S: Failed to process conditions for RuleS2S %s: %v", rule.Key(), err)
			return errors.Wrap(err, "failed to process rule s2s conditions")
		}
		// Note: ProcessRuleS2SConditions already saves the conditions internally
	}

	log.Printf("CreateRuleS2S: Successfully created RuleS2S %s", rule.Key())
	return nil
}

// UpdateRuleS2S updates an existing RuleS2S
func (s *RuleS2SResourceService) UpdateRuleS2S(ctx context.Context, rule models.RuleS2S) error {
	log.Printf("UpdateRuleS2S: Starting update of RuleS2S %s", rule.Key())

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Get existing rule for validation AND condition preservation
	existingRule, err := reader.GetRuleS2SByID(ctx, rule.ResourceIdentifier)
	if err != nil {
		return errors.Wrap(err, "failed to get existing RuleS2S")
	}

	// 🔍 CONDITION_PRESERVATION: Preserve existing conditions during update to prevent race condition
	log.Printf("🔍 UPDATE_CONDITIONS: Preserving existing conditions from current RuleS2S %s", rule.Key())
	log.Printf("   - Existing conditions count: %d", len(existingRule.Meta.Conditions))
	for i, cond := range existingRule.Meta.Conditions {
		log.Printf("   - Condition[%d]: Type=%s, Status=%s, Reason=%s", i, cond.Type, cond.Status, cond.Reason)
	}

	// Preserve existing conditions in the update rule to prevent overwrite
	if len(existingRule.Meta.Conditions) > 0 {
		rule.Meta.Conditions = existingRule.Meta.Conditions
		rule.Meta.Generation = existingRule.Meta.Generation // Preserve generation too
		log.Printf("🔄 UPDATE_CONDITIONS: Preserved %d existing conditions in update rule", len(rule.Meta.Conditions))
	} else {
		log.Printf("ℹ️ UPDATE_CONDITIONS: No existing conditions to preserve for %s", rule.Key())
	}

	// Validate rule for update
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	log.Printf("UpdateRuleS2S: Validating RuleS2S %s", rule.Key())
	if err := ruleValidator.ValidateForUpdate(ctx, *existingRule, rule); err != nil {
		log.Printf("UpdateRuleS2S: Validation failed for RuleS2S %s: %v", rule.Key(), err)
		return err
	}
	log.Printf("UpdateRuleS2S: Validation passed for RuleS2S %s", rule.Key())

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

	// Use syncRuleS2S for IEAgAgRule generation and updates
	log.Printf("UpdateRuleS2S: Syncing RuleS2S %s with IEAgAgRule updates", rule.Key())
	if err = s.syncRuleS2S(ctx, writer, []models.RuleS2S{rule}, models.SyncOpUpsert); err != nil {
		log.Printf("UpdateRuleS2S: Failed to sync RuleS2S %s: %v", rule.Key(), err)
		return errors.Wrap(err, "failed to update rule s2s")
	}

	if err = writer.Commit(); err != nil {
		log.Printf("UpdateRuleS2S: Failed to commit transaction for RuleS2S %s: %v", rule.Key(), err)
		return errors.Wrap(err, "failed to commit")
	}
	log.Printf("UpdateRuleS2S: Successfully committed RuleS2S %s", rule.Key())

	// 🔍 ENHANCED CONDITION DEBUGGING: Process conditions with detailed tracking
	if s.conditionManager != nil {
		log.Printf("🔍 UPDATE_CONDITIONS: Starting condition processing for RuleS2S %s", rule.Key())
		log.Printf("🔍 UPDATE_CONDITIONS: Pre-processing rule state:")
		log.Printf("   - ServiceLocalRef: %s/%s", rule.ServiceLocalRef.Namespace, rule.ServiceLocalRef.Name)
		log.Printf("   - ServiceRef: %s/%s", rule.ServiceRef.Namespace, rule.ServiceRef.Name)
		log.Printf("   - Traffic: %s", rule.Traffic)
		log.Printf("   - Generation: %d", rule.Meta.Generation)

		// Process conditions with enhanced error reporting
		if err := s.conditionManager.ProcessRuleS2SConditions(ctx, &rule); err != nil {
			log.Printf("❌ UPDATE_CONDITIONS: FAILED to process conditions for RuleS2S %s: %v", rule.Key(), err)
			log.Printf("❌ UPDATE_CONDITIONS: Failure context:")
			log.Printf("   - conditionManager type: %T", s.conditionManager)
			log.Printf("   - Error details: %v", err)
			return errors.Wrap(err, "failed to process rule s2s conditions")
		}

		log.Printf("✅ UPDATE_CONDITIONS: Successfully processed conditions for RuleS2S %s", rule.Key())
		log.Printf("🔍 UPDATE_CONDITIONS: Post-processing - conditions should be saved internally")
		// Note: ProcessRuleS2SConditions already saves the conditions internally
	} else {
		log.Printf("⚠️ UPDATE_CONDITIONS: conditionManager is NIL for RuleS2S %s - conditions will NOT be processed!", rule.Key())
	}

	log.Printf("UpdateRuleS2S: Successfully updated RuleS2S %s", rule.Key())
	return nil
}

// SyncRuleS2S synchronizes multiple RuleS2S
func (s *RuleS2SResourceService) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope, syncOp models.SyncOp) error {
	// 🎯 CRITICAL FIX: For DELETE operations, use our enhanced DeleteRuleS2SByIDs method
	// which includes targeted cleanup to prevent mass external sync DELETE bug
	if syncOp == models.SyncOpDelete {
		klog.Infof("🗑️ SYNC_DELETE_REDIRECT: Redirecting DELETE sync to enhanced DeleteRuleS2SByIDs for %d rules", len(rules))

		// Extract resource identifiers from rules to delete
		var idsToDelete []models.ResourceIdentifier
		for _, rule := range rules {
			idsToDelete = append(idsToDelete, rule.ResourceIdentifier)
			klog.Infof("  🎯 SYNC_DELETE_REDIRECT: Will delete RuleS2S %s via enhanced method", rule.Key())
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

	// 🎯 CRITICAL FIX: After successful sync commit, trigger IEAgAgRule regeneration for timing issues
	// This handles cases where AddressGroupBindings existed before RuleS2S creation via K8s API server
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
			ruleName := fmt.Sprintf("%s/%s", rules[i].Namespace, rules[i].Name)
			klog.Infof("🔄 SyncRuleS2S: [%d/%d] Processing conditions for RuleS2S %s", i+1, len(rules), ruleName)

			// Pre-condition checks for detailed diagnosis
			klog.Infof("🔍 SyncRuleS2S: Pre-checks for %s - ServiceLocalRef=%s/%s, ServiceRef=%s/%s",
				ruleName,
				rules[i].ServiceLocalRef.Namespace, rules[i].ServiceLocalRef.Name,
				rules[i].ServiceRef.Namespace, rules[i].ServiceRef.Name)

			if err := s.conditionManager.ProcessRuleS2SConditions(ctx, &rules[i]); err != nil {
				failureCount++
				klog.Errorf("❌ SyncRuleS2S: [%d/%d] FAILED to process conditions for %s: %v",
					i+1, len(rules), ruleName, err)
				klog.Errorf("❌ SyncRuleS2S: Failure details for %s:", ruleName)
				klog.Errorf("   - ServiceLocalRef: %s/%s", rules[i].ServiceLocalRef.Namespace, rules[i].ServiceLocalRef.Name)
				klog.Errorf("   - ServiceRef: %s/%s", rules[i].ServiceRef.Namespace, rules[i].ServiceRef.Name)
				klog.Errorf("   - Traffic: %s", rules[i].Traffic)
				klog.Errorf("   - Error: %v", err)
				// Don't fail the operation if condition processing fails, but track it
			} else {
				successCount++
				klog.Infof("✅ SyncRuleS2S: [%d/%d] SUCCESS processing conditions for %s", i+1, len(rules), ruleName)
			}
			// Note: ProcessRuleS2SConditions already saves the conditions internally
		}
	}

	// Summary logging to identify patterns
	if failureCount > 0 {
		klog.Errorf("🚨 SyncRuleS2S: BULK CONDITION PROCESSING SUMMARY - Success: %d/%d, Failures: %d/%d",
			successCount, len(rules), failureCount, len(rules))
		klog.Errorf("🚨 SyncRuleS2S: This indicates dependency resolution or validation issues during bulk operations!")
	} else {
		klog.Infof("✅ SyncRuleS2S: BULK CONDITION PROCESSING SUCCESS - All %d/%d RuleS2S got conditions", successCount, len(rules))
	}

	return nil
}

// DeleteRuleS2SByIDs deletes RuleS2S by IDs and triggers targeted IEAgAg rule cleanup
func (s *RuleS2SResourceService) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	klog.Infof("🗑️ RULES2S_DELETE: Starting deletion of %d RuleS2S with targeted cleanup", len(ids))

	// Validate dependencies for each RuleS2S
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for validation")
	}
	defer reader.Close()

	validator := validation.NewDependencyValidator(reader)
	ruleS2SValidator := validator.GetRuleS2SValidator()

	for _, id := range ids {
		log.Printf("DeleteRuleS2SByIDs: Validating dependencies for RuleS2S %s", id.Key())
		if err := ruleS2SValidator.CheckDependencies(ctx, id); err != nil {
			log.Printf("DeleteRuleS2SByIDs: Cannot delete RuleS2S %s due to dependencies: %v", id.Key(), err)
			return errors.Wrapf(err, "cannot delete RuleS2S %s", id.Key())
		}
	}

	log.Printf("DeleteRuleS2SByIDs: All %d RuleS2S validated for deletion", len(ids))

	// 🎯 CRITICAL FIX: Capture IEAgAgRules that are referenced by RuleS2S being deleted BEFORE deletion
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
		for _, ieagagRef := range rule.IEAgAgRuleRefs {
			refID := models.ResourceIdentifier{
				Namespace: ieagagRef.Namespace,
				Name:      ieagagRef.Name,
			}
			referencedIEAgAgRules = append(referencedIEAgAgRules, refID)
			klog.Infof("🎯 RULES2S_DELETE: RuleS2S %s references IEAgAgRule %s for cleanup", id.Key(), refID.Key())
		}
	}

	klog.Infof("🎯 RULES2S_DELETE: Found %d IEAgAgRules referenced by RuleS2S being deleted", len(referencedIEAgAgRules))

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

	klog.Infof("✅ RULES2S_DELETE: Successfully deleted %d RuleS2S", len(ids))

	// Step 2: 🎯 CRITICAL FIX: Targeted cleanup for ONLY the affected IEAgAgRules
	// This prevents the massive DELETE operation bug by only affecting rules that were
	// actually generated by the deleted RuleS2S
	if len(referencedIEAgAgRules) > 0 {
		klog.Infof("🔄 RULES2S_DELETE: Triggering targeted cleanup for %d affected IEAgAgRules", len(referencedIEAgAgRules))

		// 🔍 DEBUG: Log each referenced IEAgAgRule before cleanup
		for i, ruleID := range referencedIEAgAgRules {
			klog.Infof("  📋 RULES2S_DELETE[%d]: Referenced IEAgAgRule: %s", i+1, ruleID.Key())
		}

		// Use targeted cleanup that only affects specific referenced rules
		reason := fmt.Sprintf("rules2s-deletion-cleanup-%d-rules", len(ids))
		klog.Infof("🚀 RULES2S_DELETE: CALLING RecalculateTargetedIEAgAgRules with reason: %s", reason)

		if err := s.RecalculateTargetedIEAgAgRules(ctx, referencedIEAgAgRules, reason); err != nil {
			klog.Errorf("⚠️ RULES2S_DELETE: Targeted cleanup failed after deletion: %v", err)
			// Don't fail the deletion for recalculation errors, just log them
		} else {
			klog.Infof("✅ RULES2S_DELETE: Targeted cleanup completed successfully")
		}
	} else {
		klog.Infof("✅ RULES2S_DELETE: No IEAgAgRules referenced by deleted RuleS2S, no cleanup needed")
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
			klog.Infof("🔄 SYNC_CONDITION_DEBUG: Processing conditions for IEAgAgRule %s/%s", rules[i].Namespace, rules[i].Name)
			klog.Infof("🔄 SYNC_CONDITION_DEBUG: Rule %s has %d conditions before: %v", rules[i].Key(), len(rules[i].Meta.Conditions), rules[i].Meta.Conditions)

			if err := s.conditionManager.ProcessIEAgAgRuleConditions(ctx, &rules[i]); err != nil {
				klog.Errorf("❌ SYNC_CONDITION_DEBUG: Failed to process IEAgAgRule conditions for %s/%s: %v",
					rules[i].Namespace, rules[i].Name, err)
				// Don't fail the operation if condition processing fails
			} else {
				klog.Infof("✅ SYNC_CONDITION_DEBUG: Successfully processed conditions for %s", rules[i].Key())
				klog.Infof("🔄 SYNC_CONDITION_DEBUG: Rule %s now has %d conditions after: %v", rules[i].Key(), len(rules[i].Meta.Conditions), rules[i].Meta.Conditions)
			}
			// Note: ProcessIEAgAgRuleConditions already saves the conditions internally
		}
	} else {
		klog.Warningf("⚠️ SYNC_CONDITION_DEBUG: conditionManager is NIL in SyncIEAgAgRules - no conditions will be processed for %d IEAgAgRules", len(rules))
	}

	return nil
}

// DeleteIEAgAgRulesByIDs deletes IEAgAgRules by IDs WITH external sync
func (s *RuleS2SResourceService) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	klog.Infof("🗑️ IEAGAG_DELETE: Starting deletion of %d IEAgAgRules with external sync", len(ids))

	// CRITICAL FIX: First get the rules to delete for external sync BEFORE deletion
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
		klog.Infof("  📋 IEAGAG_DELETE: Prepared rule for deletion: %s", rule.SelfRef.Key())
	}

	// 🔧 SERIALIZATION_FIX: Use WriterForDeletes to reduce serialization conflicts during concurrent delete operations
	var writer ports.Writer
	if registryWithDeletes, ok := s.registry.(interface {
		WriterForDeletes(context.Context) (ports.Writer, error)
	}); ok {
		klog.V(2).Infof("🔧 SERIALIZATION_FIX: Using WriterForDeletes with ReadCommitted isolation for %d rules", len(ids))
		writer, err = registryWithDeletes.WriterForDeletes(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get delete writer with ReadCommitted isolation")
		}
	} else {
		klog.V(2).Infof("🔧 SERIALIZATION_FIX: WriterForDeletes not available, using standard writer for %d rules", len(ids))
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

	// Delete from backend
	klog.Infof("🗄️ IEAGAG_DELETE: Deleting %d rules from backend", len(ids))
	if err = writer.DeleteIEAgAgRulesByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete IEAgAgRules from backend")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit backend deletion transaction")
	}

	// 🔄 CRITICAL FIX: Sync deletions to external systems (SGROUP)
	klog.Infof("🔄 IEAGAG_DELETE: Syncing deletion of %d rules to external systems", len(rulesToDelete))
	if s.syncManager != nil {
		for _, rule := range rulesToDelete {
			if syncErr := s.syncManager.SyncEntity(ctx, &rule, types.SyncOperationDelete); syncErr != nil {
				klog.Errorf("⚠️ IEAGAG_DELETE: Failed to sync deletion of rule %s to external systems: %v", rule.SelfRef.Key(), syncErr)
				// Don't fail the entire deletion for external sync errors, but log them
			} else {
				klog.Infof("✅ IEAGAG_DELETE: Successfully synced deletion of rule %s to external systems", rule.SelfRef.Key())
			}
		}
	} else {
		klog.Warningf("⚠️ IEAGAG_DELETE: syncManager is nil - external sync SKIPPED for %d deleted rules", len(rulesToDelete))
	}

	klog.Infof("✅ IEAGAG_DELETE: Completed deletion of %d IEAgAgRules (backend + external sync)", len(ids))
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

	// Get services referenced by this rule
	localServiceAliasID := models.ResourceIdentifier{
		Name:      ruleS2S.ServiceLocalRef.Name,
		Namespace: ruleS2S.ServiceLocalRef.Namespace,
	}
	localServiceAlias, err := reader.GetServiceAliasByID(ctx, localServiceAliasID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get local service alias %s", ruleS2S.ServiceLocalRef.Name)
	}

	localServiceID := models.ResourceIdentifier{
		Name:      localServiceAlias.ServiceRef.Name,
		Namespace: localServiceAlias.ServiceRef.Namespace,
	}
	localService, err := reader.GetServiceByID(ctx, localServiceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get local service %s", localServiceAlias.ServiceRef.Name)
	}

	targetServiceAliasID := models.ResourceIdentifier{
		Name:      ruleS2S.ServiceRef.Name,
		Namespace: ruleS2S.ServiceRef.Namespace,
	}
	targetServiceAlias, err := reader.GetServiceAliasByID(ctx, targetServiceAliasID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get target service alias %s", ruleS2S.ServiceRef.Name)
	}

	targetServiceID := models.ResourceIdentifier{
		Name:      targetServiceAlias.ServiceRef.Name,
		Namespace: targetServiceAlias.ServiceRef.Namespace,
	}
	targetService, err := reader.GetServiceByID(ctx, targetServiceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get target service %s", targetServiceAlias.ServiceRef.Name)
	}

	// Extract ports based on traffic direction
	var portsSource *models.Service
	if ruleS2S.Traffic == models.INGRESS {
		portsSource = localService
	} else {
		portsSource = targetService
	}

	allPorts := s.extractPortsFromService(*portsSource)
	log.Printf("  🔍 GenerateIEAgAgRules: Traffic=%s, LocalService=%s (ports=%v), TargetService=%s (ports=%v)",
		ruleS2S.Traffic, localService.Key(), localService.IngressPorts, targetService.Key(), targetService.IngressPorts)
	log.Printf("  🔍 GenerateIEAgAgRules: Using portsSource=%s, Found %d ports: %v", portsSource.Key(), len(allPorts), allPorts)

	var generatedRules []models.IEAgAgRule

	// Create rules for all AG combinations
	for _, localAG := range localService.AddressGroups {
		for _, targetAG := range targetService.AddressGroups {
			// Group by protocol
			for _, protocol := range []models.TransportProtocol{models.TCP, models.UDP} {
				// Filter ports by protocol
				var protocolPorts []models.IngressPort
				for _, port := range allPorts {
					if port.Protocol == protocol {
						protocolPorts = append(protocolPorts, port)
					}
				}

				log.Printf("    🔍 Protocol=%s: Found %d ports: %v", protocol, len(protocolPorts), protocolPorts)

				if len(protocolPorts) == 0 {
					log.Printf("    ⏭️ Skipping protocol %s (no ports)", protocol)
					continue // Skip if no ports for this protocol
				}

				// Create aggregated IEAgAgRule with UUID-based name
				ieAgAgRule := models.IEAgAgRule{
					SelfRef: models.SelfRef{
						ResourceIdentifier: models.ResourceIdentifier{
							Name:      s.generateRuleNameForRuleS2S(ruleS2S, localAG, targetAG, protocol),
							Namespace: ruleS2S.Namespace,
						},
					},
					Traffic:           ruleS2S.Traffic,
					Transport:         protocol, // Set the transport protocol
					AddressGroupLocal: localAG,
					AddressGroup:      targetAG,
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

	log.Printf("🔨 GenerateIEAgAgRulesFromRuleS2S: Generated %d IEAgAgRules for RuleS2S %s", len(generatedRules), ruleS2S.Key())
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

// FindRuleS2SForServiceAliases finds all RuleS2S that reference given service aliases
func (s *RuleS2SResourceService) FindRuleS2SForServiceAliases(ctx context.Context, aliasIDs []models.ResourceIdentifier) ([]models.RuleS2S, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var relatedRules []models.RuleS2S

	// Find all RuleS2S that reference any of these service aliases
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		for _, aliasID := range aliasIDs {
			// Check if this rule references the alias in either ServiceRef or ServiceLocalRef
			if (rule.ServiceRef.Name == aliasID.Name && rule.ServiceRef.Namespace == aliasID.Namespace) ||
				(rule.ServiceLocalRef.Name == aliasID.Name && rule.ServiceLocalRef.Namespace == aliasID.Namespace) {
				relatedRules = append(relatedRules, rule)
				break // Don't add the same rule multiple times
			}
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return nil, errors.Wrap(err, "failed to search for rules referencing service aliases")
	}

	log.Printf("FindRuleS2SForServiceAliases: Found %d rules referencing %d service aliases", len(relatedRules), len(aliasIDs))
	return relatedRules, nil
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
	log.Printf("🔄 UpdateIEAgAgRulesForRuleS2S: Processing %d RuleS2S for operation %s (excluding %d rules)", len(rules), syncOp, len(excludeRuleIDs))

	// Track affected services for optimization
	affectedServices := make(map[string]models.ResourceIdentifier)

	// Process each RuleS2S
	for _, rule := range rules {
		log.Printf("🔍 UpdateIEAgAgRulesForRuleS2S: Processing RuleS2S %s", rule.Key())

		// Get services for this rule
		localServiceAliasID := models.ResourceIdentifier{
			Name:      rule.ServiceLocalRef.Name,
			Namespace: rule.ServiceLocalRef.Namespace,
		}

		targetServiceAliasID := models.ResourceIdentifier{
			Name:      rule.ServiceRef.Name,
			Namespace: rule.ServiceRef.Namespace,
		}

		localServiceAlias, err := reader.GetServiceAliasByID(ctx, localServiceAliasID)
		if err != nil {
			log.Printf("❌ UpdateIEAgAgRulesForRuleS2S: Failed to get local service alias %s: %v", localServiceAliasID.Key(), err)
			continue
		}

		targetServiceAlias, err := reader.GetServiceAliasByID(ctx, targetServiceAliasID)
		if err != nil {
			log.Printf("❌ UpdateIEAgAgRulesForRuleS2S: Failed to get target service alias %s: %v", targetServiceAliasID.Key(), err)
			continue
		}

		// Mark services as affected
		localServiceID := models.ResourceIdentifier{
			Name:      localServiceAlias.ServiceRef.Name,
			Namespace: localServiceAlias.Namespace,
		}
		targetServiceID := models.ResourceIdentifier{
			Name:      targetServiceAlias.ServiceRef.Name,
			Namespace: targetServiceAlias.Namespace,
		}

		affectedServices[localServiceID.Key()] = localServiceID
		affectedServices[targetServiceID.Key()] = targetServiceID
	}

	// Update IEAgAgRules for all affected services with exclusions
	return s.UpdateIEAgAgRulesForAffectedServicesWithExclusions(ctx, writer, reader, affectedServices, syncOp, excludeRuleIDs)
}

// UpdateIEAgAgRulesForRuleS2SWithReader updates IEAgAgRules using existing reader
func (s *RuleS2SResourceService) UpdateIEAgAgRulesForRuleS2SWithReader(ctx context.Context, writer ports.Writer, reader ports.Reader, rules []models.RuleS2S, syncOp models.SyncOp) error {
	log.Printf("🔄 UpdateIEAgAgRulesForRuleS2S: Processing %d RuleS2S for operation %s", len(rules), syncOp)

	// Track affected services for optimization
	affectedServices := make(map[string]models.ResourceIdentifier)

	// Process each RuleS2S
	for _, rule := range rules {
		log.Printf("🔍 UpdateIEAgAgRulesForRuleS2S: Processing RuleS2S %s", rule.Key())

		// Get services for this rule
		localServiceAliasID := models.ResourceIdentifier{
			Name:      rule.ServiceLocalRef.Name,
			Namespace: rule.ServiceLocalRef.Namespace,
		}
		targetServiceAliasID := models.ResourceIdentifier{
			Name:      rule.ServiceRef.Name,
			Namespace: rule.ServiceRef.Namespace,
		}

		// Get service aliases and their referenced services
		localServiceAlias, err := reader.GetServiceAliasByID(ctx, localServiceAliasID)
		if err != nil {
			log.Printf("❌ UpdateIEAgAgRulesForRuleS2S: Failed to get local service alias %s: %v", localServiceAliasID.Key(), err)
			continue
		}

		targetServiceAlias, err := reader.GetServiceAliasByID(ctx, targetServiceAliasID)
		if err != nil {
			log.Printf("❌ UpdateIEAgAgRulesForRuleS2S: Failed to get target service alias %s: %v", targetServiceAliasID.Key(), err)
			continue
		}

		// Mark services as affected
		localServiceID := models.ResourceIdentifier{
			Name:      localServiceAlias.ServiceRef.Name,
			Namespace: localServiceAlias.Namespace,
		}
		targetServiceID := models.ResourceIdentifier{
			Name:      targetServiceAlias.ServiceRef.Name,
			Namespace: targetServiceAlias.Namespace,
		}

		affectedServices[localServiceID.Key()] = localServiceID
		affectedServices[targetServiceID.Key()] = targetServiceID
	}

	// Update IEAgAgRules for all affected services
	return s.UpdateIEAgAgRulesForAffectedServicesWithExclusions(ctx, writer, reader, affectedServices, syncOp, nil)
}

// UpdateIEAgAgRulesForAffectedServicesWithExclusions updates IEAgAgRules for services affected by changes with exclusions
func (s *RuleS2SResourceService) UpdateIEAgAgRulesForAffectedServicesWithExclusions(ctx context.Context, writer ports.Writer, reader ports.Reader, affectedServices map[string]models.ResourceIdentifier, syncOp models.SyncOp, excludeRuleIDs []models.ResourceIdentifier) error {
	log.Printf("🔄 UpdateIEAgAgRulesForAffectedServices: Processing %d affected services (excluding %d rules)", len(affectedServices), len(excludeRuleIDs))

	// Collect all RuleS2S for affected services
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

	log.Printf("🔍 ENTRY_POINT: UpdateIEAgAgRulesForAffectedServices - Processing %d unique rules", len(rulesToProcess))
	log.Printf("🔍 ENTRY_POINT_FLOW: About to call generateAggregatedIEAgAgRules → syncIEAgAgRulesWithReader (PATH_B)")

	// Generate new aggregated IEAgAgRules with exclusions
	_, newIEAgAgRules, err := s.generateAggregatedIEAgAgRules(ctx, reader, rulesToProcess, excludeRuleIDs...)
	if err != nil {
		return errors.Wrap(err, "failed to generate aggregated IEAgAgRules")
	}

	// Update IEAgAgRules
	if len(newIEAgAgRules) > 0 {
		log.Printf("🔍 ENTRY_POINT_CRITICAL: About to call PATH_B (syncIEAgAgRulesWithReader) with %d IEAgAgRules - THIS PATH LACKS TIMING FIX!", len(newIEAgAgRules))
		if err := s.syncIEAgAgRulesWithReader(ctx, writer, reader, newIEAgAgRules, syncOp); err != nil {
			return errors.Wrap(err, "failed to sync generated IEAgAgRules")
		}
	}

	log.Printf("✅ UpdateIEAgAgRulesForAffectedServices: Successfully updated IEAgAgRules for %d affected services", len(affectedServices))
	return nil
}

// UpdateIEAgAgRulesForAffectedServices updates IEAgAgRules for services affected by changes
func (s *RuleS2SResourceService) UpdateIEAgAgRulesForAffectedServices(ctx context.Context, writer ports.Writer, reader ports.Reader, affectedServices map[string]models.ResourceIdentifier, syncOp models.SyncOp) error {
	log.Printf("🔄 UpdateIEAgAgRulesForAffectedServices: Processing %d affected services", len(affectedServices))

	// Collect all RuleS2S for affected services
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

	log.Printf("🔍 ENTRY_POINT: UpdateIEAgAgRulesForAffectedServices - Processing %d unique rules", len(rulesToProcess))
	log.Printf("🔍 ENTRY_POINT_FLOW: About to call generateAggregatedIEAgAgRules → syncIEAgAgRulesWithReader (PATH_B)")

	// Generate new aggregated IEAgAgRules
	_, newIEAgAgRules, err := s.generateAggregatedIEAgAgRules(ctx, reader, rulesToProcess)
	if err != nil {
		return errors.Wrap(err, "failed to generate aggregated IEAgAgRules")
	}

	// Update IEAgAgRules
	if len(newIEAgAgRules) > 0 {
		log.Printf("🔍 ENTRY_POINT_CRITICAL: About to call PATH_B (syncIEAgAgRulesWithReader) with %d IEAgAgRules - THIS PATH LACKS TIMING FIX!", len(newIEAgAgRules))
		if err := s.syncIEAgAgRulesWithReader(ctx, writer, reader, newIEAgAgRules, syncOp); err != nil {
			return errors.Wrap(err, "failed to sync generated IEAgAgRules")
		}
	}

	log.Printf("✅ UpdateIEAgAgRulesForAffectedServices: Successfully updated IEAgAgRules for %d affected services", len(affectedServices))
	return nil
}

// =============================================================================
// Private Helper Methods (extracted from original NetguardService)
// =============================================================================

// syncRuleS2S handles the actual RuleS2S synchronization logic with IEAgAgRule generation
func (s *RuleS2SResourceService) syncRuleS2S(ctx context.Context, writer ports.Writer, rules []models.RuleS2S, syncOp models.SyncOp) error {
	log.Printf("syncRuleS2S: Syncing %d RuleS2S with operation %s", len(rules), syncOp)

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

	// Update related IEAgAgRules
	// 🚫 CIRCULAR_DEPENDENCY_FIX: For DELETE operations, exclude the rules being deleted to prevent circular dependency
	var excludeRuleIDs []models.ResourceIdentifier
	if syncOp == models.SyncOpDelete {
		for _, rule := range rules {
			excludeRuleIDs = append(excludeRuleIDs, rule.ResourceIdentifier)
			klog.Infof("🚫 CIRCULAR_DEPENDENCY_FIX: Adding rule %s to exclusion list for DELETE operation", rule.Key())
		}
	}

	if err := s.UpdateIEAgAgRulesForRuleS2SWithReaderAndExclusions(ctx, writer, reader, rules, syncOp, excludeRuleIDs); err != nil {
		return errors.Wrap(err, "failed to update IEAgAgRules for RuleS2S")
	}

	log.Printf("syncRuleS2S: Successfully synced %d RuleS2S", len(rules))
	return nil
}

// syncIEAgAgRules handles the actual IEAgAgRule synchronization logic
func (s *RuleS2SResourceService) syncIEAgAgRules(ctx context.Context, writer ports.Writer, rules []models.IEAgAgRule, syncOp models.SyncOp) error {
	return s.syncIEAgAgRulesWithReader(ctx, writer, nil, rules, syncOp)
}

// syncIEAgAgRulesWithReader handles IEAgAgRule synchronization with existing reader
func (s *RuleS2SResourceService) syncIEAgAgRulesWithReader(ctx context.Context, writer ports.Writer, reader ports.Reader, rules []models.IEAgAgRule, syncOp models.SyncOp) error {
	log.Printf("🔍 PATH_B_ENTRY: syncIEAgAgRulesWithReader - Syncing %d IEAgAgRules with operation %s", len(rules), syncOp)
	log.Printf("🔍 PATH_B_CONDITION_STATUS: conditionManager nil? %v", s.conditionManager == nil)

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
				// Don't fail the whole operation if sgroups sync fails
			} else {
				log.Printf("syncIEAgAgRulesWithReader: Successfully batch synced %d IEAgAgRules to sgroups", len(syncableEntities))
			}
		}
	}

	// 🔍 PATH_B_CRITICAL: Check rule conditions BEFORE applying timing fix
	var rulesWithConditions, rulesWithoutConditions int
	for _, rule := range rules {
		if len(rule.Meta.Conditions) > 0 {
			rulesWithConditions++
		} else {
			rulesWithoutConditions++
		}
	}
	log.Printf("🔍 PATH_B_CONDITION_ANALYSIS: %d rules WITH conditions, %d rules WITHOUT conditions (BEFORE timing fix)", rulesWithConditions, rulesWithoutConditions)

	// 🚀 PATH_B_TIMING_FIX: Apply timing fix conditions processing BEFORE sync
	log.Printf("🚀 PATH_B_TIMING_FIX: Applying conditions-only timing fix to %d IEAgAgRules", len(rules))
	if err := s.applyTimingFixConditionsOnly(ctx, rules); err != nil {
		return errors.Wrap(err, "failed to apply conditions-only timing fix to IEAgAg rules in PATH_B")
	}

	// Sync to storage (now with conditions included)
	log.Printf("🚀 PATH_B_SYNC: Syncing %d IEAgAgRules with conditions included", len(rules))
	if err := writer.SyncIEAgAgRules(ctx, rules, ports.EmptyScope{}, ports.WithSyncOp(syncOp)); err != nil {
		return errors.Wrap(err, "failed to sync IEAgAgRules in storage")
	}

	log.Printf("✅ PATH_B_SUCCESS: Universal timing fix applied - IEAgAgRules now have proper conditions")

	// 🎯 POST_SYNC_CONDITION_FIX: Process conditions after PostgreSQL sync to ensure they're properly saved
	log.Printf("🔄 POST_SYNC_CONDITION_FIX: Processing conditions for %d IEAgAgRules after PostgreSQL sync", len(rules))
	if s.conditionManager != nil {
		for i := range rules {
			log.Printf("🔄 POST_SYNC_CONDITION_FIX: Processing conditions for IEAgAgRule %s/%s", rules[i].Namespace, rules[i].Name)
			if err := s.conditionManager.ProcessIEAgAgRuleConditions(ctx, &rules[i]); err != nil {
				log.Printf("❌ POST_SYNC_CONDITION_FIX: Failed to process IEAgAgRule conditions for %s/%s: %v", rules[i].Namespace, rules[i].Name, err)
				// Don't fail the operation if condition processing fails
			} else {
				log.Printf("✅ POST_SYNC_CONDITION_FIX: Successfully processed conditions for %s", rules[i].Key())
			}
		}
	} else {
		log.Printf("⚠️ POST_SYNC_CONDITION_FIX: conditionManager is NIL - no conditions will be processed for %d IEAgAgRules", len(rules))
	}

	log.Printf("syncIEAgAgRulesWithReader: Successfully synced %d IEAgAgRules", len(rules))
	return nil
}

// RuleS2SRegenerator interface implementation for reactive IEAgAg rule updates

// RegenerateIEAgAgRulesForService regenerates all IEAgAg rules that depend on a specific Service
// 🚀 OPTIMIZED: Now uses proven RecalculateIEAgAgRulesForAffectedRuleS2S system with complete external sync
func (s *RuleS2SResourceService) RegenerateIEAgAgRulesForService(ctx context.Context, serviceID models.ResourceIdentifier) error {
	log.Printf("🔄 RegenerateIEAgAgRulesForService: Starting service-specific regeneration for Service %s", serviceID.Key())

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

	log.Printf("🔍 RegenerateIEAgAgRulesForService: Service %s has %d AddressGroups", serviceID.Key(), len(changedService.AddressGroups))

	// Find ALL RuleS2S affected by this service change
	affectedRulesMap := make(map[string]models.RuleS2S)

	// 1. Find rules via direct ServiceAlias references
	serviceAliases, err := s.findServiceAliasesForService(ctx, reader, serviceID)
	if err != nil {
		return errors.Wrap(err, "failed to find ServiceAliases")
	}

	directRuleCount := 0
	for _, alias := range serviceAliases {
		rules, err := s.findRuleS2SReferencingServiceAlias(ctx, reader, alias.ResourceIdentifier)
		if err != nil {
			log.Printf("❌ Error finding rules for ServiceAlias %s: %v", alias.Key(), err)
			continue
		}
		for _, rule := range rules {
			if _, exists := affectedRulesMap[rule.Key()]; !exists {
				affectedRulesMap[rule.Key()] = rule
				directRuleCount++
				log.Printf("🎯 Found direct rule %s via ServiceAlias %s", rule.Key(), alias.Key())
			}
		}
	}

	// 2. Find rules via aggregation group relationships
	aggregationRuleCount := 0
	for _, ag := range changedService.AddressGroups {
		rules, err := s.findRuleS2SByAddressGroupInteraction(ctx, reader, ag)
		if err != nil {
			log.Printf("❌ Error finding rules for AddressGroup %s: %v", ag.Name, err)
			continue
		}
		for _, rule := range rules {
			if _, exists := affectedRulesMap[rule.Key()]; !exists {
				affectedRulesMap[rule.Key()] = rule
				aggregationRuleCount++
				log.Printf("🎯 Found aggregation rule %s via AddressGroup %s/%s", rule.Key(), ag.Namespace, ag.Name)
			}
		}
	}

	// Convert to slice
	var affectedRules []models.RuleS2S
	for _, rule := range affectedRulesMap {
		affectedRules = append(affectedRules, rule)
	}

	log.Printf("🔄 Found %d total affected RuleS2S for Service %s (direct: %d, aggregation: %d)",
		len(affectedRules), serviceID.Key(), directRuleCount, aggregationRuleCount)

	if len(affectedRules) == 0 {
		log.Printf("⚠️ No affected RuleS2S found for Service %s - no regeneration needed", serviceID.Key())
		return nil
	}

	// 🚀 USE PROVEN SYSTEM: Delegate to battle-tested recalculation with complete external sync
	reason := fmt.Sprintf("service %s changed (AddressGroups: %d)", serviceID.Key(), len(changedService.AddressGroups))
	log.Printf("🚀 Delegating to proven RecalculateIEAgAgRulesForAffectedRuleS2S system")
	return s.RecalculateIEAgAgRulesForAffectedRuleS2S(ctx, affectedRules, reason)
}

// RegenerateIEAgAgRulesForServiceAlias regenerates all IEAgAg rules that depend on a specific ServiceAlias
func (s *RuleS2SResourceService) RegenerateIEAgAgRulesForServiceAlias(ctx context.Context, serviceAliasID models.ResourceIdentifier) error {
	log.Printf("RegenerateIEAgAgRulesForServiceAlias: Starting regeneration for ServiceAlias %s", serviceAliasID.Key())

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

	log.Printf("RegenerateIEAgAgRulesForServiceAlias: Found %d affected RuleS2S for ServiceAlias %s", len(affectedRules), serviceAliasID.Key())

	// Regenerate IEAgAg rules for affected RuleS2S
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

// 🎯 NEW: NotifyServiceAddressGroupsChanged triggers RuleS2S condition recalculation when Service.AddressGroups changes
// This implements the reactive dependency chain from our controller analysis
func (s *RuleS2SResourceService) NotifyServiceAddressGroupsChanged(ctx context.Context, serviceID models.ResourceIdentifier) error {
	log.Printf("🔔 NOTIFICATION_START: Service %s AddressGroups changed → Finding affected RuleS2S for regeneration", serviceID.Key())
	log.Printf("🔔 NOTIFICATION_FLOW: Will use PATH_A (updateIEAgAgRulesForRuleS2SWithReader) with universal timing fix")

	// 🔍 DEBUG: Get current service state to log what changed
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Log current service state for debugging
	service, serviceErr := reader.GetServiceByID(ctx, serviceID)
	if serviceErr == nil {
		agRefs := make([]string, len(service.AddressGroups))
		for i, agRef := range service.AddressGroups {
			agRefs[i] = fmt.Sprintf("%s/%s", agRef.Namespace, agRef.Name)
		}
		log.Printf("🔍 SERVICE_CURRENT_STATE: Service %s currently has %d AddressGroups: [%s]",
			service.Key(), len(service.AddressGroups), strings.Join(agRefs, ", "))
	} else {
		log.Printf("❌ SERVICE_LOOKUP_ERROR: Failed to get current state of Service %s: %v", serviceID.Key(), serviceErr)
	}

	var affectedRules []models.RuleS2S

	// Find all RuleS2S that reference this Service (either directly or via ServiceAliases)
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		isAffected := false

		// Check direct Service references first
		if rule.ServiceLocalRef.Name == serviceID.Name && rule.ServiceLocalRef.Namespace == serviceID.Namespace {
			affectedRules = append(affectedRules, rule)
			log.Printf("🔔 NotifyServiceAddressGroupsChanged: Found affected RuleS2S %s via direct ServiceLocalRef", rule.Key())
			return nil
		}

		if rule.ServiceRef.Name == serviceID.Name && rule.ServiceRef.Namespace == serviceID.Namespace {
			affectedRules = append(affectedRules, rule)
			log.Printf("🔔 NotifyServiceAddressGroupsChanged: Found affected RuleS2S %s via direct ServiceRef", rule.Key())
			return nil
		}

		// Check ServiceAlias references - ServiceLocalRef
		if rule.ServiceLocalRef.Kind == "ServiceAlias" {
			serviceAlias, err := reader.GetServiceAliasByID(ctx, models.ResourceIdentifier{
				Name:      rule.ServiceLocalRef.Name,
				Namespace: rule.ServiceLocalRef.Namespace,
			})
			if err == nil && serviceAlias != nil {
				if serviceAlias.ServiceRef.Name == serviceID.Name && serviceAlias.ServiceRef.Namespace == serviceID.Namespace {
					affectedRules = append(affectedRules, rule)
					log.Printf("🔔 NotifyServiceAddressGroupsChanged: Found affected RuleS2S %s via ServiceAlias %s (ServiceLocalRef)", rule.Key(), serviceAlias.Key())
					isAffected = true
				}
			}
		}

		// Check ServiceAlias references - ServiceRef (only if not already marked as affected)
		if !isAffected && rule.ServiceRef.Kind == "ServiceAlias" {
			serviceAlias, err := reader.GetServiceAliasByID(ctx, models.ResourceIdentifier{
				Name:      rule.ServiceRef.Name,
				Namespace: rule.ServiceRef.Namespace,
			})
			if err == nil && serviceAlias != nil {
				if serviceAlias.ServiceRef.Name == serviceID.Name && serviceAlias.ServiceRef.Namespace == serviceID.Namespace {
					affectedRules = append(affectedRules, rule)
					log.Printf("🔔 NotifyServiceAddressGroupsChanged: Found affected RuleS2S %s via ServiceAlias %s (ServiceRef)", rule.Key(), serviceAlias.Key())
				}
			}
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

	log.Printf("📋 AFFECTED_RULES_FOUND: Found %d RuleS2S rules affected by Service %s change", len(affectedRules), serviceID.Key())
	for i, rule := range affectedRules {
		log.Printf("  📄 AFFECTED_RULE[%d]: %s (traffic: %s)", i, rule.Key(), rule.Traffic)
	}

	// 🎯 KEY: Regenerate IEAgAgRules for all affected RuleS2S
	// This is where the reactive chain completes: Service.AddressGroups change → IEAgAgRule regeneration → RuleS2S Ready=True
	log.Printf("🔄 IEAGAG_REGENERATION_START: Starting regeneration of IEAgAgRules for %d affected RuleS2S", len(affectedRules))

	// 🔍 DEBUG: Log existing IEAgAgRules count before regeneration
	existingRulesCount := 0
	reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
		existingRulesCount++
		return nil
	}, ports.EmptyScope{})
	log.Printf("📊 EXISTING_IEAGAG_COUNT: Currently %d IEAgAgRules in system before regeneration", existingRulesCount)

	// 🎯 CRITICAL FIX: Use RegenerateIEAgAgRulesForService instead of updateIEAgAgRulesForRuleS2SWithReader
	// This is the key fix for the 16→8→16 incremental deletion phenomenon
	// RegenerateIEAgAgRulesForService has the critical logic to detect when Services lose all AddressGroups
	// and trigger performServiceBasedIEAgAgRuleCleanup, while updateIEAgAgRulesForRuleS2SWithReader only
	// handles Cross-RuleS2S aggregation for Ready=True RuleS2S
	log.Printf("🎯 CRITICAL_FIX: Using RegenerateIEAgAgRulesForService instead of PATH_A to handle service-based cleanup")
	log.Printf("🔧 FIX_REASON: This enables service-based cleanup when Service loses all AddressGroups")

	err = s.RegenerateIEAgAgRulesForService(ctx, serviceID)
	if err != nil {
		log.Printf("❌ REGENERATION_ERROR: RegenerateIEAgAgRulesForService failed for service %s: %v", serviceID.Key(), err)
		return errors.Wrapf(err, "failed to regenerate IEAgAgRules for service %s", serviceID.Key())
	}

	log.Printf("✅ REGENERATION_SUCCESS: Successfully used RegenerateIEAgAgRulesForService with service-based cleanup support")

	// 🔍 DEBUG: Log new IEAgAgRules count after regeneration for comparison
	newRulesCount := 0
	reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
		newRulesCount++
		return nil
	}, ports.EmptyScope{})
	log.Printf("📊 REGENERATION_RESULTS: IEAgAgRules count changed from %d to %d (diff: %+d)",
		existingRulesCount, newRulesCount, newRulesCount-existingRulesCount)

	// 🎯 CRITICAL FIX: Re-evaluate conditions for all affected RuleS2S
	// This ensures RuleS2S are marked Ready=False if their dependencies are no longer satisfied
	log.Printf("🔍 CONDITION_REEVALUATION: Re-evaluating conditions for %d affected RuleS2S after service change", len(affectedRules))

	for _, rule := range affectedRules {
		if s.conditionManager != nil {
			if err := s.conditionManager.ProcessRuleS2SConditions(ctx, &rule); err != nil {
				log.Printf("⚠️ CONDITION_ERROR: Failed to re-evaluate conditions for RuleS2S %s: %v", rule.Key(), err)
				// Continue processing other rules
			} else {
				log.Printf("✅ CONDITION_UPDATED: Successfully re-evaluated conditions for RuleS2S %s", rule.Key())
			}
		} else {
			log.Printf("⚠️ NO_CONDITION_MANAGER: Cannot re-evaluate conditions for RuleS2S %s", rule.Key())
		}
	}

	log.Printf("✅ NOTIFICATION_COMPLETE: Completed service-aware IEAgAgRule regeneration for Service %s",
		serviceID.Key())
	log.Printf("🏁 REACTIVE_CHAIN_COMPLETE: Service %s → Service-based regeneration → RuleS2S condition re-evaluation",
		serviceID.Key())

	return nil
}

// regenerateIEAgAgRulesForRuleS2SList is a helper method to regenerate IEAgAg rules for a list of RuleS2S
func (s *RuleS2SResourceService) regenerateIEAgAgRulesForRuleS2SList(ctx context.Context, ruleS2SList []models.RuleS2S) error {
	if len(ruleS2SList) == 0 {
		log.Printf("🔄 REGENERATION_EMPTY: No RuleS2S provided for regeneration - skipping")
		return nil
	}

	log.Printf("🔄 REGENERATION_START: Starting IEAgAgRule regeneration for %d RuleS2S", len(ruleS2SList))
	for i, rule := range ruleS2SList {
		log.Printf("  📄 RULE_TO_REGENERATE[%d]: %s (traffic: %s)", i, rule.Key(), rule.Traffic)
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
		} else {
			log.Printf("✅ TRANSACTION_SUCCESS: Transaction completed successfully")
		}
	}()

	// Get reader from writer for transaction consistency
	log.Printf("💾 READER_FROM_WRITER: Creating reader from writer for transaction consistency")
	reader, err := s.registry.ReaderFromWriter(ctx, writer)
	if err != nil {
		log.Printf("❌ READER_ERROR: Failed to get reader from writer: %v", err)
		return errors.Wrap(err, "failed to get reader from writer")
	}
	defer reader.Close()

	// Use the existing updateIEAgAgRulesForRuleS2SWithReader logic from the old service
	// This will properly regenerate and sync IEAgAg rules
	log.Printf("🔄 CALLING_UPDATE_METHOD: Delegating to updateIEAgAgRulesForRuleS2SWithReader for %d RuleS2S", len(ruleS2SList))
	err = s.updateIEAgAgRulesForRuleS2SWithReader(ctx, writer, reader, ruleS2SList)
	if err != nil {
		log.Printf("❌ REGENERATION_ERROR: updateIEAgAgRulesForRuleS2SWithReader failed: %v", err)
		return err
	}

	log.Printf("✅ REGENERATION_SUCCESS: Successfully regenerated IEAgAgRules for %d RuleS2S", len(ruleS2SList))
	return nil
}

// updateIEAgAgRulesForRuleS2SWithReader regenerates IEAgAg rules for given RuleS2S (similar to old service logic)
func (s *RuleS2SResourceService) updateIEAgAgRulesForRuleS2SWithReader(ctx context.Context, writer ports.Writer, reader ports.Reader, rules []models.RuleS2S) error {
	if len(rules) == 0 {
		return nil
	}

	log.Printf("🔍 PATH_A_ENTRY: updateIEAgAgRulesForRuleS2SWithReader - Starting synchronized aggregation for %d RuleS2S", len(rules))
	log.Printf("🔍 PATH_A_CONDITION_STATUS: conditionManager nil? %v", s.conditionManager == nil)

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

	log.Printf("🔒 SYNC_AGGREGATION: Identified %d aggregation keys to synchronize", len(affectedKeys))

	// Acquire all locks in deterministic order to prevent deadlock
	keyStrs := make([]string, 0, len(affectedKeys))
	for keyStr := range affectedKeys {
		keyStrs = append(keyStrs, keyStr)
	}
	sort.Strings(keyStrs) // Sort for deterministic lock order

	// Acquire locks
	log.Printf("🔒 SYNC_AGGREGATION: Acquiring %d mutex locks in deterministic order", len(mutexes))
	for _, keyStr := range keyStrs {
		mutex := mutexes[keyStr]
		mutex.Lock()
		log.Printf("🔒 SYNC_AGGREGATION: Acquired lock for aggregation key: %s", keyStr)
	}

	// Ensure all locks are released
	defer func() {
		log.Printf("🔓 SYNC_AGGREGATION: Releasing %d mutex locks", len(mutexes))
		for _, keyStr := range keyStrs {
			mutex := mutexes[keyStr]
			mutex.Unlock()
			log.Printf("🔓 SYNC_AGGREGATION: Released lock for aggregation key: %s", keyStr)
		}
	}()

	log.Printf("updateIEAgAgRulesForRuleS2SWithReader: Updating IEAgAg rules for %d RuleS2S (SYNCHRONIZED)", len(rules))

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

	log.Printf("updateIEAgAgRulesForRuleS2SWithReader: Found %d affected aggregation groups", len(affectedGroups))

	// 🔧 CRITICAL MASS DELETION FIX: Only get existing IEAgAg rules that are ACTUALLY related to the affected aggregation groups
	// The old code used EmptyScope{} which got ALL rules from the entire system, causing mass deletion!
	// Now we build a targeted scope based on the affected groups to prevent cross-service rule deletion
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

	log.Printf("🔧 MASS_DELETION_FIX: Targeting %d namespaces, %d local AGs, %d target AGs for existing rule lookup",
		len(affectedNamespaces), len(affectedLocalAGs), len(affectedTargetAGs))

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

	log.Printf("🔧 MASS_DELETION_FIX: Found %d existing IEAgAg rules TARGETED to affected aggregation groups (not ALL system rules)", len(existingRules))

	// PHASE 2: For each affected group, find ALL RuleS2S that contribute (proper aggregation per controller reference)
	// This is CORRECT behavior - aggregation requires finding all contributors, not just input rules
	var allContributingRules []models.RuleS2S
	ruleKeysProcessed := make(map[string]bool)

	for _, group := range affectedGroups {
		log.Printf("🔍 AGGREGATION: Finding all RuleS2S for aggregation group %s", group.Key())
		groupRules, err := s.FindAllRuleS2SForAggregationGroup(ctx, reader, group)
		if err != nil {
			log.Printf("❌ Error finding rules for aggregation group %s: %v", group.Key(), err)
			continue
		}

		log.Printf("🔍 AGGREGATION: Found %d RuleS2S contributing to group %s", len(groupRules), group.Key())

		for _, rule := range groupRules {
			if !ruleKeysProcessed[rule.Key()] {
				allContributingRules = append(allContributingRules, rule)
				ruleKeysProcessed[rule.Key()] = true
				log.Printf("🔍 AGGREGATION: Added contributing RuleS2S %s to aggregation set", rule.Key())
			}
		}
	}

	log.Printf("updateIEAgAgRulesForRuleS2SWithReader: Found %d total contributing RuleS2S (including related ones)", len(allContributingRules))

	// PHASE 3: Use aggregated generation on the complete set for proper port aggregation
	expectedRulesSet, allNewRules, err := s.generateAggregatedIEAgAgRules(ctx, reader, allContributingRules)
	if err != nil {
		return errors.Wrap(err, "failed to generate aggregated IEAgAg rules")
	}

	log.Printf("updateIEAgAgRulesForRuleS2SWithReader: Generated total %d aggregated IEAgAg rules", len(allNewRules))

	// Sync new/updated IEAgAg rules
	if len(allNewRules) > 0 {
		if err = s.syncIEAgAgRulesWithReader(ctx, writer, reader, allNewRules, models.SyncOpUpsert); err != nil {
			return errors.Wrap(err, "failed to sync new IEAgAg rules")
		}
	}

	// 🔧 MASS_DELETION_FIX: Delete obsolete IEAgAg rules (now SAFELY scoped to affected groups only)
	var obsoleteRules []models.IEAgAgRule
	for existingKey, existingRule := range existingRules {
		if !expectedRulesSet[existingKey] {
			// This rule is no longer needed within the affected aggregation groups
			obsoleteRules = append(obsoleteRules, existingRule)
			log.Printf("🔧 MASS_DELETION_FIX: Marking rule as obsolete: %s (was in affected groups but not in new expected set)", existingKey)
		}
	}

	if len(obsoleteRules) > 0 {
		log.Printf("🔧 MASS_DELETION_FIX: Safely deleting %d obsolete IEAgAg rules (ONLY from affected aggregation groups)", len(obsoleteRules))

		// 🚨 SAFETY CHECK: Prevent accidental mass deletion
		totalSystemRuleCount := 0
		reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
			totalSystemRuleCount++
			return nil
		}, ports.EmptyScope{})

		deletionRatio := float64(len(obsoleteRules)) / float64(totalSystemRuleCount)
		if deletionRatio > 0.8 && totalSystemRuleCount > 10 {
			log.Printf("🚨 SAFETY_CHECK_TRIGGERED: Refusing to delete %d rules (%.1f%% of %d total system rules) - likely a bug!",
				len(obsoleteRules), deletionRatio*100, totalSystemRuleCount)
			return errors.Errorf("safety check: refusing to delete %d IEAgAg rules (%.1f%% of total %d rules) - likely mass deletion bug",
				len(obsoleteRules), deletionRatio*100, totalSystemRuleCount)
		}

		log.Printf("🔧 SAFETY_CHECK_PASSED: Deleting %d rules (%.1f%% of %d total) - within safe limits",
			len(obsoleteRules), deletionRatio*100, totalSystemRuleCount)

		// Extract IDs for deletion
		var obsoleteRuleIDs []models.ResourceIdentifier
		for _, rule := range obsoleteRules {
			obsoleteRuleIDs = append(obsoleteRuleIDs, rule.ResourceIdentifier)
		}

		if err = writer.DeleteIEAgAgRulesByIDs(ctx, obsoleteRuleIDs); err != nil {
			return errors.Wrap(err, "failed to delete obsolete IEAgAg rules")
		}
	}

	// 🚀 UNIVERSAL TIMING FIX: Apply timing fix to eliminate race condition
	// This ensures IEAgAgRules are created with proper conditions from the start
	klog.Infof("🔍 PATH_A_TIMING_FIX: Applying universal timing fix to %d IEAgAgRules", len(allNewRules))
	if err := s.applyTimingFixToIEAgAgRules(ctx, writer, allNewRules); err != nil {
		return errors.Wrap(err, "failed to apply universal timing fix to IEAgAg rules")
	}

	// Commit transaction (now includes rules WITH conditions)
	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit IEAgAg rule updates")
	}

	log.Printf("updateIEAgAgRulesForRuleS2SWithReader: Successfully updated IEAgAg rules - created/updated: %d, deleted: %d", len(allNewRules), len(obsoleteRules))

	// 🎯 CRITICAL: Recalculate RuleS2S conditions for ALL contributing rules after IEAgAgRule regeneration
	// This ensures RuleS2S Ready conditions reflect the actual IEAgAgRule state
	// TIMING FIX: Use allContributingRules instead of just input rules to fix timing race
	if s.conditionManager != nil {
		log.Printf("🔧 TIMING FIX: Recalculating conditions for %d total contributing RuleS2S after IEAgAgRule regeneration and commit", len(allContributingRules))
		for i := range allContributingRules {
			log.Printf("🔧 TIMING FIX: [%d/%d] Recalculating conditions for RuleS2S %s", i+1, len(allContributingRules), allContributingRules[i].Key())
			if err := s.conditionManager.ProcessRuleS2SConditions(ctx, &allContributingRules[i]); err != nil {
				log.Printf("❌ TIMING FIX: Failed to recalculate conditions for RuleS2S %s after IEAgAgRule regeneration: %v", allContributingRules[i].Key(), err)
				// Don't fail the entire operation if condition processing fails
			} else {
				log.Printf("✅ TIMING FIX: Successfully recalculated conditions for RuleS2S %s after IEAgAgRule regeneration", allContributingRules[i].Key())
			}
		}
		log.Printf("🎉 TIMING FIX: Completed condition recalculation for %d RuleS2S after IEAgAgRule aggregation", len(allContributingRules))
	}

	return nil
}

// generateAggregatedIEAgAgRules generates aggregated IEAgAgRules from multiple RuleS2S
// 🎯 CROSS-RULES2S AGGREGATION ENGINE (Phase 1 Implementation) - COMPLETE REWRITE
// This replaces the old per-RuleS2S approach with proper cross-RuleS2S aggregation
func (s *RuleS2SResourceService) generateAggregatedIEAgAgRules(ctx context.Context, reader ports.Reader, rules []models.RuleS2S, excludeRuleIDs ...models.ResourceIdentifier) (map[string]bool, []models.IEAgAgRule, error) {
	klog.Infof("🎯 CROSS_AGGREGATION_ENGINE: Generating aggregated rules from %d RuleS2S using cross-rule aggregation (excluding %d rules)", len(rules), len(excludeRuleIDs))

	// Create exclusion map for fast lookup
	excludeMap := make(map[string]bool)
	for _, id := range excludeRuleIDs {
		excludeMap[id.Key()] = true
		klog.Infof("🚫 EXCLUSION_FILTER: Excluding RuleS2S %s from aggregation", id.Key())
	}

	// Phase 1: Process unique AG combinations across ALL RuleS2S, not per individual rule
	type ruleGroupMetadata struct {
		traffic   models.Traffic
		localAG   models.AddressGroupRef
		targetAG  models.AddressGroupRef
		protocol  models.TransportProtocol
		namespace string
	}

	expectedRules := make(map[string]bool)
	var newRules []models.IEAgAgRule
	processedCombinations := make(map[string]bool) // Track processed AG+Protocol combinations

	// Phase 2: For each rule, find all contributing RuleS2S and aggregate
	for i, currentRule := range rules {
		klog.Infof("🔍 CROSS_AGGREGATION[%d/%d]: Processing rule %s as aggregation anchor", i+1, len(rules), currentRule.Key())

		// 🚫 EXCLUSION_FILTER: Skip rules being deleted to prevent circular dependency
		if excludeMap[currentRule.ResourceIdentifier.Key()] {
			klog.Infof("🚫 EXCLUSION_FILTER: Skipping excluded rule %s (being deleted)", currentRule.Key())
			continue
		}

		// 🎯 CRITICAL FIX: Skip Ready=False RuleS2S from contributing to aggregated IEAgAgRules
		if !currentRule.Meta.IsReady() {
			klog.Infof("🚫 READY_FILTER: Skipping inactive (Ready=False) RuleS2S %s from aggregation", currentRule.Key())
			continue
		}

		// Get services for current rule (using same reader session for consistency)
		localService, targetService, err := s.getServicesForRuleWithReader(ctx, reader, &currentRule)
		if err != nil {
			klog.Errorf("❌ CROSS_AGGREGATION: Failed to get services for rule %s: %v", currentRule.Key(), err)
			continue
		}

		// Generate IEAgAg rules for each AG combination with cross-RuleS2S aggregation
		for _, localAG := range localService.AddressGroups {
			for _, targetAG := range targetService.AddressGroups {
				// 🚀 PROTOCOL FIX: Only process protocols that have actual ports in services
				// Determine which protocols have ports based on traffic direction
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
						s.addressGroupRefKey(localAG),
						s.addressGroupRefKey(targetAG),
						protocol)

					// Skip if we already processed this combination
					if processedCombinations[combinationKey] {
						klog.V(2).Infof("  ⏭️ CROSS_AGGREGATION: Skipping already processed combination %s", combinationKey)
						continue
					}
					processedCombinations[combinationKey] = true

					klog.Infof("  🔑 CROSS_AGGREGATION: Processing new combination %s", combinationKey)

					// 🚀 CRITICAL: Find ALL RuleS2S that contribute to this AG+Protocol combination
					contributingRules, err := s.findContributingRuleS2S(ctx, &currentRule, localService, targetService, excludeMap)
					if err != nil {
						klog.Errorf("    ❌ CROSS_AGGREGATION: Failed to find contributing rules for %s: %v", combinationKey, err)
						continue
					}

					klog.Infof("    📊 CROSS_AGGREGATION: Found %d contributing RuleS2S for combination %s", len(contributingRules), combinationKey)

					// 🎯 CRITICAL: Aggregate ports from ALL contributing RuleS2S for this protocol
					aggregatedPorts := s.aggregatePortsWithProtocol(ctx, reader, contributingRules, protocol)
					if len(aggregatedPorts) == 0 {
						klog.Infof("    🧹 CROSS_AGGREGATION: No ports for protocol %s in combination %s - checking for cleanup", protocol, combinationKey)

						// 🧹 CLEANUP: Apply reference controller logic - delete existing rule if it exists
						// This implements the missing cleanup functionality from reference lines 892-925
						ruleName := s.generateRuleName(string(currentRule.Traffic), localAG.Name, targetAG.Name, string(protocol))
						err := s.cleanupOrphanedIEAgAgRule(ctx, reader, ruleName, currentRule.Namespace, combinationKey)
						if err != nil {
							klog.Errorf("    ❌ CROSS_AGGREGATION: Failed to cleanup orphaned rule %s: %v", ruleName, err)
							// Continue processing other combinations even if cleanup fails
						}
						continue
					}

					klog.Infof("    🎉 CROSS_AGGREGATION: Aggregated %d unique ports for %s: %s",
						len(aggregatedPorts), combinationKey, strings.Join(aggregatedPorts, ","))

					// Generate single aggregated IEAgAg rule
					ruleName := s.generateRuleName(string(currentRule.Traffic), localAG.Name, targetAG.Name, string(protocol))

					// 🚀 CRITICAL FIX: Use correct namespace logic from reference implementation
					// Reference: k8s-controller uses AddressGroup namespace, not RuleS2S namespace
					var ruleNamespace string
					if currentRule.Traffic == models.INGRESS {
						// For ingress, rule goes in the local AG namespace (receiver)
						ruleNamespace = localAG.Namespace
						klog.Infof("    🏠 NAMESPACE_LOGIC: INGRESS rule → using localAG namespace: %s", ruleNamespace)
					} else {
						// For egress, rule goes in the target AG namespace (receiver)
						ruleNamespace = targetAG.Namespace
						klog.Infof("    🏠 NAMESPACE_LOGIC: EGRESS rule → using targetAG namespace: %s", ruleNamespace)
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
						AddressGroupLocal: localAG,
						AddressGroup:      targetAG,
						Ports: []models.PortSpec{
							{
								Destination: strings.Join(aggregatedPorts, ","), // Single aggregated port string
							},
						},
						Action:   models.ActionAccept,
						Logs:     true,
						Trace:    false,
						Priority: int32(100),
					}

					expectedRules[ieRule.Key()] = true
					newRules = append(newRules, ieRule)

					klog.Infof("    ✅ CROSS_AGGREGATION: Created aggregated IEAgAg rule %s with ports from %d contributing RuleS2S: %s",
						ieRule.Key(), len(contributingRules), strings.Join(aggregatedPorts, ","))
				}
			}
		}
	}

	klog.Infof("🎯 CROSS_AGGREGATION_ENGINE: Generated %d aggregated IEAgAg rules (vs %d input RuleS2S)", len(newRules), len(rules))
	klog.Infof("  📋 CROSS_AGGREGATION_SUMMARY: Expected rules keys: %v", func() []string {
		keys := make([]string, 0, len(expectedRules))
		for key := range expectedRules {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return keys
	}())

	return expectedRules, newRules, nil
}

// Helper methods

// cleanupOrphanedIEAgAgRule deletes an existing IEAgAg rule when aggregation results in empty ports
// This implements the reference controller cleanup logic from lines 892-925
func (s *RuleS2SResourceService) cleanupOrphanedIEAgAgRule(ctx context.Context, reader ports.Reader, ruleName, namespace string, combinationKey string) error {
	klog.Infof("🧹 CLEANUP: Checking for orphaned IEAgAg rule %s/%s (combination: %s)", namespace, ruleName, combinationKey)

	// Check if rule exists
	ruleID := models.ResourceIdentifier{
		Name:      ruleName,
		Namespace: namespace,
	}

	existingRule, err := reader.GetIEAgAgRuleByID(ctx, ruleID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			klog.Infof("  ✅ CLEANUP: No existing rule %s/%s to clean up", namespace, ruleName)
			return nil // Rule doesn't exist, nothing to clean up
		}
		klog.Errorf("  ❌ CLEANUP: Error checking if rule %s/%s exists: %v", namespace, ruleName, err)
		return errors.Wrapf(err, "failed to check if rule %s/%s exists", namespace, ruleName)
	}

	// Rule exists and needs to be deleted
	klog.Infof("  🗑️ CLEANUP: Deleting orphaned IEAgAg rule %s/%s (no contributing ports after aggregation)", namespace, ruleName)

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

	// Delete the orphaned rule
	err = writer.DeleteIEAgAgRulesByIDs(ctx, []models.ResourceIdentifier{ruleID})
	if err != nil {
		klog.Errorf("  ❌ CLEANUP: Failed to delete orphaned rule %s/%s: %v", namespace, ruleName, err)
		return errors.Wrapf(err, "failed to delete orphaned rule %s/%s", namespace, ruleName)
	}

	// Commit the deletion
	if err = writer.Commit(); err != nil {
		klog.Errorf("  ❌ CLEANUP: Failed to commit deletion of rule %s/%s: %v", namespace, ruleName, err)
		return errors.Wrapf(err, "failed to commit deletion of rule %s/%s", namespace, ruleName)
	}

	klog.Infof("  ✅ CLEANUP: Successfully deleted orphaned IEAgAg rule %s/%s", namespace, ruleName)

	// Sync deletion to external systems (like SGroups)
	if s.syncManager != nil {
		klog.Infof("  🔄 CLEANUP: Syncing deletion of orphaned rule %s/%s to external systems", namespace, ruleName)
		err = s.syncManager.SyncEntity(ctx, existingRule, types.SyncOperationDelete)
		if err != nil {
			klog.Errorf("  ⚠️ CLEANUP: Failed to sync deletion to external systems for %s/%s: %v", namespace, ruleName, err)
			// Don't fail the cleanup for sync errors, just log them
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
	klog.Infof("🔄 UNIVERSAL_RECALC: Starting universal IEAgAg rule recalculation (reason: %s)", reason)

	startTime := time.Now()

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

	klog.Infof("  📊 UNIVERSAL_RECALC: Found %d existing IEAgAg rules to evaluate", len(existingRules))

	// Phase 2: Get ALL RuleS2S for recalculation
	var allRuleS2S []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		allRuleS2S = append(allRuleS2S, rule)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		return errors.Wrap(err, "failed to list all RuleS2S")
	}

	klog.Infof("  📋 UNIVERSAL_RECALC: Found %d total RuleS2S for aggregation calculations", len(allRuleS2S))

	// Phase 3: Generate fresh aggregated rules using existing cross-RuleS2S engine
	// Pass ALL RuleS2S to the aggregation engine for proper cross-rule aggregation
	_, freshRules, err := s.generateAggregatedIEAgAgRules(ctx, reader, allRuleS2S)
	if err != nil {
		return errors.Wrap(err, "failed to generate fresh aggregated IEAgAg rules")
	}

	klog.Infof("  🆕 UNIVERSAL_RECALC: Generated %d fresh aggregated rules", len(freshRules))

	// Phase 4: Compare existing vs fresh rules and determine operations
	operations := s.calculateRuleOperations(existingRules, freshRules)
	klog.Infof("  📈 UNIVERSAL_RECALC: Operations needed - Create: %d, Update: %d, Delete: %d",
		len(operations.toCreate), len(operations.toUpdate), len(operations.toDelete))

	// Phase 5: Execute operations with proper external sync
	if err := s.executeRuleOperations(ctx, operations, reason); err != nil {
		return errors.Wrapf(err, "failed to execute rule operations for reason: %s", reason)
	}

	duration := time.Since(startTime)
	klog.Infof("✅ UNIVERSAL_RECALC: Completed universal recalculation in %v (reason: %s)", duration, reason)

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

	klog.Infof("🔄 SCOPED_RECALC: Starting scoped IEAgAg rule recalculation for %d affected RuleS2S (reason: %s)", len(affectedRules), reason)

	startTime := time.Now()

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for scoped recalculation")
	}
	defer reader.Close()

	// Log affected RuleS2S for debugging
	for i, rule := range affectedRules {
		klog.Infof("  📄 SCOPED_RECALC: Affected RuleS2S[%d]: %s (traffic: %s)", i, rule.Key(), rule.Traffic)
	}

	// Phase 1: Get existing IEAgAg rules that COULD be affected by the RuleS2S changes
	// 🎯 CRITICAL FIX: Instead of relying on explicit references (which may not exist),
	// find existing IEAgAgRules based on the services involved in affected RuleS2S
	var existingRules []models.IEAgAgRule
	affectedServices := make(map[string]bool)

	// Collect all services involved in affected RuleS2S
	for _, rule := range affectedRules {
		// Get the services for this RuleS2S to find potentially affected IEAgAgRules
		localService, targetService, err := s.getServicesForRuleWithReader(ctx, reader, &rule)
		if err != nil {
			klog.Errorf("  ⚠️ SCOPED_RECALC: Failed to get services for affected RuleS2S %s: %v", rule.Key(), err)
			continue
		}

		// Track all services involved (for finding related IEAgAgRules)
		affectedServices[localService.Key()] = true
		affectedServices[targetService.Key()] = true

		klog.V(4).Infof("  📋 SCOPED_RECALC: Affected RuleS2S %s involves services %s and %s",
			rule.Key(), localService.Key(), targetService.Key())
	}

	// Find existing IEAgAgRules that involve any of the affected services
	// This captures rules that might need to be updated or deleted based on service changes
	err = reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
		// Check if this IEAgAgRule involves any affected service (via AddressGroups)
		for serviceKey := range affectedServices {
			serviceNamespace := strings.Split(serviceKey, "/")[0]
			if rule.AddressGroupLocal.Namespace == serviceNamespace ||
				rule.AddressGroup.Namespace == serviceNamespace {
				existingRules = append(existingRules, rule)
				klog.V(4).Infof("  📊 SCOPED_RECALC: Including existing IEAgAg rule %s (involves affected service namespace %s)", rule.Key(), serviceNamespace)
				break
			}
		}
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		return errors.Wrap(err, "failed to list existing IEAgAg rules for affected services")
	}

	klog.Infof("  📊 SCOPED_RECALC: Found %d existing IEAgAg rules that could be affected by %d RuleS2S changes", len(existingRules), len(affectedRules))

	// Phase 2: Get ALL RuleS2S for cross-aggregation (still need all for proper aggregation)
	var allRuleS2S []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		allRuleS2S = append(allRuleS2S, rule)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		return errors.Wrap(err, "failed to list all RuleS2S for cross-aggregation")
	}

	klog.Infof("  📋 SCOPED_RECALC: Found %d total RuleS2S for aggregation calculations (cross-rule aggregation)", len(allRuleS2S))

	// Phase 3: Generate fresh aggregated rules using existing cross-RuleS2S engine
	// We still need ALL RuleS2S for proper cross-rule aggregation accuracy
	_, freshRules, err := s.generateAggregatedIEAgAgRules(ctx, reader, allRuleS2S)
	if err != nil {
		return errors.Wrap(err, "failed to generate fresh aggregated IEAgAg rules for scoped recalculation")
	}

	klog.Infof("  🆕 SCOPED_RECALC: Generated %d fresh aggregated rules from cross-RuleS2S engine", len(freshRules))

	// Phase 4: Use all fresh rules for comparison (efficiency comes from scoped existing rules)
	// The comparison logic will only create/update/delete rules that intersect with our scoped existing rules
	// This approach maintains correctness while gaining the key efficiency from scoped existing rules
	klog.Infof("  🎯 SCOPED_RECALC: Using all %d fresh rules for comparison (scoping efficiency comes from existing rules)", len(freshRules))

	// Phase 5: Compare scoped existing vs fresh rules and determine operations
	operations := s.calculateRuleOperations(existingRules, freshRules)
	klog.Infof("  📈 SCOPED_RECALC: Operations needed - Create: %d, Update: %d, Delete: %d",
		len(operations.toCreate), len(operations.toUpdate), len(operations.toDelete))

	// Phase 6: Execute operations with proper external sync
	if err := s.executeRuleOperations(ctx, operations, reason); err != nil {
		return errors.Wrapf(err, "failed to execute scoped rule operations for reason: %s", reason)
	}

	duration := time.Since(startTime)
	klog.Infof("✅ SCOPED_RECALC: Completed scoped recalculation in %v for %d affected RuleS2S (reason: %s)",
		duration, len(affectedRules), reason)

	return nil
}

// RecalculateTargetedIEAgAgRules provides targeted recalculation for specific IEAgAgRules
// 🎯 CRITICAL FIX: This method is designed for RuleS2S deletion scenarios where we need to
// clean up ONLY the specific IEAgAgRules that were referenced by deleted RuleS2S,
// preventing the massive external DELETE bug
func (s *RuleS2SResourceService) RecalculateTargetedIEAgAgRules(ctx context.Context, targetedIEAgAgRuleIDs []models.ResourceIdentifier, reason string) error {
	klog.Infof("🎯 TARGETED_RECALC: *** ENTRY POINT *** Starting targeted recalculation for %d IEAgAgRules (reason: %s)",
		len(targetedIEAgAgRuleIDs), reason)

	if len(targetedIEAgAgRuleIDs) == 0 {
		klog.Infof("🎯 TARGETED_RECALC: No targeted IEAgAgRules provided for recalculation - skipping (reason: %s)", reason)
		return nil
	}

	klog.Infof("🎯 TARGETED_RECALC: Starting targeted IEAgAg rule recalculation for %d specific rules (reason: %s)", len(targetedIEAgAgRuleIDs), reason)

	startTime := time.Now()

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader for targeted recalculation")
	}
	defer reader.Close()

	// Phase 1: Get ONLY the specific existing IEAgAg rules that were referenced by deleted RuleS2S
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
		klog.Infof("  📊 TARGETED_RECALC: Including existing IEAgAg rule %s for evaluation", ruleID.Key())
	}

	klog.Infof("  📊 TARGETED_RECALC: Found %d existing targeted IEAgAg rules to evaluate", len(existingTargetedRules))

	// Phase 2: Get ALL remaining RuleS2S for fresh calculations (excludes deleted ones)
	var allRemainingRuleS2S []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		allRemainingRuleS2S = append(allRemainingRuleS2S, rule)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		return errors.Wrap(err, "failed to list remaining RuleS2S")
	}

	klog.Infof("  📋 TARGETED_RECALC: Found %d remaining RuleS2S for fresh calculations", len(allRemainingRuleS2S))

	// Phase 3: Generate fresh aggregated rules using remaining RuleS2S
	_, allFreshRules, err := s.generateAggregatedIEAgAgRules(ctx, reader, allRemainingRuleS2S)
	if err != nil {
		return errors.Wrap(err, "failed to generate fresh aggregated IEAgAg rules for targeted recalculation")
	}

	klog.Infof("  🆕 TARGETED_RECALC: Generated %d fresh aggregated rules from remaining RuleS2S", len(allFreshRules))

	// Phase 4: Filter fresh rules to only include those that match our targeted rule patterns
	// We need to check which of the fresh rules correspond to the same 4-tuple patterns as our targeted rules
	freshTargetedRules := make([]models.IEAgAgRule, 0)
	targetedPatterns := make(map[string]bool)

	// Create pattern map from existing targeted rules
	for _, existingRule := range existingTargetedRules {
		pattern := fmt.Sprintf("%s:%s:%s:%s",
			existingRule.Traffic,
			existingRule.AddressGroupLocal.Name,
			existingRule.AddressGroup.Name,
			existingRule.Transport)
		targetedPatterns[pattern] = true
	}

	// Filter fresh rules to only those matching targeted patterns
	for _, freshRule := range allFreshRules {
		pattern := fmt.Sprintf("%s:%s:%s:%s",
			freshRule.Traffic,
			freshRule.AddressGroupLocal.Name,
			freshRule.AddressGroup.Name,
			freshRule.Transport)
		if targetedPatterns[pattern] {
			freshTargetedRules = append(freshTargetedRules, freshRule)
			klog.Infof("  🎯 TARGETED_RECALC: Fresh rule %s matches targeted pattern %s", freshRule.Key(), pattern)
		}
	}

	klog.Infof("  🎯 TARGETED_RECALC: Filtered to %d fresh rules matching targeted patterns", len(freshTargetedRules))

	// Phase 5: Compare existing targeted vs fresh targeted rules and determine operations
	operations := s.calculateRuleOperations(existingTargetedRules, freshTargetedRules)
	klog.Infof("  📈 TARGETED_RECALC: Operations needed - Create: %d, Update: %d, Delete: %d",
		len(operations.toCreate), len(operations.toUpdate), len(operations.toDelete))

	// Phase 6: Execute operations with proper external sync
	if err := s.executeRuleOperations(ctx, operations, reason); err != nil {
		return errors.Wrapf(err, "failed to execute targeted rule operations for reason: %s", reason)
	}

	duration := time.Since(startTime)
	klog.Infof("✅ TARGETED_RECALC: Completed targeted recalculation in %v for %d specific IEAgAgRules (reason: %s)",
		duration, len(targetedIEAgAgRuleIDs), reason)

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
			// Rule exists - check if update needed
			if s.needsUpdate(existingRule, freshRule) {
				klog.Infof("    🔄 UNIVERSAL_RECALC: Rule %s needs UPDATE (ports changed)", key)
				operations.toUpdate = append(operations.toUpdate, *freshRule)
			} else {
				klog.V(2).Infof("    ✅ UNIVERSAL_RECALC: Rule %s unchanged", key)
			}
		} else {
			// Rule doesn't exist - create it
			klog.Infof("    🆕 UNIVERSAL_RECALC: Rule %s needs CREATE", key)
			operations.toCreate = append(operations.toCreate, *freshRule)
		}
	}

	// Find rules to delete (exist but not in fresh calculations)
	for key, existingRule := range existingMap {
		if _, exists := freshMap[key]; !exists {
			klog.Infof("    🗑️ UNIVERSAL_RECALC: Rule %s needs DELETE (orphaned)", key)
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

		// 🔄 FIXED EXTERNAL_SYNC: Always sync deletions to external systems
		// Previous logic that skipped external sync was causing orphaned rules in SGROUP
		// The real fix is to ensure deletions are properly targeted, not to skip external sync
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
		// 🎯 CRITICAL_CONDITION_FIX: Process conditions for newly created/updated IEAgAgRules BEFORE PostgreSQL sync
		klog.Infof("  🔄 UNIVERSAL_RECALC_CONDITIONS: Processing conditions for %d IEAgAgRules before PostgreSQL sync", len(allChanges))
		if s.conditionManager != nil {
			for i := range allChanges {
				klog.Infof("  🔄 UNIVERSAL_RECALC_CONDITIONS: Processing conditions for IEAgAgRule %s/%s", allChanges[i].Namespace, allChanges[i].Name)
				// Use processIEAgAgRuleConditionsInMemory instead of full ProcessIEAgAgRuleConditions to avoid double-save
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

		klog.Infof("  📝 UNIVERSAL_RECALC: Creating/updating %d rules with conditions included", len(allChanges))
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

	klog.Infof("  ✅ UNIVERSAL_RECALC: Successfully executed all operations (reason: %s)", reason)
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
	log.Printf("🔄 triggerPostCreationIEAgAgRuleGeneration: Starting regeneration check for RuleS2S %s", rule.Key())

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	// Get the services referenced by this RuleS2S to check their AddressGroup status
	localServiceAliasID := models.ResourceIdentifier{
		Name:      rule.ServiceLocalRef.Name,
		Namespace: rule.ServiceLocalRef.Namespace,
	}
	targetServiceAliasID := models.ResourceIdentifier{
		Name:      rule.ServiceRef.Name,
		Namespace: rule.ServiceRef.Namespace,
	}

	// Get service aliases and their referenced services
	localServiceAlias, err := reader.GetServiceAliasByID(ctx, localServiceAliasID)
	if err != nil {
		log.Printf("⚠️ triggerPostCreationIEAgAgRuleGeneration: Cannot get local service alias %s: %v", localServiceAliasID.Key(), err)
		return nil // Don't fail the creation
	}

	targetServiceAlias, err := reader.GetServiceAliasByID(ctx, targetServiceAliasID)
	if err != nil {
		log.Printf("⚠️ triggerPostCreationIEAgAgRuleGeneration: Cannot get target service alias %s: %v", targetServiceAliasID.Key(), err)
		return nil // Don't fail the creation
	}

	localServiceID := models.ResourceIdentifier{
		Name:      localServiceAlias.ServiceRef.Name,
		Namespace: localServiceAlias.Namespace,
	}
	targetServiceID := models.ResourceIdentifier{
		Name:      targetServiceAlias.ServiceRef.Name,
		Namespace: targetServiceAlias.Namespace,
	}

	// Get the actual services and check their AddressGroup status
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
		len(localService.AddressGroups), len(targetService.AddressGroups))

	// Check if both services have AddressGroups - if yes, we should have generated IEAgAgRules
	if len(localService.AddressGroups) > 0 && len(targetService.AddressGroups) > 0 {
		log.Printf("✅ triggerPostCreationIEAgAgRuleGeneration: Both services have AddressGroups, IEAgAgRules should exist")

		// 🎯 CRITICAL FIX: Trigger Cross-RuleS2S aggregation via notification system
		// This avoids transaction conflicts and ensures new rule is included in aggregation
		log.Printf("🔄 triggerPostCreationIEAgAgRuleGeneration: Triggering Cross-RuleS2S aggregation via notification system")

		// Use the notification system to trigger Cross-RuleS2S aggregation
		// This will find ALL RuleS2S that affect the same services and trigger proper aggregation
		log.Printf("🔄 triggerPostCreationIEAgAgRuleGeneration: Notifying about service changes to trigger aggregation")

		// Notify about both services to ensure complete aggregation
		if err := s.NotifyServiceAddressGroupsChanged(ctx, localServiceID); err != nil {
			log.Printf("⚠️ triggerPostCreationIEAgAgRuleGeneration: Failed to notify about local service %s: %v", localServiceID.Key(), err)
		}

		if err := s.NotifyServiceAddressGroupsChanged(ctx, targetServiceID); err != nil {
			log.Printf("⚠️ triggerPostCreationIEAgAgRuleGeneration: Failed to notify about target service %s: %v", targetServiceID.Key(), err)
		}

		log.Printf("✅ triggerPostCreationIEAgAgRuleGeneration: Successfully triggered Cross-RuleS2S aggregation via notifications")
	} else {
		log.Printf("⚠️ triggerPostCreationIEAgAgRuleGeneration: Services don't have sufficient AddressGroups - Local: %d, Target: %d",
			len(localService.AddressGroups), len(targetService.AddressGroups))
	}

	return nil
}

func (s *RuleS2SResourceService) extractPortsFromService(service models.Service) []models.IngressPort {
	return service.IngressPorts
}

// 🎯 CROSS-RULES2S AGGREGATION METHODS (Phase 1 Implementation)
// Based on reference: netguard-k8s-controller/internal/controller/rules2s_controller.go

// findContributingRuleS2S finds all RuleS2S that contribute to the same IEAgAg rule aggregation
// Based on reference lines 603-654
func (s *RuleS2SResourceService) findContributingRuleS2S(
	ctx context.Context,
	currentRule *models.RuleS2S,
	localService *models.Service,
	targetService *models.Service,
	excludeMap map[string]bool,
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

	klog.Infof("  📊 CROSS_AGGREGATION: Found %d total RuleS2S in system for comparison", len(allRules))

	var contributingRules []ContributingRule

	for _, rule := range allRules {
		// 🚫 EXCLUSION_FILTER: Skip rules being deleted to prevent circular dependency
		if excludeMap[rule.ResourceIdentifier.Key()] {
			klog.Infof("  🚫 EXCLUSION_FILTER: Skipping excluded rule %s from contribution check", rule.Key())
			continue
		}

		// 🎯 CRITICAL FIX: Skip Ready=False RuleS2S from contributing to aggregated IEAgAgRules
		if !rule.Meta.IsReady() {
			klog.Infof("  🚫 READY_FILTER: Skipping inactive (Ready=False) RuleS2S %s from contribution", rule.Key())
			continue
		}

		// No deletion checking needed in our backend implementation
		contributes, ports, err := s.checkIfRuleContributes(ctx, &rule, currentRule, localService, targetService)
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
	if len(service1.AddressGroups) != len(service2.AddressGroups) {
		klog.V(2).Infof("  ❌ COMPARE_SERVICES: Different AG counts - %s has %d, %s has %d",
			service1.Key(), len(service1.AddressGroups),
			service2.Key(), len(service2.AddressGroups))
		return false
	}

	// Create map of AddressGroup keys from service1
	agMap := make(map[string]bool)
	for _, ag := range service1.AddressGroups {
		key := s.addressGroupRefKey(ag)
		agMap[key] = true
		klog.V(2).Infof("  📍 COMPARE_SERVICES: Service1 AddressGroup: %s", key)
	}

	// Check if all AddressGroups from service2 exist in service1
	for _, ag := range service2.AddressGroups {
		key := s.addressGroupRefKey(ag)
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
	klog.Infof("🔀 PORT_AGGREGATION: Aggregating ports for protocol %s from %d contributing rules (using reference pattern)",
		protocol, len(contributingRules))

	// Simple deduplication using map[string]bool like reference implementation (lines 793-798)
	portSet := make(map[string]bool)

	// Process ALL pre-populated ports from ContributingRule.Ports (NO PROTOCOL FILTERING)
	// Following reference implementation exactly - just aggregate all ports
	for _, rule := range contributingRules {
		klog.V(2).Infof("  📦 PORT_AGGREGATION: Processing rule %s with %d pre-populated ports",
			rule.RuleS2S.Key(), len(rule.Ports))

		// 🚀 REFERENCE MATCH: Aggregate ALL ports without protocol filtering (reference lines 795-798)
		for _, port := range rule.Ports {
			portSet[port] = true
			klog.V(3).Infof("    ➕ PORT_AGGREGATION: Added port %s from rule %s", port, rule.RuleS2S.Key())
		}

		klog.V(2).Infof("    ✅ PORT_AGGREGATION: Rule %s contributed %d ports",
			rule.RuleS2S.Key(), len(rule.Ports))
	}

	// Convert set to sorted slice (same as reference)
	var aggregatedPorts []string
	for port := range portSet {
		aggregatedPorts = append(aggregatedPorts, port)
	}

	sort.Strings(aggregatedPorts)

	klog.Infof("🎯 PORT_AGGREGATION: Final aggregated ports for protocol %s: %s (%d unique ports)",
		protocol, strings.Join(aggregatedPorts, ","), len(aggregatedPorts))

	return aggregatedPorts
}

// checkIfRuleContributes checks if a candidate rule should contribute to the same IEAgAg aggregation
// Based on reference lines 656-693
func (s *RuleS2SResourceService) checkIfRuleContributes(
	ctx context.Context,
	candidateRule *models.RuleS2S,
	currentRule *models.RuleS2S,
	localService *models.Service,
	targetService *models.Service,
) (bool, []string, error) {
	klog.Infof("🔍 CHECK_CONTRIBUTION: Checking if rule %s contributes to aggregation with rule %s",
		candidateRule.Key(), currentRule.Key())

	// Check if traffic direction matches
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

	// 🚀 BREAKTHROUGH FIX: Aggregation happens at AddressGroup combination level
	// Each rule generates localAG→targetAG combinations. Rules contribute to the SAME IEAgAgRule
	// if they generate overlapping localAG→targetAG→protocol combinations

	// Generate all AddressGroup combinations for current rule
	currentCombinations := s.generateAGCombinations(localService, targetService, currentRule.Traffic)
	candidateCombinations := s.generateAGCombinations(candidateLocalService, candidateTargetService, candidateRule.Traffic)

	klog.Infof("  🔍 CHECK_CONTRIBUTION: Current rule %s generates %d AG combinations: %v",
		currentRule.Key(), len(currentCombinations), currentCombinations)
	klog.Infof("  🔍 CHECK_CONTRIBUTION: Candidate rule %s generates %d AG combinations: %v",
		candidateRule.Key(), len(candidateCombinations), candidateCombinations)

	// Find overlapping combinations (same traffic direction and same localAG→targetAG pair)
	hasOverlap := false
	var overlappingCombination string
	for _, currentCombo := range currentCombinations {
		for _, candidateCombo := range candidateCombinations {
			if currentCombo == candidateCombo {
				hasOverlap = true
				overlappingCombination = currentCombo
				klog.Infof("  ✅ CHECK_CONTRIBUTION: Found overlapping combination: %s", currentCombo)
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

	klog.Infof("  ✅ CHECK_CONTRIBUTION: Rules share combination '%s' - aggregation possible", overlappingCombination)

	// Extract ports based on traffic direction (same logic as reference)
	var ports []string
	// Extract ports based on traffic direction (following reference implementation)
	// INGRESS: use local service ports (service receiving traffic)
	// EGRESS: use target service ports (service receiving traffic)
	if strings.ToLower(string(candidateRule.Traffic)) == "ingress" {
		ports = s.extractPortStringsFromService(*candidateLocalService)
		klog.Infof("  📍 DEBUG_PORT_EXTRACTION: INGRESS rule %s - extracting ports from LOCAL service %s: %s",
			candidateRule.Key(), candidateLocalService.Key(), strings.Join(ports, ","))
	} else {
		ports = s.extractPortStringsFromService(*candidateTargetService)
		klog.Infof("  📍 DEBUG_PORT_EXTRACTION: EGRESS rule %s - extracting ports from TARGET service %s: %s",
			candidateRule.Key(), candidateTargetService.Key(), strings.Join(ports, ","))
	}

	klog.Infof("  ✅ CHECK_CONTRIBUTION: Rule %s contributes %d ports: %s",
		candidateRule.Key(), len(ports), strings.Join(ports, ","))

	return true, ports, nil
}

// getServicesForRule gets local and target services for a RuleS2S with populated AddressGroups
// 🚀 CRITICAL FIX: Now populates Service.AddressGroups from AddressGroupBinding relationships
// 🔧 CROSS-RULES2S TIMING FIX: Uses ReadCommitted reader to see recently deleted AddressGroupBindings
func (s *RuleS2SResourceService) getServicesForRule(
	ctx context.Context,
	rule *models.RuleS2S,
) (*models.Service, *models.Service, error) {
	// 🔧 FIX: Use ReadCommitted isolation to see recently committed binding deletions
	// This resolves the Cross-RuleS2S aggregation timing bug where deleted AddressGroupBindings
	// were still visible in populateServiceAddressGroups due to connection pool timing issues
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
	// Use provided reader instead of creating new session
	// This ensures consistent Service.AddressGroups state

	// Get local service alias
	localServiceAliasID := models.ResourceIdentifier{
		Name:      rule.ServiceLocalRef.Name,
		Namespace: rule.ServiceLocalRef.Namespace,
	}
	localServiceAlias, err := reader.GetServiceAliasByID(ctx, localServiceAliasID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "local service alias %s not found", localServiceAliasID.Key())
	}

	// Get target service alias
	targetServiceAliasID := models.ResourceIdentifier{
		Name:      rule.ServiceRef.Name,
		Namespace: rule.ServiceRef.Namespace,
	}
	targetServiceAlias, err := reader.GetServiceAliasByID(ctx, targetServiceAliasID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "target service alias %s not found", targetServiceAliasID.Key())
	}

	// Get local service
	localServiceID := models.ResourceIdentifier{
		Name:      localServiceAlias.ServiceRef.Name,
		Namespace: localServiceAlias.Namespace,
	}
	localService, err := reader.GetServiceByID(ctx, localServiceID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "local service %s not found", localServiceID.Key())
	}

	// Get target service
	targetServiceID := models.ResourceIdentifier{
		Name:      targetServiceAlias.ServiceRef.Name,
		Namespace: targetServiceAlias.Namespace,
	}
	targetService, err := reader.GetServiceByID(ctx, targetServiceID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "target service %s not found", targetServiceID.Key())
	}

	// 🚀 CRITICAL FIX: Populate AddressGroups from AddressGroupBinding relationships
	// This was the root cause - services had empty AddressGroups so all appeared identical
	localService, err = s.populateServiceAddressGroups(ctx, reader, localService)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to populate AddressGroups for local service %s", localServiceID.Key())
	}

	targetService, err = s.populateServiceAddressGroups(ctx, reader, targetService)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to populate AddressGroups for target service %s", targetServiceID.Key())
	}

	klog.V(2).Infof("🔍 SERVICES_WITH_ADDRESSGROUPS: Local service %s has %d AddressGroups, Target service %s has %d AddressGroups",
		localService.Key(), len(localService.AddressGroups),
		targetService.Key(), len(targetService.AddressGroups))

	return localService, targetService, nil
}

// populateServiceAddressGroups populates Service.AddressGroups from AddressGroupBinding relationships
// 🚀 CRITICAL FIX: This method fixes the Cross-RuleS2S aggregation bug by ensuring services have
// their AddressGroups field properly populated from AddressGroupBinding relationships
func (s *RuleS2SResourceService) populateServiceAddressGroups(
	ctx context.Context,
	reader ports.Reader,
	service *models.Service,
) (*models.Service, error) {
	klog.V(2).Infof("🔧 POPULATE_ADDRESSGROUPS: Starting AddressGroup population for service %s", service.Key())

	// Create a copy of the service to avoid modifying the original
	serviceCopy := *service
	serviceCopy.AddressGroups = []models.AddressGroupRef{} // Reset to empty slice

	// Find all AddressGroupBindings that reference this service
	err := reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		// Check if this binding references our service
		if binding.ServiceRef.Name == service.Name && binding.ServiceRef.Namespace == service.Namespace {
			klog.V(2).Infof("  🔗 FOUND_BINDING: %s → AddressGroup %s/%s",
				binding.Key(), binding.AddressGroupRef.Namespace, binding.AddressGroupRef.Name)

			// Create AddressGroupRef from the binding
			agRef := models.AddressGroupRef{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: binding.AddressGroupRef.APIVersion,
					Kind:       binding.AddressGroupRef.Kind,
					Name:       binding.AddressGroupRef.Name,
				},
				Namespace: binding.AddressGroupRef.Namespace,
			}

			// Add to service's AddressGroups
			serviceCopy.AddressGroups = append(serviceCopy.AddressGroups, agRef)
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return nil, errors.Wrapf(err, "failed to list AddressGroupBindings for service %s", service.Key())
	}

	klog.V(2).Infof("  ✅ POPULATE_ADDRESSGROUPS: Service %s now has %d AddressGroups populated",
		service.Key(), len(serviceCopy.AddressGroups))

	// Log the AddressGroups for debugging
	for i, ag := range serviceCopy.AddressGroups {
		klog.V(2).Infof("    📍 AG[%d]: %s/%s", i, ag.Namespace, ag.Name)
	}

	return &serviceCopy, nil
}

// extractPortStringsFromService extracts port strings from a service's ingress ports
// Based on reference lines 777-786 - returns []string for cross-RuleS2S aggregation
func (s *RuleS2SResourceService) extractPortStringsFromService(
	service models.Service,
) []string {
	var ports []string
	for _, port := range service.IngressPorts {
		ports = append(ports, port.Port)
	}
	klog.V(2).Infof("  📦 EXTRACT_PORTS: Service %s has %d ingress ports: %s",
		service.Key(), len(ports), strings.Join(ports, ","))
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
	// Get service aliases
	localServiceAliasID := models.ResourceIdentifier{
		Name:      rule.ServiceLocalRef.Name,
		Namespace: rule.ServiceLocalRef.Namespace,
	}
	targetServiceAliasID := models.ResourceIdentifier{
		Name:      rule.ServiceRef.Name,
		Namespace: rule.ServiceRef.Namespace,
	}

	localServiceAlias, err := reader.GetServiceAliasByID(ctx, localServiceAliasID)
	if err != nil {
		return false, nil // Skip if service alias not found
	}

	targetServiceAlias, err := reader.GetServiceAliasByID(ctx, targetServiceAliasID)
	if err != nil {
		return false, nil // Skip if service alias not found
	}

	// Get actual services
	localServiceID := models.ResourceIdentifier{
		Name:      localServiceAlias.ServiceRef.Name,
		Namespace: localServiceAlias.Namespace,
	}
	targetServiceID := models.ResourceIdentifier{
		Name:      targetServiceAlias.ServiceRef.Name,
		Namespace: targetServiceAlias.Namespace,
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
	for _, localAG := range localService.AddressGroups {
		for _, targetAG := range targetService.AddressGroups {
			// Check if this combination matches the aggregation group
			if s.addressGroupRefMatches(localAG, group.LocalAG) &&
				s.addressGroupRefMatches(targetAG, group.TargetAG) {

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
	// Get service aliases
	localServiceAliasID := models.ResourceIdentifier{
		Name:      rule.ServiceLocalRef.Name,
		Namespace: rule.ServiceLocalRef.Namespace,
	}
	targetServiceAliasID := models.ResourceIdentifier{
		Name:      rule.ServiceRef.Name,
		Namespace: rule.ServiceRef.Namespace,
	}

	localServiceAlias, err := reader.GetServiceAliasByID(ctx, localServiceAliasID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get local service alias %s", rule.ServiceLocalRef.Name)
	}

	targetServiceAlias, err := reader.GetServiceAliasByID(ctx, targetServiceAliasID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get target service alias %s", rule.ServiceRef.Name)
	}

	// Get actual services
	localServiceID := models.ResourceIdentifier{
		Name:      localServiceAlias.ServiceRef.Name,
		Namespace: localServiceAlias.Namespace,
	}
	targetServiceID := models.ResourceIdentifier{
		Name:      targetServiceAlias.ServiceRef.Name,
		Namespace: targetServiceAlias.Namespace,
	}

	localService, err := reader.GetServiceByID(ctx, localServiceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get local service %s", localServiceAlias.ServiceRef.Name)
	}

	targetService, err := reader.GetServiceByID(ctx, targetServiceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get target service %s", targetServiceAlias.ServiceRef.Name)
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
	for _, localAG := range localService.AddressGroups {
		for _, targetAG := range targetService.AddressGroups {
			// Check what protocols this service supports
			protocolsSupported := make(map[models.TransportProtocol]bool)
			for _, port := range portsSource.IngressPorts {
				protocolsSupported[port.Protocol] = true
			}

			// Create aggregation groups for each protocol
			for protocol := range protocolsSupported {
				group := AggregationGroup{
					Traffic:   rule.Traffic,
					LocalAG:   localAG,
					TargetAG:  targetAG,
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
// This matches the reference implementation's nested loop: for localAG, for targetAG
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
	// This mirrors reference implementation lines 427-428: for localAG, for targetAG
	for _, localAG := range localService.AddressGroups {
		for _, targetAG := range targetService.AddressGroups {
			for protocol := range protocolsWithPorts {
				// Format: traffic-localAG_namespace/name-targetAG_namespace/name-protocol
				combination := fmt.Sprintf("%s-%s/%s-%s/%s-%s",
					traffic,
					localAG.Namespace, localAG.Name,
					targetAG.Namespace, targetAG.Name,
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

	// Get the ServiceAlias references from the not-ready RuleS2S
	localServiceAliasID := models.ResourceIdentifier{
		Name:      ruleS2S.ServiceLocalRef.Name,
		Namespace: ruleS2S.ServiceLocalRef.Namespace,
	}
	targetServiceAliasID := models.ResourceIdentifier{
		Name:      ruleS2S.ServiceRef.Name,
		Namespace: ruleS2S.ServiceRef.Namespace,
	}

	// Step 2: Get the actual Service IDs that these ServiceAliases reference
	localAlias, err := reader.GetServiceAliasByID(ctx, localServiceAliasID)
	if err != nil {
		klog.Errorf("🧹 CleanupIEAgAgRulesForRuleS2S: Could not get local ServiceAlias %s: %v", localServiceAliasID.Key(), err)
		// Continue with cleanup even if we can't find the alias
	}

	targetAlias, err := reader.GetServiceAliasByID(ctx, targetServiceAliasID)
	if err != nil {
		klog.Errorf("🧹 CleanupIEAgAgRulesForRuleS2S: Could not get target ServiceAlias %s: %v", targetServiceAliasID.Key(), err)
		// Continue with cleanup even if we can't find the alias
	}

	// Step 3: Trigger regeneration for the affected services
	// This will cause the aggregation system to regenerate all IEAgAgRules for these services,
	// naturally excluding the not-ready RuleS2S

	var affectedServices []models.ResourceIdentifier

	if localAlias != nil {
		localServiceID := models.ResourceIdentifier{
			Name:      localAlias.ServiceRef.Name,
			Namespace: localAlias.ServiceRef.Namespace,
		}
		affectedServices = append(affectedServices, localServiceID)
		klog.Infof("🧹 CleanupIEAgAgRulesForRuleS2S: Will regenerate aggregation for local service %s", localServiceID.Key())
	}

	if targetAlias != nil {
		targetServiceID := models.ResourceIdentifier{
			Name:      targetAlias.ServiceRef.Name,
			Namespace: targetAlias.ServiceRef.Namespace,
		}
		// Avoid duplicate if it's the same service
		if len(affectedServices) == 0 || affectedServices[0].Key() != targetServiceID.Key() {
			affectedServices = append(affectedServices, targetServiceID)
			klog.Infof("🧹 CleanupIEAgAgRulesForRuleS2S: Will regenerate aggregation for target service %s", targetServiceID.Key())
		}
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
				klog.Infof("🗑️ CleanupIEAgAgRulesForRuleS2S: Found %d IEAgAgRules that were deleted, syncing to sgroups", len(deletedRules))

				// Sync each deleted rule to sgroups with DELETE operation
				for _, deletedRule := range deletedRules {
					if err := s.syncManager.SyncEntity(ctx, &deletedRule, types.SyncOperationDelete); err != nil {
						klog.Errorf("❌ CleanupIEAgAgRulesForRuleS2S: Failed to sync deleted rule %s to sgroups: %v", deletedRule.GetSyncKey(), err)
					} else {
						klog.Infof("✅ CleanupIEAgAgRulesForRuleS2S: Successfully synced deletion of rule %s to sgroups", deletedRule.GetSyncKey())
					}
				}
			} else {
				klog.Infof("✅ CleanupIEAgAgRulesForRuleS2S: No IEAgAgRules were deleted during regeneration")
			}
		}
	} else if s.syncManager == nil {
		klog.Warningf("⚠️ CleanupIEAgAgRulesForRuleS2S: syncManager is nil - external sync SKIPPED")
	}

	klog.Infof("🏁 CleanupIEAgAgRulesForRuleS2S: Completed aggregation-based cleanup WITH external sync for RuleS2S %s/%s", ruleS2S.Namespace, ruleS2S.Name)
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
