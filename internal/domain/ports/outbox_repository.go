package ports

import (
	"context"
	"github.com/google/uuid"
	"netguard-pg-backend/internal/domain"
	"time"
)

type OutboxRepository interface {
	Create(ctx context.Context, entry *domain.OutboxEntry) error
	FindByID(ctx context.Context, id uuid.UUID) (*domain.OutboxEntry, error)
	FindByResource(ctx context.Context, resourceType string, resourceID uuid.UUID) ([]*domain.OutboxEntry, error)
	FindPending(ctx context.Context, limit int) ([]*domain.OutboxEntry, error)
	CountPending(ctx context.Context) (int, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OutboxStatus,
		processedAt *time.Time, lastError *string, errorCategory *string) error
	IncrementAttempts(ctx context.Context, id uuid.UUID, nextRetryAt *time.Time) error
	MarkCompleted(ctx context.Context, id uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteOldSuccessful(ctx context.Context, olderThan time.Duration) (int64, error)
}
