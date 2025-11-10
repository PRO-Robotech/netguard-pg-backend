package resources

import (
	"context"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

type SvcFqdnRuleResourceService struct {
	registry ports.Registry
}

func NewSvcFqdnRuleResourceService(registry ports.Registry) *SvcFqdnRuleResourceService {
	return &SvcFqdnRuleResourceService{registry: registry}
}

func (s *SvcFqdnRuleResourceService) GetSvcFqdnRules(ctx context.Context, scope ports.Scope) ([]models.SvcFqdnRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var rules []models.SvcFqdnRule
	if err := reader.ListSvcFqdnRules(ctx, func(rule models.SvcFqdnRule) error {
		rules = append(rules, rule)
		return nil
	}, scope); err != nil {
		return nil, errors.Wrap(err, "failed to list SvcFqdnRules")
	}

	return rules, nil
}

func (s *SvcFqdnRuleResourceService) GetSvcFqdnRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.SvcFqdnRule, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	rule, err := reader.GetSvcFqdnRuleByID(ctx, id)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get SvcFqdnRule %s/%s", id.Namespace, id.Name)
	}

	return rule, nil
}

func (s *SvcFqdnRuleResourceService) CreateSvcFqdnRule(ctx context.Context, rule models.SvcFqdnRule) (err error) {
	return s.SyncSvcFqdnRules(ctx, []models.SvcFqdnRule{rule}, ports.EmptyScope{}, models.SyncOpUpsert)
}

func (s *SvcFqdnRuleResourceService) UpdateSvcFqdnRule(ctx context.Context, rule models.SvcFqdnRule) error {
	return s.SyncSvcFqdnRules(ctx, []models.SvcFqdnRule{rule}, ports.EmptyScope{}, models.SyncOpUpsert)
}

func (s *SvcFqdnRuleResourceService) DeleteSvcFqdnRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier) error {
	if len(ids) == 0 {
		return nil
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

	if err = writer.DeleteSvcFqdnRulesByIDs(ctx, ids); err != nil {
		return errors.Wrap(err, "failed to delete SvcFqdnRules")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}

func (s *SvcFqdnRuleResourceService) SyncSvcFqdnRules(ctx context.Context, rules []models.SvcFqdnRule, scope ports.Scope, syncOp models.SyncOp) (err error) {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}

	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncSvcFqdnRules(ctx, rules, scope, ports.SyncOption{Operation: syncOp}); err != nil {
		return errors.Wrap(err, "failed to sync SvcFqdnRules")
	}

	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}
