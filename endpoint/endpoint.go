// Package endpoint defines the interface for communication endpoints in the GoBroke system.
// Endpoints handle the actual message transmission between clients and the broker.
package endpoint

import (
	"context"

	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/types"
)

// Endpoint defines the interface that all communication endpoints must implement.
// An endpoint is responsible for managing client connections and message transmission.
type Endpoint interface {
	// Sender sets up the channel for outgoing messages from the broker to clients.
	// Returns an error if the sender channel cannot be established.
	Sender(chan types.Message) error

	// ReplyDirectly allows us to send a message directly to the endpoint without a channel in between.
	ReplyDirectly(msg types.Message) error

	// Receiver sets up the channel for incoming messages from clients to the broker.
	// Returns an error if the receiver channel cannot be established.
	Receiver(chan types.Message) error

	// Disconnect terminates a client's connection to the endpoint.
	// Returns an error if the client cannot be disconnected properly.
	Disconnect(client *clients.Client) error

	// Start begins the endpoint's message processing operations.
	// It runs until the provided context is cancelled.
	Start(ctx context.Context)
}
