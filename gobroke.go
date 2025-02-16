// Package GoBroke provides a flexible message broker implementation for handling
// client-to-client and client-to-logic communication patterns. It supports
// different types of message routing, client management, and custom logic handlers.
package GoBroke

import (
	"context"
	"errors"
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
	return nil
}

// RemoveClient removes a client from the GoBroke instance and disconnects them
// from the endpoint. It returns an error if the client doesn't exist or if
// the disconnection fails.
func (broke *Broke) RemoveClient(client *clients.Client) error {
	broke.clientsMutex.RLock()
	_, ok := broke.clients[client.GetUUID()]
	broke.clientsMutex.RUnlock()
	if !ok {
		return brokeerrors.ErrorClientDoesNotExist
	}
	err := broke.endpoint.Disconnect(client)
	if err != nil {
		return errors.Join(brokeerrors.ErrorClientCouldNotBeDisconnected, err)
	}

	broke.clientsMutex.Lock()
	delete(broke.clients, client.GetUUID())
	broke.clientsMutex.Unlock()
	return nil
}

// GetClient retrieves a client by their UUID.
// It returns the client instance and nil if found, or nil and an error if not found.
func (broke *Broke) GetClient(uuid string) (*clients.Client, error) {
	broke.clientsMutex.RLock()
	defer broke.clientsMutex.RUnlock()
	if client, ok := broke.clients[uuid]; ok {
		return client, nil
	}
	return nil, brokeerrors.ErrorClientDoesNotExist
}

// GetAllClients returns a slice containing all currently connected clients.
func (broke *Broke) GetAllClients() []*clients.Client {
	broke.clientsMutex.RLock()
	defer broke.clientsMutex.RUnlock()
	var cl []*clients.Client
	for _, value := range broke.clients {
		cl = append(cl, value)
	}
	return cl
}

// SendMessage queues a message for processing by GoBroke.
// This method can be used to send messages to both logic handlers and clients.
// If the message is from a client, their last message timestamp is updated.
func (broke *Broke) SendMessage(message types.Message) {
	for _, middleFn := range broke.sendMiddlewareFunc {
		message = middleFn(message)
	}
	if message.FromClient != nil {
		message.FromClient.SetLastMessageNow()
	}
	broke.receiveQueue <- message
}

// SendMessageQuickly sends a message directly to the endpoint for processing.
// This method should only be used for client-to-client communication as it
// bypasses logic handlers.
func (broke *Broke) SendMessageQuickly(message types.Message) {
	for _, middleFn := range broke.sendMiddlewareFunc {
		message = middleFn(message)
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

// Start begins processing messages in the GoBroke instance.
// It runs until the context is cancelled, at which point it closes
// all message queues and stops processing.
func (broke *Broke) Start() {
	for {
		select {
		case <-broke.ctx.Done():
			close(broke.receiveQueue)
			close(broke.sendQueue)
			return
		case msg := <-broke.receiveQueue:
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
