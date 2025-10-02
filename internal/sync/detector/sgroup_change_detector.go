package detector

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"netguard-pg-backend/internal/sync/interfaces"
)

// SGROUPChangeDetector detects changes in SGROUP system
type SGROUPChangeDetector struct {
	// client is the gateway to SGROUP system
	client interfaces.SGroupGateway

	// handlers stores registered change handlers
	handlers     map[string]ChangeHandler
	handlersLock sync.RWMutex

	// config holds detector configuration
	config SGROUPDetectorConfig

	// Control fields for lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// State
	started       bool
	lastUpdate    *timestamppb.Timestamp
	nextHandlerID int
}

// SGROUPDetectorConfig holds configuration for SGROUP detector
type SGROUPDetectorConfig struct {
	// ReconnectInterval is the interval to wait before reconnecting after failure
	ReconnectInterval time.Duration

	// MaxRetries is the maximum number of reconnection attempts (0 = unlimited)
	MaxRetries int

	// ChangeEventSource is the source name to use in ChangeEvent
	ChangeEventSource string
}

// NewSGROUPChangeDetector creates a new SGROUP change detector
func NewSGROUPChangeDetector(client interfaces.SGroupGateway, config SGROUPDetectorConfig) ChangeDetector {
	// Set default values
	if config.ReconnectInterval == 0 {
		config.ReconnectInterval = 5 * time.Second
	}
	if config.ChangeEventSource == "" {
		config.ChangeEventSource = "sgroup"
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &SGROUPChangeDetector{
		client:   client,
		handlers: make(map[string]ChangeHandler),
		config:   config,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start starts the SGROUP change detector
func (d *SGROUPChangeDetector) Start(ctx context.Context) error {
	d.handlersLock.Lock()
	defer d.handlersLock.Unlock()

	if d.started {
		return fmt.Errorf("SGROUP change detector is already started")
	}


	d.started = true

	// Start the monitoring goroutine
	d.wg.Add(1)
	go d.monitorChanges()

	return nil
}

// Stop stops the SGROUP change detector
func (d *SGROUPChangeDetector) Stop() error {
	d.handlersLock.Lock()
	defer d.handlersLock.Unlock()

	if !d.started {
		return nil
	}


	d.started = false
	d.cancel()
	d.wg.Wait()

	return nil
}

// Subscribe subscribes a handler to receive change events
func (d *SGROUPChangeDetector) Subscribe(handler ChangeHandler) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	d.handlersLock.Lock()
	defer d.handlersLock.Unlock()

	d.nextHandlerID++
	handlerID := fmt.Sprintf("handler-%d", d.nextHandlerID)
	d.handlers[handlerID] = handler


	return nil
}

// Unsubscribe removes a handler from receiving change events
func (d *SGROUPChangeDetector) Unsubscribe(handler ChangeHandler) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	d.handlersLock.Lock()
	defer d.handlersLock.Unlock()

	// For simplicity, remove the first handler found
	// In a production system, you might want to use handler IDs or references
	for id := range d.handlers {
		delete(d.handlers, id)
		break
	}

	return nil
}

// monitorChanges runs the main monitoring loop
func (d *SGROUPChangeDetector) monitorChanges() {
	defer d.wg.Done()


	retryCount := 0
	maxRetries := d.config.MaxRetries

	for {
		select {
		case <-d.ctx.Done():
			return
		default:
		}

		err := d.connectToStream()
		if err != nil {
			retryCount++

			// Check if we've exceeded max retries
			if maxRetries > 0 && retryCount >= maxRetries {
				return
			}

			// Wait before retrying
			select {
			case <-time.After(d.config.ReconnectInterval):
				continue
			case <-d.ctx.Done():
				return
			}
		}

		// Reset retry count on successful connection
		retryCount = 0
	}
}

// connectToStream connects to SGROUP stream and processes updates
func (d *SGROUPChangeDetector) connectToStream() error {

	timestamps, err := d.client.GetStatuses(d.ctx)
	if err != nil {
		return fmt.Errorf("failed to get statuses stream: %w", err)
	}


	// Process timestamps from the stream
	for {
		select {
		case <-d.ctx.Done():
			return nil
		case timestamp, ok := <-timestamps:
			if !ok {
				return fmt.Errorf("stream closed")
			}

			if timestamp != nil {

				// Check if this is a new update
				if d.isNewUpdate(timestamp) {
					d.lastUpdate = timestamp
					err := d.notifyHandlers(timestamp)
					if err != nil {
					}
				}
			}
		}
	}
}

// isNewUpdate checks if the timestamp represents a new update
func (d *SGROUPChangeDetector) isNewUpdate(timestamp *timestamppb.Timestamp) bool {
	if d.lastUpdate == nil {
		return true
	}

	newTime := timestamp.AsTime()
	lastTime := d.lastUpdate.AsTime()

	return newTime.After(lastTime)
}

// notifyHandlers notifies all registered handlers about a change
func (d *SGROUPChangeDetector) notifyHandlers(timestamp *timestamppb.Timestamp) error {
	d.handlersLock.RLock()
	handlers := make(map[string]ChangeHandler)
	for id, handler := range d.handlers {
		handlers[id] = handler
	}
	d.handlersLock.RUnlock()

	if len(handlers) == 0 {
		return nil
	}

	event := ChangeEvent{
		Timestamp: timestamp.AsTime(),
		Source:    d.config.ChangeEventSource,
		Metadata: map[string]interface{}{
			"sgroup_timestamp": timestamp,
		},
	}


	var errors []error
	for handlerID, handler := range handlers {
		err := handler.OnChange(d.ctx, event)
		if err != nil {
			errors = append(errors, fmt.Errorf("handler %s: %w", handlerID, err))
		} else {
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("some handlers failed: %v", errors)
	}

	return nil
}

// GetHandlerCount returns the number of registered handlers (for testing)
func (d *SGROUPChangeDetector) GetHandlerCount() int {
	d.handlersLock.RLock()
	defer d.handlersLock.RUnlock()
	return len(d.handlers)
}

// IsStarted returns true if the detector is started (for testing)
func (d *SGROUPChangeDetector) IsStarted() bool {
	d.handlersLock.RLock()
	defer d.handlersLock.RUnlock()
	return d.started
}

// GetLastUpdate returns the last update timestamp (for testing)
func (d *SGROUPChangeDetector) GetLastUpdate() *timestamppb.Timestamp {
	return d.lastUpdate
}
