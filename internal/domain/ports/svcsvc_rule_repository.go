package ports

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
)

// SvcSvcRuleRepository defines the port for SvcSvcRule persistence
// Implements Clean Architecture Ports & Adapters pattern
//
// This repository handles Service-to-Service firewall rules with:
// - Full CRUD operations
// - NamespacedObjectReference storage (JSONB)
// - xSvcSvcRules array maintenance (triggered automatically by DB)
// - OUTBOX integration (triggered automatically by DB)
type SvcSvcRuleRepository interface {
	// Reader methods

	// GetByKey returns a single SvcSvcRule by namespace/name
	// Returns ErrNotFound if the rule does not exist
	//
	// Example:
	//   rule, err := repo.GetByKey(ctx, models.ResourceIdentifier{Namespace: "prod", Name: "rule-001"})
	GetByKey(ctx context.Context, id models.ResourceIdentifier) (*models.SvcSvcRule, error)

	// ListAll returns all SvcSvcRules with optional scope filtering
	// Results are ordered by namespace, name
	// Filters out resources being deleted (deletion_timestamp IS NOT NULL)
	//
	// Example:
	//   err := repo.ListAll(ctx, func(rule models.SvcSvcRule) error {
	//     // Process each rule
	//     return nil
	//   }, ports.InNamespace("prod"))
	ListAll(ctx context.Context, consume func(models.SvcSvcRule) error, scope Scope) error

	// FindRulesByService returns all rules where the given service participates
	// as either ServiceFrom or ServiceTo
	//
	// Used for:
	// - Service deletion protection (can't delete if rules exist)
	// - Populating xSvcSvcRules arrays
	// - Querying service dependencies
	//
	// Example:
	//   rules, err := repo.FindRulesByService(ctx, "prod", "service-a")
	FindRulesByService(ctx context.Context, namespace, serviceName string) ([]models.SvcSvcRule, error)

	// FindRuleByServicePair returns a rule for a specific service pair
	// Returns ErrNotFound if no rule exists for this pair
	//
	// Used for UNIQUE constraint validation before creation
	//
	// Example:
	//   rule, err := repo.FindRuleByServicePair(ctx, "prod", "service-a", "service-b")
	FindRuleByServicePair(ctx context.Context, ruleNamespace, serviceFromName, serviceFromNamespace, serviceToName, serviceToNamespace string) (*models.SvcSvcRule, error)

	// Writer methods

	// SyncSvcSvcRules syncs SvcSvcRule resources to PostgreSQL
	// Supports operations: Upsert, Delete, FullSync
	//
	// For Upsert/FullSync:
	// - Inserts new rules or updates existing ones
	// - Triggers update xSvcSvcRules arrays in services table (via DB trigger)
	// - Creates OUTBOX entries for SGROUP sync (via DB trigger)
	//
	// For Delete:
	// - Removes rules from database
	// - Triggers cleanup of xSvcSvcRules arrays (via DB trigger)
	// - Creates DELETE OUTBOX entries (via DB trigger)
	//
	// Validation Requirements:
	// - serviceFrom and serviceTo are IMMUTABLE (enforced by DB trigger)
	// - UNIQUE constraint on (namespace, serviceFrom, serviceTo) (enforced by DB)
	//
	// Example:
	//   rules := []models.SvcSvcRule{...}
	//   err := repo.SyncSvcSvcRules(ctx, rules, ports.InNamespace("prod"), ports.WithSyncOp(models.SyncOpUpsert))
	SyncSvcSvcRules(ctx context.Context, rules []models.SvcSvcRule, scope Scope, options ...Option) error
}
