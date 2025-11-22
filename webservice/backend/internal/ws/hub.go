package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/gorilla/websocket"
	"github.com/leafsii/leafsii-backend/internal/metrics"
	"github.com/leafsii/leafsii-backend/internal/store"
	"go.uber.org/zap"
)

type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	cache      *store.Cache
	logger     *zap.SugaredLogger
	metrics    *metrics.Metrics
	mu         sync.RWMutex
}

type Client struct {
	hub        *Hub
	conn       *websocket.Conn
	send       chan []byte
	topics     map[string]bool
	address    string // User address for user-specific updates
	lastActive time.Time
}

type Message struct {
	Type      string          `json:"type"`
	Topic     string          `json:"topic"`
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
}

type WSSubscriptionRequest struct {
	Type    string   `json:"type"`
	Topics  []string `json:"topics"`
	Address string   `json:"address,omitempty"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Check allowed origins - in production, this should be configurable
		origin := r.Header.Get("Origin")
		allowedOrigins := []string{
			"http://localhost:3000",
			"http://localhost:5173", // Vite dev server
			"https://app.leafsii.com",
		}

		for _, allowed := range allowedOrigins {
			if origin == allowed {
				return true
			}
		}

		// Allow same-origin requests (when Origin header is empty)
		return origin == ""
	},
}

func NewHub(cache *store.Cache, logger *zap.SugaredLogger, metrics *metrics.Metrics) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte),
		cache:      cache,
		logger:     logger,
		metrics:    metrics,
	}
}

func (h *Hub) Run(ctx context.Context) {
	// Start Redis subscription for real-time updates
	go h.startRedisSubscription(ctx)

	// Start client cleanup routine
	go h.startClientCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			h.logger.Infow("WebSocket hub shutting down")
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.metrics.IncrementConnections(ctx)
			h.logger.Debugw("Client registered", "address", client.address, "topics", client.topics)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			h.metrics.DecrementConnections(ctx)
			h.logger.Debugw("Client unregistered", "address", client.address)

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					delete(h.clients, client)
					close(client.send)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) startRedisSubscription(ctx context.Context) {
	// Subscribe to all event channels
	channels := []string{
		"fx:protocol:state",
		"fx:sp:index",
		"fx:events:REBALANCE",
		"fx:events:MINT",
		"fx:events:REDEEM",
		"fx:events:STAKE",
		"fx:events:UNSTAKE",
		"fx:events:CLAIM",
	}

	// Try Redis pubsub first
	pubsub := h.cache.Subscribe(ctx, channels...)
	if pubsub != nil {
		defer pubsub.Close()
		h.handleRedisPubSubMessages(ctx, pubsub)
		return
	}

	// Fall back to in-memory pubsub if available
	if h.cache.IsInMemoryMode() {
		mockPubsub := h.cache.SubscribeInMemory(ctx, channels...)
		if mockPubsub != nil {
			defer mockPubsub.Close()
			h.logger.Debugw("Using in-memory PubSub for WebSocket hub", "channels", channels)
			h.handleMockPubSubMessages(ctx, mockPubsub)
			return
		}
	}

	h.logger.Warnw("No PubSub available; skipping WebSocket subscriptions")
}

func (h *Hub) handleRedisMessage(ctx context.Context, msg *redis.Message) {
	h.logger.Debugw("Received Redis message", "channel", msg.Channel, "payload", msg.Payload)

	// Create WebSocket message
	wsMessage := Message{
		Type:      "update",
		Topic:     msg.Channel,
		Data:      json.RawMessage(msg.Payload),
		Timestamp: time.Now().Unix(),
	}

	messageBytes, err := json.Marshal(wsMessage)
	if err != nil {
		h.logger.Errorw("Failed to marshal WebSocket message", "error", err)
		return
	}

	// Broadcast to relevant clients
	h.broadcastToClients(messageBytes, msg.Channel)
}

func (h *Hub) broadcastToClients(message []byte, topic string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		// Check if client is subscribed to this topic
		if client.isSubscribed(topic) {
			select {
			case client.send <- message:
			default:
				// Client is slow or disconnected
				delete(h.clients, client)
				close(client.send)
			}
		}
	}
}

func (h *Hub) startClientCleanup(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.cleanupInactiveClients()
		}
	}
}

func (h *Hub) cleanupInactiveClients() {
	h.mu.Lock()
	defer h.mu.Unlock()

	cutoff := time.Now().Add(-60 * time.Second) // 1 minute timeout

	for client := range h.clients {
		if client.lastActive.Before(cutoff) {
			delete(h.clients, client)
			close(client.send)
			h.logger.Debugw("Cleaned up inactive client", "address", client.address)
		}
	}
}

// WebSocket endpoint handler
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Errorw("WebSocket upgrade failed", "error", err)
		return
	}

	client := &Client{
		hub:        h,
		conn:       conn,
		send:       make(chan []byte, 256),
		topics:     make(map[string]bool),
		lastActive: time.Now(),
	}

	client.hub.register <- client

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.hub.logger.Errorw("WebSocket error", "error", err)
			}
			break
		}

		c.lastActive = time.Now()
		c.handleMessage(message)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(message []byte) {
	var sub WSSubscriptionRequest
	if err := json.Unmarshal(message, &sub); err != nil {
		c.hub.logger.Warnw("Invalid subscription message", "error", err)
		return
	}

	switch sub.Type {
	case "subscribe":
		for _, topic := range sub.Topics {
			c.topics[topic] = true
		}
		if sub.Address != "" {
			c.address = sub.Address
			// Subscribe to user-specific updates
			userTopic := fmt.Sprintf("fx:user:%s", sub.Address)
			c.topics[userTopic] = true
		}
		c.hub.logger.Debugw("Client subscribed to topics", "topics", sub.Topics, "address", sub.Address)

	case "unsubscribe":
		for _, topic := range sub.Topics {
			delete(c.topics, topic)
		}
		c.hub.logger.Debugw("Client unsubscribed from topics", "topics", sub.Topics)
	}
}

func (c *Client) isSubscribed(topic string) bool {
	// Check exact match
	if c.topics[topic] {
		return true
	}

	// Check pattern matches (simplified)
	if c.topics["fx:protocol:*"] && topic == "fx:protocol:state" {
		return true
	}
	if c.topics["fx:sp:*"] && topic == "fx:sp:index" {
		return true
	}
	if c.topics["fx:events:*"] && topic[:10] == "fx:events:" {
		return true
	}

	return false
}

// handleRedisPubSubMessages handles Redis pubsub messages
func (h *Hub) handleRedisPubSubMessages(ctx context.Context, pubsub *redis.PubSub) {
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			h.handleRedisMessage(ctx, msg)
		}
	}
}

// handleMockPubSubMessages handles in-memory pubsub messages
func (h *Hub) handleMockPubSubMessages(ctx context.Context, mockPubsub *store.MockPubSub) {
	ch := mockPubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if msg != nil {
				h.handleMockMessage(ctx, msg)
			}
		}
	}
}

// handleMockMessage processes in-memory pubsub messages
func (h *Hub) handleMockMessage(ctx context.Context, msg *store.MockMessage) {
	h.logger.Debugw("Received in-memory message", "channel", msg.Channel, "payload", msg.Payload)

	// Create WebSocket message - same format as Redis
	wsMessage := Message{
		Type:      "update",
		Topic:     msg.Channel,
		Data:      json.RawMessage(msg.Payload),
		Timestamp: time.Now().Unix(),
	}

	messageBytes, err := json.Marshal(wsMessage)
	if err != nil {
		h.logger.Errorw("Failed to marshal WebSocket message", "error", err)
		return
	}

	// Broadcast to relevant clients
	h.broadcastToClients(messageBytes, msg.Channel)
}
