package GoBroke

import (
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
}

func New(endpoint endpoint.Endpoint) Broke {
	return Broke{
		endpoint:     endpoint,
		logic:        make(map[string]logic.Logic),
		clients:      make(map[string]*clients.Client),
		receiveQueue: make(chan message.Message),
		sendQueue:    make(chan message.Message),
	}
}

func (broke *Broke) AddLogic(name string, logic logic.Logic) error {
	if _, ok := broke.logic[name]; ok {
		return ErrorLogicAlreadyExists
	}
	broke.logic[name] = logic
	return nil
}

func (broke *Broke) RemoveLogic(name string) error {
	if _, ok := broke.logic[name]; ok {
		delete(broke.logic, name)
	}
	return nil
}

func (broke *Broke) registerClient(client *clients.Client) error {
	if _, ok := broke.clients[client.GetUUID()]; ok {
		return ErrorClientAlreadyExists
	}
	broke.clients[client.GetUUID()] = client
	return nil
}

func (broke *Broke) removeClient(client *clients.Client) error {
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

// SendMessage will put a message to be processed by GoBroke, this can be used to send a message to logic or clients
func (broke *Broke) SendMessage(message message.Message) {
	broke.receiveQueue <- message
}

// SendMessageQuickly will put a message to be processed by the endpoint directly, *this can be used to send a message to clients only*
func (broke *Broke) SendMessageQuickly(message message.Message) {
	broke.sendQueue <- message
}

// Start GoBroke
func (broke *Broke) Start() error {
	for message := range broke.receiveQueue {
		if len(message.ToClient) != 0 {
			broke.sendQueue <- message
		}
		for _, logicFn := range broke.logic {
			switch logicFn.Type() {
			case logic.WORKER:
				logicFn.RunLogic(message) //todo: handle errors
			case logic.DISPATCHED:
				go logicFn.RunLogic(message) //todo: handle errors
			}
		}
	}
	return nil
}
