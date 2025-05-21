// Package GoBroke provides a flexible message broker implementation for handling
// client-to-client and client-to-logic communication patterns. It supports
// different types of message routing, client management, and custom logic handlers.
package GoBroke

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/endpoint"
	brokeerrors "github.com/A13xB0/GoBroke/errors"
	"github.com/A13xB0/GoBroke/types"
)

type middlewareFunc func(types.Message) types.Message

// Broke represents a message broker instance that manages client connections,
// message routing, and custom logic handlers.
type Broke struct {
	endpoint           endpoint.Endpoint
	logic              map[types.LogicName]types.Logic
	clients            map[string]*clients.Client
	clientsMutex       sync.RWMutex
	sendQueue          chan types.Message
	receiveQueue       chan types.Message
	ctx                context.Context
	recvMiddlewareFunc []middlewareFunc
	sendMiddlewareFunc []middlewareFunc
	redis              *redisClient // Redis client for high availability
}

// New creates a new GoBroke instance with the specified endpoint and optional configuration.
// It returns an error if the endpoint is nil or if there are issues setting up message queues.
func New(endpoint endpoint.Endpoint, opts ...brokeOptsFunc) (*Broke, error) {

	//Get options
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}

	gb := &Broke{
		endpoint:     endpoint,
		logic:        make(map[types.LogicName]types.Logic),
		clients:      make(map[string]*clients.Client),
		receiveQueue: make(chan types.Message, o.channelSize),
		sendQueue:    make(chan types.Message, o.channelSize),
		ctx:          o.ctx,
	}
	// todo: Handle Errors
	if endpoint == nil {
		return nil, errors.Join(brokeerrors.ErrorCouldNotCreateServer, brokeerrors.ErrorNoEndpointProvided)
	}
	if err := endpoint.Sender(gb.sendQueue); err != nil {
		return nil, errors.Join(brokeerrors.ErrorCouldNotCreateServer, err)
	}
	if err := endpoint.Receiver(gb.receiveQueue); err != nil {
		return nil, errors.Join(brokeerrors.ErrorCouldNotCreateServer, err)
	}

	// Initialize Redis client if enabled
	if o.redis.Enabled {
		redisClient, err := newRedisClient(o.redis, gb, o.ctx)
		if err != nil {
			return nil, errors.Join(brokeerrors.ErrorCouldNotCreateServer, err)
		}
		gb.redis = redisClient
	}

	return gb, nil
}

// AddLogic adds a new logic handler to the GoBroke instance.
// It returns an error if a logic handler with the same name already exists.
func (broke *Broke) AddLogic(logic types.Logic) error {
	if _, ok := broke.logic[logic.Name()]; ok {
		return brokeerrors.ErrorLogicAlreadyExists
	}
	broke.logic[logic.Name()] = logic
	return nil
}

// RemoveLogic removes a logic handler from the GoBroke instance by its name.
// It returns nil even if the logic handler doesn't exist.
func (broke *Broke) RemoveLogic(name types.LogicName) error {
	if _, ok := broke.logic[name]; ok {
		delete(broke.logic, name)
	}
	return nil
}

// RegisterClient registers a new client in the GoBroke instance.
// This method should be called from the endpoint implementation.
// It returns an error if the client is already registered.
func (broke *Broke) RegisterClient(client *clients.Client) error {
	broke.clientsMutex.RLock()
	_, ok := broke.clients[client.GetUUID()]
	broke.clientsMutex.RUnlock()
	if ok {
		return brokeerrors.ErrorClientAlreadyExists
	}

	broke.clientsMutex.Lock()
	broke.clients[client.GetUUID()] = client
	broke.clientsMutex.Unlock()

	client.SetLastMessageNow()

	// Register client in Redis if enabled
	if broke.redis != nil {
		if err := broke.redis.registerClientInRedis(client); err != nil {
			// Log error but don't fail registration
			fmt.Printf("Error registering client in Redis: %v\n", err)
		}
		broke.redis.updateClientLastMessageTime(client)
	}

	return nil
}

