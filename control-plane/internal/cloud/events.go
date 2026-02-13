package cloud

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

// EventBus provides publish/subscribe for cloud events.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string]chan CloudEvent
	buffer      []CloudEvent
	bufferSize  int
}

// NewEventBus creates a new event bus with the given buffer size.
func NewEventBus(bufferSize int) *EventBus {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &EventBus{
		subscribers: make(map[string]chan CloudEvent),
		buffer:      make([]CloudEvent, 0, bufferSize),
		bufferSize:  bufferSize,
	}
}

// Publish sends an event to all subscribers.
func (eb *EventBus) Publish(event CloudEvent) {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	eb.mu.Lock()
	// Add to ring buffer.
	if len(eb.buffer) >= eb.bufferSize {
		eb.buffer = eb.buffer[1:]
	}
	eb.buffer = append(eb.buffer, event)

	// Copy subscribers to avoid holding lock during send.
	subs := make(map[string]chan CloudEvent, len(eb.subscribers))
	for k, v := range eb.subscribers {
		subs[k] = v
	}
	eb.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// Drop if subscriber is slow.
		}
	}
}

// Subscribe returns a channel for receiving events.
func (eb *EventBus) Subscribe() (string, <-chan CloudEvent) {
	id := uuid.New().String()
	ch := make(chan CloudEvent, 32)

	eb.mu.Lock()
	eb.subscribers[id] = ch
	eb.mu.Unlock()

	return id, ch
}

// Unsubscribe removes a subscriber.
func (eb *EventBus) Unsubscribe(id string) {
	eb.mu.Lock()
	if ch, ok := eb.subscribers[id]; ok {
		close(ch)
		delete(eb.subscribers, id)
	}
	eb.mu.Unlock()
}

// Recent returns the most recent events from the buffer.
func (eb *EventBus) Recent(limit int) []CloudEvent {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	if limit <= 0 || limit > len(eb.buffer) {
		limit = len(eb.buffer)
	}

	start := len(eb.buffer) - limit
	result := make([]CloudEvent, limit)
	copy(result, eb.buffer[start:])
	return result
}

// EmitInstanceEvent is a convenience method for publishing instance lifecycle events.
func (eb *EventBus) EmitInstanceEvent(eventType, instanceID string, data interface{}) {
	var rawData json.RawMessage
	if data != nil {
		if b, err := json.Marshal(data); err == nil {
			rawData = b
		}
	}

	eb.Publish(CloudEvent{
		Type:       eventType,
		InstanceID: instanceID,
		Timestamp:  time.Now().UTC(),
		Data:       rawData,
	})
}

// Common event types.
const (
	EventInstanceRequested    = "instance.requested"
	EventInstanceProvisioning = "instance.provisioning"
	EventInstanceRunning      = "instance.running"
	EventInstanceStopped      = "instance.stopped"
	EventInstanceTerminated   = "instance.terminated"
	EventInstanceFailed       = "instance.failed"
	EventInstanceConnected    = "instance.connected" // agent registered
	EventHostAllocated        = "host.allocated"
	EventHostReleased         = "host.released"
)
