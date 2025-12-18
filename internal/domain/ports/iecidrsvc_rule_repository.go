package ports

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
)

// IECidrSvcRuleRepository defines persistence operations for ingress/egress CIDR-to-service rules
type IECidrSvcRuleRepository interface {
	// Reader operations
	GetByKey(ctx context.Context, id models.ResourceIdentifier) (*models.IECidrSvcRule, error)
	ListAll(ctx context.Context, consume func(models.IECidrSvcRule) error, scope Scope) error
	FindRulesByService(ctx context.Context, namespace, serviceName string) ([]models.IECidrSvcRule, error)

	// Writer operations
	SyncIECidrSvcRules(ctx context.Context, rules []models.IECidrSvcRule, scope Scope, options ...Option) error
}
