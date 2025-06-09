package patterns

// Subject represents an observable subject
type Subject interface {
	// Subscribe registers an observer
	Subscribe(observer interface{}) error
	// Unsubscribe removes an observer
	Unsubscribe(observer interface{}) error
	// Notify notifies all observers
	Notify(event interface{})
}
