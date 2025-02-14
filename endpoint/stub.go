// Package endpoint provides interfaces and implementations for communication endpoints.
package endpoint

import (
	"context"
	"sync"

	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/types"
	"github.com/google/uuid"
)

// StubEndpoint provides a basic in-memory implementation of the Endpoint interface.
// It's useful for testing and as a reference implementation.
type StubEndpoint struct {
	senderChan   chan types.Message
	receiverChan chan types.Message
	clients      map[string]*clients.Client
	clientsMutex sync.RWMutex
}

// NewStubEndpoint creates a new instance of StubEndpoint.
// It initializes the necessary channels and client storage.
func NewStubEndpoint() *StubEndpoint {
	return &StubEndpoint{
		clients: make(map[string]*clients.Client),
	}
}

// Sender implements the Endpoint interface by setting up the channel
// for sending messages to clients.
func (s *StubEndpoint) Sender(ch chan types.Message) error {
	s.senderChan = ch
	return nil
}

// Receiver implements the Endpoint interface by setting up the channel
// for receiving messages from clients.
func (s *StubEndpoint) Receiver(ch chan types.Message) error {
	s.receiverChan = ch
	return nil
}

// Disconnect implements the Endpoint interface by removing the client
// from the endpoint's client map.
func (s *StubEndpoint) Disconnect(client *clients.Client) error {
	s.clientsMutex.Lock()
	defer s.clientsMutex.Unlock()
	delete(s.clients, client.GetUUID())
	return nil
}

// Start implements the Endpoint interface. In this stub implementation,
// it simply waits for context cancellation.
func (s *StubEndpoint) Start(ctx context.Context) {
	<-ctx.Done()
}

// SimulateClientConnection creates a new client and simulates its connection
// to the endpoint. This is useful for testing.
func (s *StubEndpoint) SimulateClientConnection() *clients.Client {
	client := clients.New(clients.WithUUID(uuid.New().String()))
	s.clientsMutex.Lock()
	s.clients[client.GetUUID()] = client
	s.clientsMutex.Unlock()
	return client
}

// SimulateClientMessage simulates a client sending a message through the endpoint.
// This is useful for testing message flow.
func (s *StubEndpoint) SimulateClientMessage(from *clients.Client, to []*clients.Client, message []byte) {
	if s.receiverChan != nil {
		s.receiverChan <- types.Message{
			FromClient: from,
			ToClient:   to,
			MessageRaw: message,
		}
	}
}
