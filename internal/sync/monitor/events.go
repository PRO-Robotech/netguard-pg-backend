package monitor

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// ConnectionEventType - тип события связи с SGROUP
type ConnectionEventType int

const (
	// EventConnected - stream с SGROUP установлен успешно
	EventConnected ConnectionEventType = iota
	// EventDisconnected - stream с SGROUP оборвался
	EventDisconnected
	// EventTimestamp - получен timestamp из stream
	EventTimestamp
)

// String возвращает строковое представление типа события
func (t ConnectionEventType) String() string {
	switch t {
	case EventConnected:
		return "Connected"
	case EventDisconnected:
		return "Disconnected"
	case EventTimestamp:
		return "Timestamp"
	default:
		return "Unknown"
	}
}

// ConnectionEvent - событие связи с SGROUP
type ConnectionEvent struct {
	Type       ConnectionEventType    // тип события
	Timestamp  *timestamppb.Timestamp // для EventTimestamp - полученный timestamp
	Error      error                  // для EventDisconnected - причина обрыва
	OccurredAt time.Time              // время возникновения события
}

// ConnectionListener - интерфейс подписчика на события связи с SGROUP
// Все методы вызываются в отдельных goroutines и должны быть thread-safe
type ConnectionListener interface {
	// OnConnectionEvent обрабатывает событие связи с SGROUP
	// Вызывается асинхронно, не должен блокироваться надолго
	OnConnectionEvent(event ConnectionEvent)

	// GetListenerName возвращает имя listener для логирования
	GetListenerName() string
}

// ConnectionStats - статистика работы монитора
type ConnectionStats struct {
	IsConnected   bool                   // текущее состояние подключения
	LastConnected time.Time              // время последнего успешного подключения
	LastTimestamp *timestamppb.Timestamp // последний полученный timestamp
	ListenerCount int                    // количество активных подписчиков
}

// SyncChangeEvent represents a data change event from SGROUP
// This replaces the deprecated detector.ChangeEvent
type SyncChangeEvent struct {
	// Timestamp when the change occurred (from SyncStatuses stream)
	Timestamp time.Time `json:"timestamp"`

	// Source is the name of the external system that generated the change
	Source string `json:"source"`

	// Metadata contains additional information about the change
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}
