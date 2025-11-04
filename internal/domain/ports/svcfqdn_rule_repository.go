package ports

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
)

// SvcFqdnRuleRepository defines persistence operations for service-to-FQDN rules
type SvcFqdnRuleRepository interface {
	// Reader operations
	GetByKey(ctx context.Context, id models.ResourceIdentifier) (*models.SvcFqdnRule, error)
	ListAll(ctx context.Context, consume func(models.SvcFqdnRule) error, scope Scope) error
	FindRulesByService(ctx context.Context, namespace, serviceName string) ([]models.SvcFqdnRule, error)

	// Writer operations
	SyncSvcFqdnRules(ctx context.Context, rules []models.SvcFqdnRule, scope Scope, options ...Option) error
}
