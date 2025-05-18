// Package GoBroke provides Redis integration for high availability message routing.
package GoBroke

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/proto"
	"github.com/A13xB0/GoBroke/types"
	"github.com/redis/go-redis/v9"
	protobuf "google.golang.org/protobuf/proto"
)

// RedisConfig holds configuration for Redis integration.
type RedisConfig struct {
	Enabled     bool
	Client      *redis.Client // Optional existing Redis client
	ChannelName string
	InstanceID  string // Unique identifier for this GoBroke instance
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
	var rm proto.RedisMessage
	if err := protobuf.Unmarshal([]byte(payload), &rm); err != nil {
		// Log error and continue
		fmt.Printf("Error unmarshaling Redis message: %v\n", err)
		return
	}

	// Ignore messages from this instance to prevent loops
	if rm.InstanceId == rc.config.InstanceID {
		return
	}

	// Convert RedisMessage back to types.Message
	message := types.Message{
		UUID:       rm.MessageUuid,
		MessageRaw: rm.MessageRaw,
		ToLogic:    convertToLogicNames(rm.ToLogic),
		Metadata:   convertMetadata(rm.Metadata),
		Tags:       convertTags(rm.Tags),
		State:      types.ACCEPTED,
	}

	// Set FromClient if applicable
	if rm.FromClientId != "" {
		client, err := rc.broke.GetClient(rm.FromClientId)
		if err == nil {
			message.FromClient = client
		}
	}

	// Set FromLogic if applicable
	if rm.FromLogic != "" {
		message.FromLogic = types.LogicName(rm.FromLogic)
	}

	// Resolve ToClient references
	for _, clientID := range rm.ToClientIds {
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

	// Convert logic names to strings
	toLogic := make([]string, len(message.ToLogic))
	for i, logic := range message.ToLogic {
		toLogic[i] = string(logic)
	}

	// Create Redis message
	rm := &proto.RedisMessage{
		InstanceId:  rc.config.InstanceID,
		MessageUuid: message.UUID,
		ToClientIds: toClientIDs,
		ToLogic:     toLogic,
		MessageRaw:  message.MessageRaw,
		Metadata:    convertToByteMap(message.Metadata),
		Tags:        convertTagsToByteMap(message.Tags),
	}

	// Set client ID if message is from a client
	if message.FromClient != nil {
		rm.FromClientId = message.FromClient.GetUUID()
	}

	// Set logic name if message is from logic
	if message.FromLogic != "" {
		rm.FromLogic = string(message.FromLogic)
	}

	// Serialize and publish
	payload, err := protobuf.Marshal(rm)
	if err != nil {
		return fmt.Errorf("error marshaling message for Redis: %w", err)
	}

	return rc.client.Publish(rc.ctx, rc.config.ChannelName, payload).Err()
}

// Helper functions for converting between protobuf and types

// convertToLogicNames converts a slice of strings to a slice of LogicName
func convertToLogicNames(logicNames []string) []types.LogicName {
	result := make([]types.LogicName, len(logicNames))
	for i, name := range logicNames {
		result[i] = types.LogicName(name)
	}
	return result
}

// convertMetadata converts a map of string to []byte to a map of string to any
func convertMetadata(metadata map[string][]byte) map[string]any {
	if metadata == nil {
		return nil
	}
	result := make(map[string]any, len(metadata))
	for k, v := range metadata {
		// Try to detect if this is a string or binary data
		if isASCII(v) {
			// If it looks like a string, convert it to a string
			result[k] = string(v)
		} else {
			// Otherwise, keep it as binary data
			result[k] = v
		}
	}
	return result
}

// convertTags converts a map of string to []byte to a map of string to interface{}
func convertTags(tags map[string][]byte) map[string]interface{} {
	if tags == nil {
		return make(map[string]interface{})
	}
	result := make(map[string]interface{}, len(tags))
	for k, v := range tags {
		// Try to detect if this is a string or binary data
		if isASCII(v) {
			// If it looks like a string, convert it to a string
			result[k] = string(v)
		} else {
			// Otherwise, keep it as binary data
			result[k] = v
		}
	}
	return result
}

// convertToByteMap converts a map of string to any to a map of string to []byte
func convertToByteMap(metadata map[string]any) map[string][]byte {
	if metadata == nil {
		return nil
	}
	result := make(map[string][]byte, len(metadata))
	for k, v := range metadata {
		// Handle different types appropriately
		switch val := v.(type) {
		case string:
			result[k] = []byte(val)
		case []byte:
			result[k] = val
		default:
			// For other types, use a string representation
			result[k] = []byte(fmt.Sprintf("%v", v))
		}
	}
	return result
}

// convertTagsToByteMap converts a map of string to interface{} to a map of string to []byte
func convertTagsToByteMap(tags map[string]interface{}) map[string][]byte {
	if tags == nil {
		return nil
	}
	result := make(map[string][]byte, len(tags))
	for k, v := range tags {
		// Handle different types appropriately
		switch val := v.(type) {
		case string:
			result[k] = []byte(val)
		case []byte:
			result[k] = val
		default:
			// For other types, use a string representation
			result[k] = []byte(fmt.Sprintf("%v", v))
		}
	}
	return result
}

// isASCII checks if a byte slice contains only ASCII characters
func isASCII(data []byte) bool {
	// If it's empty, treat it as a string
	if len(data) == 0 {
		return true
	}

	// Check if it looks like a string (mostly printable ASCII characters)
	printableCount := 0
	for _, b := range data {
		if (b >= 32 && b <= 126) || b == '\n' || b == '\r' || b == '\t' {
			printableCount++
		}
	}

	// If more than 90% of characters are printable, consider it a string
	return float64(printableCount)/float64(len(data)) > 0.9
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

	clientID := client.GetUUID()

	// Remove client registration
	key := fmt.Sprintf("gobroke:client:%s", clientID)
	if err := rc.client.Del(rc.ctx, key).Err(); err != nil {
		return err
	}

	// Remove client last message time
	lastMsgKey := fmt.Sprintf("gobroke:client_lastmsg:%s", clientID)
	return rc.client.Del(rc.ctx, lastMsgKey).Err()
}

// unregisterClientFromRedisByID removes a client from Redis by its ID.
// This is used when removing a client that exists on another replica.
func (rc *redisClient) unregisterClientFromRedisByID(clientID string) error {
	if !rc.config.Enabled {
		return nil
	}

	// Remove client registration
	key := fmt.Sprintf("gobroke:client:%s", clientID)
	if err := rc.client.Del(rc.ctx, key).Err(); err != nil {
		return err
	}

	// Remove client last message time
	lastMsgKey := fmt.Sprintf("gobroke:client_lastmsg:%s", clientID)
	return rc.client.Del(rc.ctx, lastMsgKey).Err()
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
	key := fmt.Sprintf("gobroke:client_lastmsg:%s", client.GetUUID())
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

	key := fmt.Sprintf("gobroke:client_lastmsg:%s", clientID)
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
