package detector

import (
	"context"
	"time"
)

// ChangeDetector represents a change detector that monitors external systems for changes
type ChangeDetector interface {
	// Start starts the change detector
	Start(ctx context.Context) error

	// Stop stops the change detector
	Stop() error

	// Subscribe subscribes a handler to receive change events
	Subscribe(handler ChangeHandler) error

	// Unsubscribe removes a handler from receiving change events
	Unsubscribe(handler ChangeHandler) error
}

// ChangeHandler handles change events from external systems
type ChangeHandler interface {
	// OnChange is called when a change event occurs
	OnChange(ctx context.Context, event ChangeEvent) error
}

// ChangeEvent represents a change event from an external system
type ChangeEvent struct {
	// Timestamp when the change occurred
	Timestamp time.Time `json:"timestamp"`

	// Source is the name of the external system that generated the change
	Source string `json:"source"`

	// Metadata contains additional information about the change
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ChangeHandlerFunc is a function adapter for ChangeHandler interface
type ChangeHandlerFunc func(ctx context.Context, event ChangeEvent) error

// OnChange implements ChangeHandler interface
func (f ChangeHandlerFunc) OnChange(ctx context.Context, event ChangeEvent) error {
	return f(ctx, event)
}
