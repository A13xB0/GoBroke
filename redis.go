// Package GoBroke provides Redis integration for high availability message routing.
package GoBroke

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/types"
	"github.com/redis/go-redis/v9"
)

// RedisConfig holds configuration for Redis integration.
type RedisConfig struct {
	Enabled     bool
	Client      *redis.Client // Optional existing Redis client
	ChannelName string
	InstanceID  string // Unique identifier for this GoBroke instance
}

// redisMessage represents a message that will be serialized and sent through Redis.
type redisMessage struct {
	InstanceID   string   // Source instance ID to prevent message loops
	MessageUUID  string   // Original message UUID
	ToClientIDs  []string // Target client UUIDs
	ToLogic      []types.LogicName
	FromClientID string // Source client UUID, if applicable
	FromLogic    types.LogicName
	MessageRaw   []byte
	Metadata     map[string]any
	Tags         map[string]interface{}
}

// redisClient manages the Redis connection and message handling.
type redisClient struct {
	client      *redis.Client
	config      RedisConfig
	broke       *Broke
	ctx         context.Context
	mu          sync.RWMutex
	clientCache map[string]bool // Cache of client IDs known to be on other instances
	stopTicker  chan struct{}   // Channel to stop the heartbeat ticker
}

// newRedisClient creates a new Redis client with the provided configuration.
func newRedisClient(config RedisConfig, broke *Broke, ctx context.Context) (*redisClient, error) {
	if !config.Enabled {
		return nil, nil
	}

	// Set default values if not provided
	if config.ChannelName == "" {
		config.ChannelName = "gobroke:messages"
	}

	if config.InstanceID == "" {
		config.InstanceID = fmt.Sprintf("gobroke:%d", time.Now().UnixNano())
	}

	var client *redis.Client

	// Use provided client or create a new one
	if config.Client != nil {
		client = config.Client
	} else {
		return nil, fmt.Errorf("Redis client must be provided")
	}

	// Test connection
	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	rc := &redisClient{
		client:      client,
		config:      config,
		broke:       broke,
		ctx:         ctx,
		clientCache: make(map[string]bool),
		stopTicker:  make(chan struct{}),
	}

	// Start subscription in a goroutine
	go rc.subscribe()

	// Start the client heartbeat ticker to update last message times
	go rc.startClientHeartbeat()

	return rc, nil
}

// subscribe listens for messages from other GoBroke instances.
func (rc *redisClient) subscribe() {
	pubsub := rc.client.Subscribe(rc.ctx, rc.config.ChannelName)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for msg := range ch {
		rc.handleRedisMessage(msg.Payload)
	}
}

// handleRedisMessage processes a message received from Redis.
func (rc *redisClient) handleRedisMessage(payload string) {
	var rm redisMessage
	if err := json.Unmarshal([]byte(payload), &rm); err != nil {
		// Log error and continue
		fmt.Printf("Error unmarshaling Redis message: %v\n", err)
		return
	}

	// Ignore messages from this instance to prevent loops
	if rm.InstanceID == rc.config.InstanceID {
		return
	}

	// Convert redisMessage back to types.Message
	message := types.Message{
		UUID:       rm.MessageUUID,
		MessageRaw: rm.MessageRaw,
		ToLogic:    rm.ToLogic,
		Metadata:   rm.Metadata,
		Tags:       rm.Tags,
		State:      types.ACCEPTED,
	}

	// Set FromClient if applicable
	if rm.FromClientID != "" {
		client, err := rc.broke.GetClient(rm.FromClientID)
		if err == nil {
			message.FromClient = client
		}
	}

	// Set FromLogic if applicable
	if rm.FromLogic != "" {
		message.FromLogic = rm.FromLogic
	}

	// Resolve ToClient references
	for _, clientID := range rm.ToClientIDs {
		client, err := rc.broke.GetClient(clientID)
		if err == nil {
			message.ToClient = append(message.ToClient, client)
		}
	}

	// Process the message
	rc.broke.SendMessage(message)
}

// publishMessage sends a message to other GoBroke instances via Redis.
func (rc *redisClient) publishMessage(message types.Message) error {
	if !rc.config.Enabled {
		return nil
	}

	// Extract client IDs for serialization
	toClientIDs := make([]string, 0, len(message.ToClient))
	for _, client := range message.ToClient {
		toClientIDs = append(toClientIDs, client.GetUUID())
	}

	// Create Redis message
	rm := redisMessage{
		InstanceID:  rc.config.InstanceID,
		MessageUUID: message.UUID,
		ToClientIDs: toClientIDs,
		ToLogic:     message.ToLogic,
		MessageRaw:  message.MessageRaw,
		Metadata:    message.Metadata,
		Tags:        message.Tags,
	}

	// Set client ID if message is from a client
	if message.FromClient != nil {
		rm.FromClientID = message.FromClient.GetUUID()
	}

	// Set logic name if message is from logic
	if message.FromLogic != "" {
		rm.FromLogic = message.FromLogic
	}

	// Serialize and publish
	payload, err := json.Marshal(rm)
	if err != nil {
		return fmt.Errorf("error marshaling message for Redis: %w", err)
	}

	return rc.client.Publish(rc.ctx, rc.config.ChannelName, payload).Err()
}

