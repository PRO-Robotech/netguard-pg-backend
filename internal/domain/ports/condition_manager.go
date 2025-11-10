package ports

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
)

// ConditionManager defines the minimal surface required by components that need to
// recalculate resource conditions outside of the main request flow.
type ConditionManager interface {
	ProcessAddressGroupConditions(ctx context.Context, ag *models.AddressGroup) error
	SaveAddressGroupConditions(ctx context.Context, ag *models.AddressGroup) error

	ProcessHostConditions(ctx context.Context, host *models.Host, syncResult error) error
	SaveHostConditions(ctx context.Context, host *models.Host) error
}
