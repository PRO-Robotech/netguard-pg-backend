package base

import (
	"context"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

type BackendOperations[D any] interface {
	Get(ctx context.Context, id models.ResourceIdentifier) (*D, error)
	List(ctx context.Context, scope ports.Scope) ([]D, error)
	Create(ctx context.Context, obj *D) error
	Update(ctx context.Context, obj *D) error
	Delete(ctx context.Context, id models.ResourceIdentifier) error
	MarkForDeletion(ctx context.Context, namespace, name, kind string) error
}
