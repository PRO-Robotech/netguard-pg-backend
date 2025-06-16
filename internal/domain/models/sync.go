package models

// ProtoToSyncOp преобразует proto SyncOp в models.SyncOp
func ProtoToSyncOp(protoSyncOp int32) SyncOp {
	switch protoSyncOp {
	case 0: // NoOp
		return SyncOpNoOp
	case 1: // FullSync
		return SyncOpFullSync
	case 2: // Upsert
		return SyncOpUpsert
	case 3: // Delete
		return SyncOpDelete
	default:
		return SyncOpFullSync // По умолчанию используем FullSync
	}
}

// SyncOpToProto преобразует models.SyncOp в proto SyncOp
func SyncOpToProto(syncOp SyncOp) int32 {
	switch syncOp {
	case SyncOpNoOp:
		return 0 // NoOp
	case SyncOpFullSync:
		return 1 // FullSync
	case SyncOpUpsert:
		return 2 // Upsert
	case SyncOpDelete:
		return 3 // Delete
	default:
		return 1 // По умолчанию используем FullSync
	}
}

// IsValidSyncOp проверяет, является ли значение допустимой операцией синхронизации
func IsValidSyncOp(syncOp SyncOp) bool {
	return syncOp >= SyncOpNoOp && syncOp <= SyncOpDelete
}

// DefaultSyncOp возвращает операцию синхронизации по умолчанию
func DefaultSyncOp() SyncOp {
	return SyncOpFullSync
}