// RemoveClient removes a client from the GoBroke instance and disconnects them
// from the endpoint. It can remove clients from both the local instance and from Redis.
// If the client is not found locally but exists in Redis, it will be removed from Redis.
func (broke *Broke) RemoveClient(client *clients.Client) error {
	clientID := client.GetUUID()

	// Check if client exists locally
	broke.clientsMutex.RLock()
	_, isLocalClient := broke.clients[clientID]
	broke.clientsMutex.RUnlock()

	// If client exists locally, remove it locally
	if isLocalClient {
		// Disconnect the client from the endpoint
		err := broke.endpoint.Disconnect(client)
		if err != nil {
			return errors.Join(brokeerrors.ErrorClientCouldNotBeDisconnected, err)
		}

		// Remove from local clients map
		broke.clientsMutex.Lock()
		delete(broke.clients, clientID)
		broke.clientsMutex.Unlock()

		// Unregister client from Redis if enabled
		if broke.redis != nil {
			if err := broke.redis.unregisterClientFromRedis(client); err != nil {
				// Log error but don't fail removal
				fmt.Printf("Error unregistering client from Redis: %v\n", err)
			}
		}

		return nil
	}

	// If client doesn't exist locally, check if it exists in Redis
	if broke.redis != nil && broke.redis.isClientOnOtherInstance(clientID) {
		// Remove client from Redis
		if err := broke.redis.unregisterClientFromRedisByID(clientID); err != nil {
			// Log error but don't fail removal
			fmt.Printf("Error unregistering remote client from Redis: %v\n", err)
		}
		return nil
	}

	// Client doesn't exist locally or in Redis
	return brokeerrors.ErrorClientDoesNotExist
}

// GetClient retrieves a client by their UUID.
// It returns the client instance and nil if found, or nil and an error if not found.
// If Redis is enabled and the client is not found locally, it checks if the client
// exists on another instance.
func (broke *Broke) GetClient(uuid string, localOnly ...bool) (*clients.Client, error) {
	// Check local clients first
	broke.clientsMutex.RLock()
	if client, ok := broke.clients[uuid]; ok {
		broke.clientsMutex.RUnlock()
		return client, nil
	}
	broke.clientsMutex.RUnlock()
	lo := false
	if len(localOnly) != 0 {
		lo = localOnly[0]
	}

	// If Redis is enabled, check if client exists on another instance
	if broke.redis != nil && !lo && broke.redis.isClientOnOtherInstance(uuid) {
		// Create a virtual client reference for cross-instance communication
		client := clients.New(clients.WithUUID(uuid))

		// Get the client's last message time from Redis
		lastMsgTime := broke.redis.getClientLastMessageTime(uuid)
		if !lastMsgTime.IsZero() {
			// Set the last message time on the virtual client
			client.SetLastMessage(lastMsgTime)
		}

		return client, nil
	}

	return nil, brokeerrors.ErrorClientDoesNotExist
}

// GetAllClients returns a slice containing all currently connected clients.
// If Redis is enabled, it also includes clients connected to other instances.
func (broke *Broke) GetAllClients(localOnly ...bool) []*clients.Client {
	// Get local clients
	broke.clientsMutex.RLock()
	var cl []*clients.Client
	for _, value := range broke.clients {
		cl = append(cl, value)
	}
	broke.clientsMutex.RUnlock()
	lo := false
	if len(localOnly) != 0 {
		lo = localOnly[0]
	}

	// If Redis is enabled, get clients from other instances
	if broke.redis != nil && !lo {
		remoteClientIDs, err := broke.redis.getRemoteClientIDs()
		if err != nil {
			// Log error but continue with local clients
			fmt.Printf("Error getting remote clients: %v\n", err)
		} else {
			// Create virtual client references for remote clients
			for _, clientID := range remoteClientIDs {
				client := clients.New(clients.WithUUID(clientID))

				// Get the client's last message time from Redis
				lastMsgTime := broke.redis.getClientLastMessageTime(clientID)
				if !lastMsgTime.IsZero() {
					// Set the last message time on the virtual client
					client.SetLastMessage(lastMsgTime)
				}

				cl = append(cl, client)
			}
		}
	}

	return cl
}

// SendMessage queues a message for processing by GoBroke.
// This method can be used to send messages to both logic handlers and clients.
// If the message is from a client, their last message timestamp is updated.
// If Redis is enabled and the message is for clients not on this instance,
// it will be published to Redis for routing to other instances.
func (broke *Broke) SendMessage(message types.Message) {
	for _, middleFn := range broke.sendMiddlewareFunc {
		message = middleFn(message)
	}
	if message.FromClient != nil {
		message.FromClient.SetLastMessageNow()
	}

	// Check if any target clients need Redis routing
	if broke.redis != nil && len(message.ToClient) > 0 {
		// Filter clients that are not on this instance
		var localClients []*clients.Client
		var needsRedis bool

		for _, client := range message.ToClient {
			// Check if client is local (has a real client object)
			broke.clientsMutex.RLock()
			_, isLocal := broke.clients[client.GetUUID()]
			broke.clientsMutex.RUnlock()

			if isLocal {
				localClients = append(localClients, client)
			} else {
				// This client needs Redis routing
				needsRedis = true
			}
		}

		// If some clients need Redis routing, publish the message
		if needsRedis && !message.FromRedis {
			// Don't wait for Redis publish to complete
			go func(msg types.Message) {
				if err := broke.redis.publishMessage(msg); err != nil {
					// Log error but continue
					fmt.Printf("Error publishing message to Redis: %v\n", err)
				}
			}(message)
		}

		// Update message with only local clients
		if len(localClients) < len(message.ToClient) {
			message.ToClient = localClients
		}
	}

	broke.receiveQueue <- message
}

