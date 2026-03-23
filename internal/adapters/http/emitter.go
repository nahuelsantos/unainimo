package httphandler

import (
	"sync"

	"github.com/google/uuid"
	"github.com/unanimo-ai/unanimo/internal/ports"
)

// InMemoryEventEmitter is an in-process SSE broadcaster
type InMemoryEventEmitter struct {
	mu          sync.RWMutex
	subscribers map[string]map[string]chan ports.Event // gameID -> subscriberID -> channel
}

// NewInMemoryEventEmitter creates a new event emitter
func NewInMemoryEventEmitter() *InMemoryEventEmitter {
	return &InMemoryEventEmitter{
		subscribers: make(map[string]map[string]chan ports.Event),
	}
}

// Emit sends an event to all subscribers of the given game
func (e *InMemoryEventEmitter) Emit(gameID string, event ports.Event) {
	e.mu.RLock()
	subs, ok := e.subscribers[gameID]
	if !ok {
		e.mu.RUnlock()
		return
	}
	// Copy channels to avoid holding lock during send
	channels := make([]chan ports.Event, 0, len(subs))
	for _, ch := range subs {
		channels = append(channels, ch)
	}
	e.mu.RUnlock()

	for _, ch := range channels {
		select {
		case ch <- event:
		default:
			// Drop event if channel is full — non-blocking
		}
	}
}

// Subscribe registers a new subscriber and returns their channel and subscriber ID
func (e *InMemoryEventEmitter) Subscribe(gameID string) (<-chan ports.Event, string) {
	subID := uuid.New().String()
	ch := make(chan ports.Event, 64)

	e.mu.Lock()
	if e.subscribers[gameID] == nil {
		e.subscribers[gameID] = make(map[string]chan ports.Event)
	}
	e.subscribers[gameID][subID] = ch
	e.mu.Unlock()

	return ch, subID
}

// Unsubscribe removes a subscriber and closes their channel
func (e *InMemoryEventEmitter) Unsubscribe(gameID string, subscriberID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if subs, ok := e.subscribers[gameID]; ok {
		if ch, ok := subs[subscriberID]; ok {
			close(ch)
			delete(subs, subscriberID)
		}
		if len(subs) == 0 {
			delete(e.subscribers, gameID)
		}
	}
}
