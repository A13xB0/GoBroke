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

// Broke represents a message broker instance that manages client connections,
// message routing, and custom logic handlers.
type Broke struct {
	endpoint     endpoint.Endpoint
	logic        map[string]types.Logic
	clients      map[string]*clients.Client
	clientsMutex sync.RWMutex
	sendQueue    chan types.Message
	receiveQueue chan types.Message
	ctx          context.Context
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
		logic:        make(map[string]types.Logic),
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
func (broke *Broke) RemoveLogic(name string) error {
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
	if message.FromClient != nil {
		message.FromClient.SetLastMessageNow()
	}
	broke.receiveQueue <- message
}

// SendMessageQuickly sends a message directly to the endpoint for processing.
// This method should only be used for client-to-client communication as it
// bypasses logic handlers.
func (broke *Broke) SendMessageQuickly(message types.Message) {
	broke.sendQueue <- message
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
			if len(msg.ToClient) != 0 {
				broke.sendQueue <- msg
			}
			for _, logicFn := range broke.logic {
				switch logicFn.Type() {
				case types.WORKER:
					logicFn.RunLogic(msg) //todo: handle errors
				case types.DISPATCHED:
					go logicFn.RunLogic(msg) //todo: handle errors
				case types.PASSIVE:
				}
			}
		}
	}
}
