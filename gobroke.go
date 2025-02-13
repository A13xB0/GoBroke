package GoBroke

import (
	"context"
	"errors"
	"sync"

	brokeerrors "github.com/A13xB0/GoBroke/broke-errors"
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/endpoint"
	"github.com/A13xB0/GoBroke/types"
)

type Broke struct {
	endpoint     endpoint.Endpoint
	logic        map[string]types.Logic
	clients      map[string]*clients.Client
	clientsMutex sync.RWMutex
	sendQueue    chan types.Message
	receiveQueue chan types.Message
	ctx          context.Context
}

// New GoBroke instance
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

// AddLogic to the GoBroke instance
func (broke *Broke) AddLogic(logic types.Logic) error {
	if _, ok := broke.logic[logic.Name()]; ok {
		return brokeerrors.ErrorLogicAlreadyExists
	}
	broke.logic[logic.Name()] = logic
	return nil
}

// RemoveLogic from the GoBroke instance
func (broke *Broke) RemoveLogic(name string) error {
	if _, ok := broke.logic[name]; ok {
		delete(broke.logic, name)
	}
	return nil
}

// RegisterClient in the GoBroke instance. This should be run from the endpoint.
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

// RemoveClient in the GoBroke instance (the equivalent of kicking someone)
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

// GetClient by using the client uuid.
func (broke *Broke) GetClient(uuid string) (*clients.Client, error) {
	broke.clientsMutex.RLock()
	defer broke.clientsMutex.RUnlock()
	if client, ok := broke.clients[uuid]; ok {
		return client, nil
	}
	return nil, brokeerrors.ErrorClientDoesNotExist
}

// GetAllClients gets all clients in a slice.
func (broke *Broke) GetAllClients() []*clients.Client {
	broke.clientsMutex.RLock()
	defer broke.clientsMutex.RUnlock()
	var cl []*clients.Client
	for _, value := range broke.clients {
		cl = append(cl, value)
	}
	return cl
}

// SendMessage will put a message to be processed by GoBroke, this can be used to send a message to logic or clients
func (broke *Broke) SendMessage(message types.Message) {
	if message.FromClient != nil {
		message.FromClient.SetLastMessageNow()
	}
	broke.receiveQueue <- message
}

// SendMessageQuickly will put a message to be processed by the endpoint directly, *this can be used to send a message to clients only*
func (broke *Broke) SendMessageQuickly(message types.Message) {
	broke.sendQueue <- message
}

// Start GoBroke
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
