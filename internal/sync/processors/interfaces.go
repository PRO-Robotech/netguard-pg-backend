package processors

import (
	"context"

	"netguard-pg-backend/internal/sync/detector"
)

// EntityProcessor processes changes for a specific entity type
type EntityProcessor interface {
	// GetEntityType returns the entity type that this processor handles
	GetEntityType() string

	// ProcessChanges processes changes for entities of this type
	ProcessChanges(ctx context.Context, event detector.ChangeEvent) error
}

// ProcessResult contains the result of processing changes
type ProcessResult struct {
	// ProcessedCount is the number of entities successfully processed
	ProcessedCount int `json:"processed_count"`

	// ErrorCount is the number of entities that failed to process
	ErrorCount int `json:"error_count"`

	// Errors contains the errors that occurred during processing
	Errors []error `json:"-"`

	// Details contains additional information about the processing
	Details map[string]interface{} `json:"details,omitempty"`
}

// EntityProcessorFunc is a function adapter for EntityProcessor interface
type EntityProcessorFunc struct {
	entityType  string
	processFunc func(ctx context.Context, event detector.ChangeEvent) error
}

// NewEntityProcessorFunc creates a new EntityProcessorFunc
func NewEntityProcessorFunc(entityType string, processFunc func(ctx context.Context, event detector.ChangeEvent) error) EntityProcessor {
	return &EntityProcessorFunc{
		entityType:  entityType,
		processFunc: processFunc,
	}
}

// GetEntityType implements EntityProcessor interface
func (f *EntityProcessorFunc) GetEntityType() string {
	return f.entityType
}

// ProcessChanges implements EntityProcessor interface
func (f *EntityProcessorFunc) ProcessChanges(ctx context.Context, event detector.ChangeEvent) error {
	return f.processFunc(ctx, event)
}

// AddError adds an error to the ProcessResult
func (r *ProcessResult) AddError(err error) {
	if err != nil {
		r.Errors = append(r.Errors, err)
		r.ErrorCount++
	}
}

// AddProcessed increments the processed count
func (r *ProcessResult) AddProcessed(count int) {
	r.ProcessedCount += count
}

// HasErrors returns true if there are any errors
func (r *ProcessResult) HasErrors() bool {
	return r.ErrorCount > 0
}

// SetDetail sets a detail value
func (r *ProcessResult) SetDetail(key string, value interface{}) {
	if r.Details == nil {
		r.Details = make(map[string]interface{})
	}
	r.Details[key] = value
}

// GetDetail gets a detail value
func (r *ProcessResult) GetDetail(key string) interface{} {
	if r.Details == nil {
		return nil
	}
	return r.Details[key]
}
