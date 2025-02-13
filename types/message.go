// Package types provides core type definitions for the GoBroke message broker system.
package types

import (
	"github.com/A13xB0/GoBroke/clients"
)

// Message represents a communication unit within the GoBroke system.
// It contains routing information, payload data, and metadata for message processing.
type Message struct {
	// ToClient is a slice of client recipients for this message
	ToClient []*clients.Client

	// ToLogic is a slice of logic handlers that should process this message
	ToLogic []Logic

	// FromClient identifies the client that originated this message, if any
	FromClient *clients.Client

	// FromLogic identifies the logic handler that originated this message, if any
	FromLogic Logic

	// MessageRaw contains the raw message payload as bytes
	MessageRaw []byte

	// Metadata holds additional key-value pairs associated with the message
	Metadata map[string]any

	// UUID uniquely identifies this message instance
	UUID string
}
