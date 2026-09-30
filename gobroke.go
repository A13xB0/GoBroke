// Package GoBroke provides a flexible message broker implementation for handling
// client-to-client and client-to-logic communication patterns. It supports
// different types of message routing, client management, and custom logic handlers.
package GoBroke

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
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
	onLogicPanic       func(types.LogicName, types.Message, any, string)
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
		onLogicPanic: o.OnLogicPanic,
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

	client.SetLastMessageNow()

	return nil
}

// RemoveClient removes a client from the GoBroke instance and disconnects them
// from the endpoint.
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

		return nil
	}

	return brokeerrors.ErrorClientDoesNotExist
}

// GetClient retrieves a client by their UUID.
// It returns the client instance and nil if found, or nil and an error if not found.
//
// The localOnly parameter is ignored (Redis support was removed); it is kept for source compatibility.
func (broke *Broke) GetClient(uuid string, localOnly ...bool) (*clients.Client, error) {
	_ = localOnly
	broke.clientsMutex.RLock()
	defer broke.clientsMutex.RUnlock()
	if client, ok := broke.clients[uuid]; ok {
		return client, nil
	}
	return nil, brokeerrors.ErrorClientDoesNotExist
}

// GetAllClients returns a slice containing all currently connected clients.
//
// The localOnly parameter is ignored (Redis support was removed); it is kept for source compatibility.
func (broke *Broke) GetAllClients(localOnly ...bool) []*clients.Client {
	_ = localOnly
	broke.clientsMutex.RLock()
	defer broke.clientsMutex.RUnlock()
	cl := make([]*clients.Client, 0, len(broke.clients))
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
	message.SentQuickly = true

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
						if err := broke.runLogicRecover(logicName, msg, logicFn); err != nil {
							// TODO: Implement error handling strategy
							continue
						}
					case types.DISPATCHED:
						go func(name types.LogicName, l types.Logic, m types.Message) {
							if err := broke.runLogicRecover(name, m, l); err != nil {
								// TODO: Implement error handling strategy
							}
						}(logicName, logicFn, msg)
					case types.PASSIVE:
						// Passive logic handlers don't process messages
					}
				}
			}
		}
	}
}

// runLogicRecover runs l.RunLogic(msg). If RunLogic panics, the panic is recovered,
// onLogicPanic or stderr is used for logging, and err is nil. Otherwise err is the return value of RunLogic.
func (broke *Broke) runLogicRecover(logicName types.LogicName, msg types.Message, l types.Logic) (err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			if broke.onLogicPanic != nil {
				broke.onLogicPanic(logicName, msg, r, stack)
			} else {
				_, _ = fmt.Fprintf(os.Stderr, "logic panic logic=%s recovered=%v\n%s\n", logicName, r, stack)
			}
		}
	}()
	return l.RunLogic(msg)
}