// SendMessageQuickly sends a message directly to the endpoint for processing.
// This method should only be used for client-to-client communication as it
// bypasses logic handlers.
func (broke *Broke) SendMessageQuickly(message types.Message) {
	if message.FromRedis {
		return
	}
	for _, middleFn := range broke.sendMiddlewareFunc {
		message = middleFn(message)
	}
	message.SentQuickly = true
	// Check if any target clients need Redis routing
	if broke.redis != nil && len(message.ToClient) > 0 {
		// Filter clients that are not on this instance
		var localClients []*clients.Client
		var needsRedis bool

		for _, client := range message.ToClient {
			// Check if client is local (has a real client object)
			broke.clientsMutex.RLock()
			_, isLocal := broke.clients[client.GetUUID()]
			broke.clientsMutex.RUnlock()

			if isLocal {
				localClients = append(localClients, client)
			} else {
				// This client needs Redis routing
				needsRedis = true
			}
		}

		// If some clients need Redis routing, publish the message
		if needsRedis && !message.FromRedis {
			// Don't wait for Redis publish to complete
			go func(msg types.Message) {
				if err := broke.redis.publishMessage(msg); err != nil {
					// Log error but continue
					fmt.Printf("Error publishing message to Redis: %v\n", err)
				}
			}(message)
		}

		// Update message with only local clients
		if len(localClients) < len(message.ToClient) {
			message.ToClient = localClients
		}
	}
	broke.sendQueue <- message
}

// AttachReceiveMiddleware adds a middleware function to the receive message pipeline.
// Middleware functions are executed in the order they are attached and can modify
// or filter messages before they are processed by the broker.
//
// The middleware function receives a Message and returns a modified Message or nil
// if the message should be dropped from the pipeline.
func (broke *Broke) AttachReceiveMiddleware(mFunc middlewareFunc) {
	broke.recvMiddlewareFunc = append(broke.recvMiddlewareFunc, mFunc)
}

// AttachSendMiddleware adds a middleware function to the send message pipeline.
// Middleware functions are executed in the order they are attached and can modify
// or filter messages before they are sent to clients.
//
// The middleware function receives a Message and returns a modified Message or nil
// if the message should be dropped from the pipeline.
func (broke *Broke) AttachSendMiddleware(mFunc middlewareFunc) {
	broke.sendMiddlewareFunc = append(broke.sendMiddlewareFunc, mFunc)
}

// GetEndpoint returns the endpoint used by this broker.
// This can be useful for endpoint-specific operations.
func (broke *Broke) GetEndpoint() endpoint.Endpoint {
	return broke.endpoint
}

// Start begins processing messages in the GoBroke instance.
// It runs until the context is cancelled, at which point it closes
// all message queues and stops processing.
func (broke *Broke) Start() {
	broke.endpoint.Start(broke.ctx)
	for {
		select {
		case <-broke.ctx.Done():
			// Close Redis connection if enabled
			if broke.redis != nil {
				if err := broke.redis.close(); err != nil {
					// Log error but continue shutdown
					fmt.Printf("Error closing Redis connection: %v\n", err)
				}
			}
			close(broke.receiveQueue)
			close(broke.sendQueue)
			return
		case msg := <-broke.receiveQueue:
			if msg.SentQuickly {
				broke.sendQueue <- msg
			}
			//Default message state of accepted
			msg.State = types.ACCEPTED
			// Recv Middlware Func
			for _, middleFn := range broke.recvMiddlewareFunc {
				msg = middleFn(msg)
			}
			if msg.State != types.ACCEPTED {
				continue
			}
			if len(msg.ToClient) != 0 {
				broke.sendQueue <- msg
			}
			// Process message through registered logic handlers
			for _, logicName := range msg.ToLogic {
				if logicFn, ok := broke.logic[logicName]; ok {
					switch logicFn.Type() {
					case types.WORKER:
						if err := logicFn.RunLogic(msg); err != nil {
							// TODO: Implement error handling strategy
							continue
						}
					case types.DISPATCHED:
						go func(l types.Logic, m types.Message) {
							if err := l.RunLogic(m); err != nil {
								// TODO: Implement error handling strategy
							}
						}(logicFn, msg)
					case types.PASSIVE:
						// Passive logic handlers don't process messages
					}
				}
				// TODO: Consider logging when logic handler is not found
			}
		}
	}
}
