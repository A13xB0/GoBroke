package GoBroke

import (
	"context"
	"errors"
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/endpoint"
	"github.com/A13xB0/GoBroke/logic"
	"github.com/A13xB0/GoBroke/message"
)

type Broke struct {
	endpoint     endpoint.Endpoint
	logic        map[string]logic.Logic
	clients      map[string]*clients.Client
	sendQueue    chan message.Message
	receiveQueue chan message.Message
	ctx          context.Context
}

// New GoBroke instance
func New(endpoint endpoint.Endpoint, opts ...brokeOptsFunc) *Broke {

	//Get options
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}

	return &Broke{
		endpoint:     endpoint,
		logic:        make(map[string]logic.Logic),
		clients:      make(map[string]*clients.Client),
		receiveQueue: make(chan message.Message, o.channelSize),
		sendQueue:    make(chan message.Message, o.channelSize),
		ctx:          o.ctx,
	}
}

// AddLogic to the GoBroke instance
func (broke *Broke) AddLogic(logic logic.Logic) error {
	if _, ok := broke.logic[logic.Name()]; ok {
		return ErrorLogicAlreadyExists
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
	if _, ok := broke.clients[client.GetUUID()]; ok {
		return ErrorClientAlreadyExists
	}
	broke.clients[client.GetUUID()] = client
	return nil
}

// RemoveClient in the GoBroke instance (the equivalent of kicking someone)
func (broke *Broke) RemoveClient(client *clients.Client) error {
	if _, ok := broke.clients[client.GetUUID()]; !ok {

		return ErrorClientDoesNotExist
	}
	err := broke.endpoint.Disconnect(client)
	if err != nil {
		return errors.Join(ErrorClientCouldNotBeDisconnected, err)
	}
	delete(broke.clients, client.GetUUID())
	return nil
}

// GetClient by using the client uuid.
func (broke *Broke) GetClient(uuid string) (*clients.Client, error) {
	if client, ok := broke.clients[uuid]; ok {
		return client, nil
	}
	return nil, ErrorClientDoesNotExist
}

// GetAllClients gets all clients in a slice.
func (broke *Broke) GetAllClients() []*clients.Client {
	var clients []*clients.Client
	for _, value := range broke.clients {
		clients = append(clients, value)
	}
	return clients
}

// SendMessage will put a message to be processed by GoBroke, this can be used to send a message to logic or clients
func (broke *Broke) SendMessage(message message.Message) {
	if message.FromClient != nil {
		message.FromClient.SetLastMessageNow()
	}
	broke.receiveQueue <- message
}

// SendMessageQuickly will put a message to be processed by the endpoint directly, *this can be used to send a message to clients only*
func (broke *Broke) SendMessageQuickly(message message.Message) {
	broke.sendQueue <- message
}

// Start GoBroke
func (broke *Broke) Start() {
	for {
		select {
		case <-broke.ctx.Done():
			return
		case message := <-broke.receiveQueue:
			if len(message.ToClient) != 0 {
				broke.sendQueue <- message
			}
			for _, logicFn := range broke.logic {
				switch logicFn.Type() {
				case logic.WORKER:
					logicFn.RunLogic(message) //todo: handle errors
				case logic.DISPATCHED:
					go logicFn.RunLogic(message) //todo: handle errors
				case logic.PASSIVE:
				}
			}
		}
	}
}
