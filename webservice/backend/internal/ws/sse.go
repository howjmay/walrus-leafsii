package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/leafsii/leafsii-backend/internal/store"
	"go.uber.org/zap"
)

type SSEHandler struct {
	cache   *store.Cache
	logger  *zap.SugaredLogger
	request *http.Request // Store request for query parameter access
}

func NewSSEHandler(cache *store.Cache, logger *zap.SugaredLogger) *SSEHandler {
	return &SSEHandler{
		cache:  cache,
		logger: logger,
	}
}

func (h *SSEHandler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	// Store request for later use
	h.request = r

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Set configurable CORS origins - in production, this should be from config
	origin := r.Header.Get("Origin")
	allowedOrigins := []string{
		"http://localhost:3000",
		"http://localhost:5173", // Vite dev server
		"https://app.leafsii.com",
	}

	corsOrigin := ""
	for _, allowed := range allowedOrigins {
		if origin == allowed {
			corsOrigin = allowed
			break
		}
	}

	if corsOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", corsOrigin)
	}
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Parse query parameters for subscription topics
	topics := h.parseTopics(r)
	address := r.URL.Query().Get("address")

	h.logger.Debugw("SSE connection established", "topics", topics, "address", address)

	// Create context that cancels when client disconnects
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Subscribe to Redis channels
	channels := h.mapTopicsToChannels(topics, address)
	if len(channels) == 0 {
		// Default to protocol updates if no specific topics requested
		channels = []string{"fx:protocol:state"}
	}

	// Try Redis pubsub first
	pubsub := h.cache.Subscribe(ctx, channels...)
	if pubsub != nil {
		defer pubsub.Close()
		h.handleRedisPubSub(ctx, w, pubsub)
		return
	}

	// Fall back to in-memory pubsub if available
	if h.cache.IsInMemoryMode() {
		mockPubsub := h.cache.SubscribeInMemory(ctx, channels...)
		if mockPubsub != nil {
			defer mockPubsub.Close()
			h.logger.Debugw("Using in-memory PubSub for SSE", "channels", channels)
			h.handleMockPubSub(ctx, w, mockPubsub)
			return
		}
	}

	h.logger.Warnw("No PubSub available; SSE updates disabled for this connection")
	h.sendEvent(w, "connected", "SSE connection established (no pubsub)", nil)
	return
}

func (h *SSEHandler) parseTopics(r *http.Request) []string {
	topicsParam := r.URL.Query().Get("topics")
	if topicsParam == "" {
		return nil
	}
	return strings.Split(topicsParam, ",")
}

func (h *SSEHandler) getSymbolFromQuery(r *http.Request) string {
	return r.URL.Query().Get("symbol")
}

func (h *SSEHandler) mapTopicsToChannels(topics []string, address string) []string {
	channels := make([]string, 0)

	for _, topic := range topics {
		switch topic {
		case "protocol", "protocol_state":
			channels = append(channels, "fx:protocol:state")
		case "sp", "stability_pool":
			channels = append(channels, "fx:sp:index")
		case "rebalance":
			channels = append(channels, "fx:events:REBALANCE")
		case "price":
			// Price topic requires symbol parameter
			symbol := h.getSymbolFromQuery(h.request)
			if symbol != "" {
				channels = append(channels, fmt.Sprintf("fx:oracle:price:%s", strings.ToUpper(symbol)))
			} else {
				// Default to FTOKEN if no symbol specified
				channels = append(channels, "fx:oracle:price:FTOKEN")
			}
		case "events":
			channels = append(channels,
				"fx:events:MINT",
				"fx:events:REDEEM",
				"fx:events:STAKE",
				"fx:events:UNSTAKE",
				"fx:events:CLAIM",
				"fx:events:REBALANCE",
			)
		}
	}

	// Add user-specific channel if address provided
	if address != "" {
		channels = append(channels, fmt.Sprintf("fx:user:%s", address))
	}

	return channels
}

func (h *SSEHandler) channelToEventType(channel string) string {
	switch {
	case channel == "fx:protocol:state":
		return "protocol_update"
	case channel == "fx:sp:index":
		return "sp_update"
	case strings.HasPrefix(channel, "fx:oracle:price:"):
		return "price_update"
	case strings.HasPrefix(channel, "fx:events:"):
		eventType := strings.TrimPrefix(channel, "fx:events:")
		return strings.ToLower(eventType) + "_event"
	case strings.HasPrefix(channel, "fx:user:"):
		return "user_update"
	default:
		return "update"
	}
}

func (h *SSEHandler) sendEvent(w http.ResponseWriter, eventType, id string, data interface{}) {
	if data != nil {
		dataBytes, err := json.Marshal(data)
		if err != nil {
			h.logger.Errorw("Failed to marshal SSE data", "error", err)
			return
		}
		fmt.Fprintf(w, "event: %s\n", eventType)
		fmt.Fprintf(w, "id: %s\n", id)
		fmt.Fprintf(w, "data: %s\n\n", dataBytes)
	} else {
		fmt.Fprintf(w, "event: %s\n", eventType)
		fmt.Fprintf(w, "id: %s\n", id)
		fmt.Fprintf(w, "data: {}\n\n")
	}

	// Flush the data to the client
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// handleRedisPubSub handles Redis pubsub messages for SSE
func (h *SSEHandler) handleRedisPubSub(ctx context.Context, w http.ResponseWriter, pubsub *redis.PubSub) {
	// Send initial heartbeat
	h.sendEvent(w, "connected", "SSE connection established", nil)

	// Start heartbeat routine
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	// Listen for messages
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			h.logger.Debugw("SSE client disconnected")
			return

		case <-heartbeat.C:
			h.sendEvent(w, "heartbeat", "ping", map[string]interface{}{
				"timestamp": time.Now().Unix(),
			})

		case msg := <-ch:
			if msg == nil {
				continue
			}

			h.logger.Debugw("Sending SSE message", "channel", msg.Channel)

			// Parse message data
			var data interface{}
			if err := json.Unmarshal([]byte(msg.Payload), &data); err != nil {
				h.logger.Warnw("Failed to parse message payload", "error", err)
				continue
			}

			// Send SSE event
			eventType := h.channelToEventType(msg.Channel)
			h.sendEvent(w, eventType, msg.Channel, data)
		}
	}
}

// handleMockPubSub handles in-memory pubsub messages for SSE
func (h *SSEHandler) handleMockPubSub(ctx context.Context, w http.ResponseWriter, mockPubsub *store.MockPubSub) {
	// Send initial heartbeat
	h.sendEvent(w, "connected", "SSE connection established (in-memory)", nil)

	// Start heartbeat routine
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	// Listen for messages
	ch := mockPubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			h.logger.Debugw("SSE client disconnected")
			return

		case <-heartbeat.C:
			h.sendEvent(w, "heartbeat", "ping", map[string]interface{}{
				"timestamp": time.Now().Unix(),
			})

		case msg := <-ch:
			if msg == nil {
				continue
			}

			h.logger.Debugw("Sending SSE message", "channel", msg.Channel)

			// Parse message data
			var data interface{}
			if err := json.Unmarshal([]byte(msg.Payload), &data); err != nil {
				h.logger.Warnw("Failed to parse message payload", "error", err)
				continue
			}

			// Send SSE event
			eventType := h.channelToEventType(msg.Channel)
			h.sendEvent(w, eventType, msg.Channel, data)
		}
	}
}
