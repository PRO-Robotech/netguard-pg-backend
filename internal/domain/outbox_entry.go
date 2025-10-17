package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

// SyncOperation represents operation type
type SyncOperation string

const (
	SyncOperationCreate SyncOperation = "CREATE"
	SyncOperationUpdate SyncOperation = "UPDATE"
	SyncOperationDelete SyncOperation = "DELETE"
)

// TargetSystem represents external sync target
type TargetSystem string

const (
	TargetSystemSGROUP   TargetSystem = "SGROUP"
	TargetSystemInternal TargetSystem = "INTERNAL"
)

// OutboxStatus represents outbox entry status
type OutboxStatus string

const (
	OutboxStatusPending         OutboxStatus = "PENDING"
	OutboxStatusProcessing      OutboxStatus = "PROCESSING"
	OutboxStatusSuccess         OutboxStatus = "SUCCESS"
	OutboxStatusFailedRetryable OutboxStatus = "FAILED_RETRYABLE"
	OutboxStatusFailedPermanent OutboxStatus = "FAILED_PERMANENT"
)

// OutboxEntry represents a sync_outbox table record
type OutboxEntry struct {
	// Identification
	ID           uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ResourceType string    `json:"resource_type" gorm:"type:varchar(50);not null"`
	ResourceID   uuid.UUID `json:"resource_id" gorm:"type:uuid;not null"`

	// K8s Identifiers for namespace-based resource lookup
	// These fields enable querying entity tables by (namespace, name)
	ResourceNamespace string `json:"resource_namespace" gorm:"type:varchar(253);not null"`
	ResourceName      string `json:"resource_name" gorm:"type:varchar(253);not null"`

	// Operation
	Operation    SyncOperation `json:"operation" gorm:"type:sync_operation;not null"`
	TargetSystem TargetSystem  `json:"target_system" gorm:"type:target_system;not null"`

	// Data
	Payload          json.RawMessage `json:"payload" gorm:"type:jsonb;not null"`
	Delta            json.RawMessage `json:"delta,omitempty" gorm:"type:jsonb"`
	AffectsResources json.RawMessage `json:"affects_resources,omitempty" gorm:"type:jsonb"`

	// Status and retry
	Status      OutboxStatus `json:"status" gorm:"type:outbox_status;not null;default:'PENDING'"`
	Attempts    int          `json:"attempts" gorm:"not null;default:0"`
	MaxRetries  int          `json:"max_retries" gorm:"not null;default:5"`
	NextRetryAt *time.Time   `json:"next_retry_at,omitempty" gorm:"type:timestamptz"`

	// Error tracking
	LastError     *string `json:"last_error,omitempty" gorm:"type:text"`
	ErrorCategory *string `json:"error_category,omitempty" gorm:"type:varchar(50)"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at" gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"type:timestamptz;not null;default:now()"`
	ProcessedAt *time.Time `json:"processed_at,omitempty" gorm:"type:timestamptz"`
}

// TableName specifies the table name
func (OutboxEntry) TableName() string {
	return "sync_outbox"
}

// Validate validates the outbox entry
func (e *OutboxEntry) Validate() error {
	if e.ResourceType == "" {
		return fmt.Errorf("resource_type is required")
	}
	if e.ResourceID == uuid.Nil {
		return fmt.Errorf("resource_id is required")
	}
	// Validate K8s identifiers
	if e.ResourceNamespace == "" {
		return fmt.Errorf("resource_namespace is required")
	}
	if e.ResourceName == "" {
		return fmt.Errorf("resource_name is required")
	}
	if e.Operation == "" {
		return fmt.Errorf("operation is required")
	}
	if e.TargetSystem == "" {
		return fmt.Errorf("target_system is required")
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("payload is required")
	}
	if e.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be non-negative")
	}

	// Validate enums
	switch e.Operation {
	case SyncOperationCreate, SyncOperationUpdate, SyncOperationDelete:
		// Valid
	default:
		return fmt.Errorf("invalid operation: %s", e.Operation)
	}

	switch e.TargetSystem {
	case TargetSystemSGROUP, TargetSystemInternal:
		// Valid
	default:
		return fmt.Errorf("invalid target_system: %s", e.TargetSystem)
	}

	switch e.Status {
	case OutboxStatusPending, OutboxStatusProcessing, OutboxStatusSuccess,
		OutboxStatusFailedRetryable, OutboxStatusFailedPermanent:
		// Valid
	default:
		return fmt.Errorf("invalid status: %s", e.Status)
	}

	return nil
}

// IsRetryable checks if entry can be retried
func (e *OutboxEntry) IsRetryable() bool {
	if e.Status == OutboxStatusFailedPermanent {
		return false
	}
	if e.Status == OutboxStatusSuccess {
		return false
	}
	if e.Attempts >= e.MaxRetries {
		return false
	}
	return true
}

// CanTransitionTo checks if status transition is valid
func (e *OutboxEntry) CanTransitionTo(newStatus OutboxStatus) bool {
	switch e.Status {
	case OutboxStatusPending:
		return newStatus == OutboxStatusProcessing || newStatus == OutboxStatusSuccess
	case OutboxStatusProcessing:
		return newStatus == OutboxStatusSuccess ||
			newStatus == OutboxStatusFailedRetryable ||
			newStatus == OutboxStatusFailedPermanent
	case OutboxStatusFailedRetryable:
		return newStatus == OutboxStatusProcessing ||
			newStatus == OutboxStatusFailedPermanent
	case OutboxStatusSuccess:
		return false // Terminal state
	case OutboxStatusFailedPermanent:
		return false // Terminal state
	default:
		return false
	}
}

// MarkProcessing marks entry as processing
func (e *OutboxEntry) MarkProcessing() {
	e.Status = OutboxStatusProcessing
	e.UpdatedAt = time.Now()
}

// MarkSuccess marks entry as successfully processed
func (e *OutboxEntry) MarkSuccess() {
	e.Status = OutboxStatusSuccess
	now := time.Now()
	e.UpdatedAt = now
	e.ProcessedAt = &now
	e.LastError = nil
	e.ErrorCategory = nil
}

// MarkFailed marks entry as failed (retryable or permanent)
func (e *OutboxEntry) MarkFailed(err error, category string, isPermanent bool) {
	e.Attempts++
	errMsg := err.Error()
	e.LastError = &errMsg
	e.ErrorCategory = &category
	e.UpdatedAt = time.Now()

	if isPermanent || e.Attempts >= e.MaxRetries {
		e.Status = OutboxStatusFailedPermanent
		e.NextRetryAt = nil
	} else {
		e.Status = OutboxStatusFailedRetryable
		nextRetry := e.CalculateNextRetry()
		e.NextRetryAt = &nextRetry
	}
}

// CalculateNextRetry calculates next retry time using exponential backoff
func (e *OutboxEntry) CalculateNextRetry() time.Time {
	// Exponential backoff: 2^attempts seconds
	// Attempt 1: 2s, Attempt 2: 4s, Attempt 3: 8s, Attempt 4: 16s, Attempt 5: 32s
	backoffSeconds := math.Pow(2, float64(e.Attempts))

	// Cap at 5 minutes
	if backoffSeconds > 300 {
		backoffSeconds = 300
	}

	return time.Now().Add(time.Duration(backoffSeconds) * time.Second)
}

// IsDueForRetry checks if entry is ready to retry
func (e *OutboxEntry) IsDueForRetry() bool {
	if e.Status != OutboxStatusFailedRetryable {
		return false
	}
	if e.NextRetryAt == nil {
		return true
	}
	return time.Now().After(*e.NextRetryAt)
}
