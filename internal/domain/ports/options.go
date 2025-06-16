package ports

import (
	"netguard-pg-backend/internal/domain/models"
)

// SyncOption определяет опцию для операции синхронизации
type SyncOption struct {
	Operation models.SyncOp
}

// WithSyncOp создает опцию с указанной операцией синхронизации
func WithSyncOp(op models.SyncOp) Option {
	return SyncOption{Operation: op}
}