// isClientOnOtherInstance checks if a client is available on another instance.
func (rc *redisClient) isClientOnOtherInstance(clientID string) bool {
	if !rc.config.Enabled {
		return false
	}

	// Check cache first
	rc.mu.RLock()
	if _, found := rc.clientCache[clientID]; found {
		rc.mu.RUnlock()
		return true
	}
	rc.mu.RUnlock()

	// Check Redis for client presence
	key := fmt.Sprintf("gobroke:client:%s", clientID)
	exists, err := rc.client.Exists(rc.ctx, key).Result()
	if err != nil {
		return false
	}

	if exists > 0 {
		// Update cache
		rc.mu.Lock()
		rc.clientCache[clientID] = true
		rc.mu.Unlock()
		return true
	}

	return false
}

// registerClientInRedis registers a client in Redis for discovery by other instances.
func (rc *redisClient) registerClientInRedis(client *clients.Client) error {
	if !rc.config.Enabled {
		return nil
	}

	key := fmt.Sprintf("gobroke:client:%s", client.GetUUID())
	// Store instance ID with the client
	return rc.client.Set(rc.ctx, key, rc.config.InstanceID, 24*time.Hour).Err()
}

// unregisterClientFromRedis removes a client from Redis when it disconnects.
func (rc *redisClient) unregisterClientFromRedis(client *clients.Client) error {
	if !rc.config.Enabled {
		return nil
	}

	key := fmt.Sprintf("gobroke:client:%s", client.GetUUID())
	return rc.client.Del(rc.ctx, key).Err()
}

// getRemoteClientIDs returns a list of client IDs that are registered in Redis
// but not on this instance.
func (rc *redisClient) getRemoteClientIDs() ([]string, error) {
	if !rc.config.Enabled {
		return nil, nil
	}

	// Get all client keys from Redis
	keys, err := rc.client.Keys(rc.ctx, "gobroke:client:*").Result()
	if err != nil {
		return nil, fmt.Errorf("error getting client keys from Redis: %w", err)
	}

	// Extract client IDs from keys
	clientIDs := make([]string, 0, len(keys))
	for _, key := range keys {
		// Extract client ID from key (format: "gobroke:client:{uuid}")
		clientID := key[len("gobroke:client:"):]

		// Get instance ID for this client
		instanceID, err := rc.client.Get(rc.ctx, key).Result()
		if err != nil {
			// Skip if we can't get the instance ID
			continue
		}

		// Skip clients on this instance
		if instanceID == rc.config.InstanceID {
			continue
		}

		clientIDs = append(clientIDs, clientID)
	}

	return clientIDs, nil
}

// close closes the Redis client connection and stops the heartbeat ticker.
func (rc *redisClient) close() error {
	if !rc.config.Enabled || rc.client == nil {
		return nil
	}

	// Stop the heartbeat ticker
	close(rc.stopTicker)

	return rc.client.Close()
}

// startClientHeartbeat starts a ticker that updates client last message times in Redis every second.
func (rc *redisClient) startClientHeartbeat() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Update last message time for all local clients
			rc.updateAllClientLastMessageTimes()
		case <-rc.stopTicker:
			return
		case <-rc.ctx.Done():
			return
		}
	}
}

// updateAllClientLastMessageTimes updates the last message time in Redis for all local clients.
func (rc *redisClient) updateAllClientLastMessageTimes() {
	// Get all local clients
	localClients := rc.broke.GetAllClients(true) // true = local clients only

	for _, client := range localClients {
		// Update the client's last message time in Redis
		rc.updateClientLastMessageTime(client)
	}
}

// updateClientLastMessageTime updates the last message time in Redis for a specific client.
func (rc *redisClient) updateClientLastMessageTime(client *clients.Client) {
	if !rc.config.Enabled {
		return
	}

	// Get the client's last message time
	lastMsgTime := client.GetLastMessage()

	// Skip if the time is zero (client hasn't sent any messages yet)
	if lastMsgTime.IsZero() {
		return
	}

	// Convert time to Unix timestamp for storage
	timestamp := lastMsgTime.UnixNano()

	// Store the timestamp in Redis
	key := fmt.Sprintf("gobroke:client:%s:lastmsg", client.GetUUID())
	err := rc.client.Set(rc.ctx, key, timestamp, 24*time.Hour).Err()
	if err != nil {
		// Log error but continue
		fmt.Printf("Error updating client last message time in Redis: %v\n", err)
	}
}

// getClientLastMessageTime retrieves the last message time from Redis for a client.
// Returns a zero time if the client has no recorded last message time.
func (rc *redisClient) getClientLastMessageTime(clientID string) time.Time {
	if !rc.config.Enabled {
		return time.Time{}
	}

	key := fmt.Sprintf("gobroke:client:%s:lastmsg", clientID)
	val, err := rc.client.Get(rc.ctx, key).Result()

	if err != nil {
		if err != redis.Nil {
			// Log error but continue
			fmt.Printf("Error getting client last message time from Redis: %v\n", err)
		}
		return time.Time{}
	}

	// Parse the timestamp
	timestamp, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		fmt.Printf("Error parsing client last message timestamp: %v\n", err)
		return time.Time{}
	}

	return time.Unix(0, timestamp)
}
