package resources

import (
	"context"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/interfaces"
)

// SvcSvcRuleResourceService handles SvcSvcRule CRUD operations
type SvcSvcRuleResourceService struct {
	registry         ports.Registry
	syncManager      interfaces.SyncManager
	conditionManager SvcSvcRuleConditionManager
}

// SvcSvcRuleConditionManager interface for handling SvcSvcRule conditions
type SvcSvcRuleConditionManager interface {
	ProcessSvcSvcRuleConditions(ctx context.Context, rule *models.SvcSvcRule) error
}

// NewSvcSvcRuleResourceService creates a new SvcSvcRuleResourceService
func NewSvcSvcRuleResourceService(registry ports.Registry, syncManager interfaces.SyncManager, conditionManager SvcSvcRuleConditionManager) *SvcSvcRuleResourceService {
	return &SvcSvcRuleResourceService{
		registry:         registry,
		syncManager:      syncManager,
		conditionManager: conditionManager,
	}
}

// =============================================================================
// SvcSvcRule Operations
// =============================================================================

// GetSvcSvcRules returns all SvcSvcRules within scope
func (s *SvcSvcRuleResourceService) GetSvcSvcRules(ctx context.Context, scope ports.Scope) ([]models.SvcSvcRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var rules []models.SvcSvcRule
	err = reader.ListSvcSvcRules(ctx, func(rule models.SvcSvcRule) error {
		rules = append(rules, rule)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list SvcSvcRules")
	}
	return rules, nil
}

// GetSvcSvcRuleByID returns SvcSvcRule by ID
func (s *SvcSvcRuleResourceService) GetSvcSvcRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcSvcRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	rule, err := reader.GetSvcSvcRuleByID(ctx, id)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get SvcSvcRule %s/%s", id.Namespace, id.Name)
	}

	return rule, nil
}

// GetSvcSvcRulesByIDs returns multiple SvcSvcRules by their IDs
func (s *SvcSvcRuleResourceService) GetSvcSvcRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) ([]models.SvcSvcRule, error) {
	if len(ids) == 0 {
		return []models.SvcSvcRule{}, nil
	}

	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	scope := ports.NewResourceIdentifierScope(ids...)
	var rules []models.SvcSvcRule
	err = reader.ListSvcSvcRules(ctx, func(rule models.SvcSvcRule) error {
		rules = append(rules, rule)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list SvcSvcRules by IDs")
	}

	return rules, nil
}

// CreateSvcSvcRule creates a new SvcSvcRule
func (s *SvcSvcRuleResourceService) CreateSvcSvcRule(ctx context.Context, rule models.SvcSvcRule) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}

	// Process conditions if condition manager is available
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessSvcSvcRuleConditions(ctx, &rule); err != nil {
			return errors.Wrap(err, "failed to process SvcSvcRule conditions")
		}
	}

	// Sync the rule (will upsert)
	if err := writer.SyncSvcSvcRules(ctx, []models.SvcSvcRule{rule}, ports.EmptyScope{}, ports.SyncOption{Operation: models.SyncOpUpsert}); err != nil {
		return errors.Wrapf(err, "failed to create SvcSvcRule %s/%s", rule.Namespace, rule.Name)
	}

	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}

// UpdateSvcSvcRule updates an existing SvcSvcRule
func (s *SvcSvcRuleResourceService) UpdateSvcSvcRule(ctx context.Context, rule models.SvcSvcRule) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}

	// Process conditions if condition manager is available
	if s.conditionManager != nil {
		if err := s.conditionManager.ProcessSvcSvcRuleConditions(ctx, &rule); err != nil {
			return errors.Wrap(err, "failed to process SvcSvcRule conditions")
		}
	}

	// Sync the rule (will upsert)
	if err := writer.SyncSvcSvcRules(ctx, []models.SvcSvcRule{rule}, ports.EmptyScope{}, ports.SyncOption{Operation: models.SyncOpUpsert}); err != nil {
		return errors.Wrapf(err, "failed to update SvcSvcRule %s/%s", rule.Namespace, rule.Name)
	}

	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}

// SyncSvcSvcRules syncs multiple SvcSvcRules within a scope
func (s *SvcSvcRuleResourceService) SyncSvcSvcRules(ctx context.Context, rules []models.SvcSvcRule, scope ports.Scope, syncOp models.SyncOp) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}

	// Process conditions for each rule if condition manager is available
	if s.conditionManager != nil {
		for i := range rules {
			if err := s.conditionManager.ProcessSvcSvcRuleConditions(ctx, &rules[i]); err != nil {
				return errors.Wrapf(err, "failed to process conditions for SvcSvcRule %s/%s", rules[i].Namespace, rules[i].Name)
			}
		}
	}

	// Sync rules
	if err := writer.SyncSvcSvcRules(ctx, rules, scope, ports.SyncOption{Operation: syncOp}); err != nil {
		return errors.Wrap(err, "failed to sync SvcSvcRules")
	}

	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}

// DeleteSvcSvcRulesByIDs deletes SvcSvcRules by their IDs
func (s *SvcSvcRuleResourceService) DeleteSvcSvcRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	if len(ids) == 0 {
		return nil
	}

	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}

	if err := writer.DeleteSvcSvcRulesByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete SvcSvcRules")
	}

	if err := writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}
