package resources

import (
	"context"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// IECidrSvcRuleConditionManager interface for handling IECidrSvcRule conditions
type IECidrSvcRuleConditionManager interface {
	ProcessIECidrSvcRuleConditions(ctx context.Context, rule *models.IECidrSvcRule) error
}

type IECidrSvcRuleResourceService struct {
	registry         ports.Registry
	conditionManager IECidrSvcRuleConditionManager
}

func NewIECidrSvcRuleResourceService(registry ports.Registry, conditionManager IECidrSvcRuleConditionManager) *IECidrSvcRuleResourceService {
	return &IECidrSvcRuleResourceService{
		registry:         registry,
		conditionManager: conditionManager,
	}
}

func (s *IECidrSvcRuleResourceService) GetIECidrSvcRules(ctx context.Context, scope ports.Scope) ([]models.IECidrSvcRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var rules []models.IECidrSvcRule
	if err := reader.ListIECidrSvcRules(ctx, func(rule models.IECidrSvcRule) error {
		rules = append(rules, rule)
		return nil
	}, scope); err != nil {
		return nil, errors.Wrap(err, "failed to list IECidrSvcRules")
	}

	return rules, nil
}

func (s *IECidrSvcRuleResourceService) GetIECidrSvcRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IECidrSvcRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	rule, err := reader.GetIECidrSvcRuleByID(ctx, id)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get IECidrSvcRule %s/%s", id.Namespace, id.Name)
	}

	return rule, nil
}

func (s *IECidrSvcRuleResourceService) CreateIECidrSvcRule(ctx context.Context, rule models.IECidrSvcRule) (err error) {
	return s.SyncIECidrSvcRules(ctx, []models.IECidrSvcRule{rule}, ports.EmptyScope{}, models.SyncOpUpsert)
}

func (s *IECidrSvcRuleResourceService) UpdateIECidrSvcRule(ctx context.Context, rule models.IECidrSvcRule) error {
	return s.SyncIECidrSvcRules(ctx, []models.IECidrSvcRule{rule}, ports.EmptyScope{}, models.SyncOpUpsert)
}

func (s *IECidrSvcRuleResourceService) DeleteIECidrSvcRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	if len(ids) == 0 {
		return nil
	}

	return s.registry.ExecuteDeleteWithRetry(ctx, func(writer ports.Writer) error {
		return writer.DeleteIECidrSvcRulesByIDs(ctx, ids)
	})
}

func (s *IECidrSvcRuleResourceService) SyncIECidrSvcRules(ctx context.Context, rules []models.IECidrSvcRule, scope ports.Scope, syncOp models.SyncOp) (err error) {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}

	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	// Process conditions for each rule if condition manager is available
	if s.conditionManager != nil {
		for i := range rules {
			if err := s.conditionManager.ProcessIECidrSvcRuleConditions(ctx, &rules[i]); err != nil {
				return errors.Wrapf(err, "failed to process conditions for IECidrSvcRule %s/%s", rules[i].Namespace, rules[i].Name)
			}
		}
	}

	if err = writer.SyncIECidrSvcRules(ctx, rules, scope, ports.SyncOption{Operation: syncOp}); err != nil {
		return errors.Wrap(err, "failed to sync IECidrSvcRules")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}
