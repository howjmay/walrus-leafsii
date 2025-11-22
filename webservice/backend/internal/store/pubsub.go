package store

import (
	"context"
	"sync"
)

// MockMessage mimics redis.Message for in-memory pubsub
type MockMessage struct {
	Channel string
	Payload string
}

// MockPubSub mimics redis.PubSub for in-memory implementation
type MockPubSub struct {
	channels map[string]bool
	msgChan  chan *MockMessage
	closeCh  chan struct{}
	closed   bool
	mu       sync.RWMutex
}

// NewMockPubSub creates a new mock pubsub instance
func NewMockPubSub(channels []string) *MockPubSub {
	channelMap := make(map[string]bool)
	for _, ch := range channels {
		channelMap[ch] = true
	}
	
	return &MockPubSub{
		channels: channelMap,
		msgChan:  make(chan *MockMessage, 100), // Buffered channel
		closeCh:  make(chan struct{}),
	}
}

// Channel returns the message channel
func (m *MockPubSub) Channel() <-chan *MockMessage {
	return m.msgChan
}

// Close closes the pubsub connection
func (m *MockPubSub) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if !m.closed {
		m.closed = true
		close(m.closeCh)
		close(m.msgChan)
	}
	return nil
}

// isClosed checks if the pubsub is closed (must hold lock)
func (m *MockPubSub) isClosed() bool {
	return m.closed
}

// isSubscribedTo checks if subscribed to a channel
func (m *MockPubSub) isSubscribedTo(channel string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.closed {
		return false
	}
	return m.channels[channel]
}

// sendMessage sends a message to subscribers (non-blocking)
func (m *MockPubSub) sendMessage(msg *MockMessage) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.closed || !m.channels[msg.Channel] {
		return
	}
	
	// Non-blocking send
	select {
	case m.msgChan <- msg:
	default:
		// Channel is full, drop message to prevent blocking
	}
}

// PubSubHub manages all mock pubsub subscriptions
type PubSubHub struct {
	subscribers map[string][]*MockPubSub // channel -> list of subscribers
	mu          sync.RWMutex
}

// NewPubSubHub creates a new pubsub hub
func NewPubSubHub() *PubSubHub {
	return &PubSubHub{
		subscribers: make(map[string][]*MockPubSub),
	}
}

// Subscribe creates a new mock pubsub for the given channels
func (h *PubSubHub) Subscribe(ctx context.Context, channels ...string) *MockPubSub {
	pubsub := NewMockPubSub(channels)
	
	h.mu.Lock()
	defer h.mu.Unlock()
	
	// Register this pubsub for each channel
	for _, channel := range channels {
		h.subscribers[channel] = append(h.subscribers[channel], pubsub)
	}
	
	// Start cleanup goroutine
	go func() {
		select {
		case <-ctx.Done():
			pubsub.Close()
		case <-pubsub.closeCh:
		}
		
		// Clean up from subscribers list
		h.mu.Lock()
		defer h.mu.Unlock()
		
		for _, channel := range channels {
			subscribers := h.subscribers[channel]
			for i, sub := range subscribers {
				if sub == pubsub {
					// Remove this subscriber
					h.subscribers[channel] = append(subscribers[:i], subscribers[i+1:]...)
					break
				}
			}
			// Clean up empty channels
			if len(h.subscribers[channel]) == 0 {
				delete(h.subscribers, channel)
			}
		}
	}()
	
	return pubsub
}

// Publish sends a message to all subscribers of a channel
func (h *PubSubHub) Publish(channel, payload string) {
	h.mu.RLock()
	subscribers := make([]*MockPubSub, len(h.subscribers[channel]))
	copy(subscribers, h.subscribers[channel])
	h.mu.RUnlock()
	
	if len(subscribers) == 0 {
		return
	}
	
	msg := &MockMessage{
		Channel: channel,
		Payload: payload,
	}
	
	// Send to all subscribers
	for _, sub := range subscribers {
		if !sub.isClosed() {
			sub.sendMessage(msg)
		}
	}
}