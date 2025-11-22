package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestInMemoryPubSub(t *testing.T) {
	// Create a cache in in-memory mode by passing an invalid Redis address
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()
	
	cache, err := NewCache("invalid:6379", sugar, nil)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()
	
	// Verify it's in in-memory mode
	if !cache.IsInMemoryMode() {
		t.Fatal("Expected cache to be in in-memory mode")
	}
	
	// Test basic key-value operations
	ctx := context.Background()
	testKey := "test:key"
	testValue := map[string]interface{}{
		"message": "hello world",
		"timestamp": time.Now().Unix(),
	}
	
	// Set a value
	err = cache.Set(ctx, testKey, testValue, 1*time.Minute)
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}
	
	// Get the value back
	var retrieved map[string]interface{}
	err = cache.Get(ctx, testKey, &retrieved)
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}
	
	if retrieved["message"] != testValue["message"] {
		t.Errorf("Expected %v, got %v", testValue["message"], retrieved["message"])
	}
	
	// Test pubsub functionality
	channel := "test:channel"
	message := map[string]string{
		"event": "test_event",
		"data":  "test data",
	}
	
	// Subscribe to the channel
	mockPubsub := cache.SubscribeInMemory(ctx, channel)
	if mockPubsub == nil {
		t.Fatal("Expected mock pubsub to be available")
	}
	defer mockPubsub.Close()
	
	// Publish a message
	err = cache.Publish(ctx, channel, message)
	if err != nil {
		t.Fatalf("Failed to publish message: %v", err)
	}
	
	// Receive the message (with timeout)
	select {
	case msg := <-mockPubsub.Channel():
		if msg == nil {
			t.Fatal("Received nil message")
		}
		if msg.Channel != channel {
			t.Errorf("Expected channel %s, got %s", channel, msg.Channel)
		}
		
		// Parse the message payload
		var receivedMessage map[string]string
		err = json.Unmarshal([]byte(msg.Payload), &receivedMessage)
		if err != nil {
			t.Fatalf("Failed to unmarshal message: %v", err)
		}
		
		if receivedMessage["event"] != message["event"] {
			t.Errorf("Expected event %s, got %s", message["event"], receivedMessage["event"])
		}
		
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for pubsub message")
	}
	
	t.Log("In-memory PubSub test completed successfully")
}