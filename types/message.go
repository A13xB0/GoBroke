// Package types provides core type definitions for the GoBroke message broker system.
// It defines the fundamental data structures and interfaces that form the basis of
// message routing, processing, and state management within the system.
package types

import (
	"github.com/A13xB0/GoBroke/clients"
	brokeerrors "github.com/A13xB0/GoBroke/errors"
)

// MessageState represents the processing state of a message within the system.
// It is used by middleware to control message flow and handling.
type MessageState int

const (
	// ACCEPTED indicates the message should continue through the processing pipeline.
	// This is the default state for new messages.
	ACCEPTED MessageState = iota

	// REJECTED indicates the message should be dropped from the processing pipeline.
	// Middleware can set this state to prevent further message processing.
	REJECTED
)

// Message represents a communication unit within the GoBroke system.
// It encapsulates all information needed for message routing and processing,
// including sender and recipient information, payload data, and processing state.
// Messages can be created using the functions in the message package.
type Message struct {
	// ToClient is a slice of client recipients for this message
	ToClient []*clients.Client

	// ToLogic is a slice of logic handlers that should process this message
	ToLogic []LogicName

	// FromClient identifies the client that originated this message, if any
	FromClient *clients.Client

	// FromLogic identifies the logic handler that originated this message, if any
	FromLogic LogicName

	// MessageRaw contains the raw message payload as bytes
	MessageRaw []byte

	// Metadata holds additional key-value pairs associated with the message
	Metadata map[string]any

	// UUID uniquely identifies this message instance
	UUID string

	// State is used for middleware to reject or accept a message
	State MessageState

	// Tags is used for middlware to be able to add tags to the message
	Tags map[string]interface{}
}

// Accept marks the message as accepted for further processing.
// This is typically used by middleware to explicitly allow a message
// to continue through the processing pipeline.
func (m *Message) Accept() {
	m.State = ACCEPTED
}

// Reject marks the message as rejected, preventing further processing.
// This is typically used by middleware to filter out unwanted messages
// or implement access control.
func (m *Message) Reject() {
	m.State = REJECTED
}

// AddTag associates a key-value pair with the message.
// Tags can be used by middleware and logic handlers to attach
// arbitrary data to messages for processing or tracking purposes.
//
// Parameters:
//   - tag: The key for the tag
//   - value: The value to associate with the tag
func (m *Message) AddTag(tag string, value interface{}) {
	m.Tags[tag] = value
}

// GetTag retrieves the value associated with a tag.
// If the tag doesn't exist, it returns an error.
//
// Parameters:
//   - tag: The key for the tag to retrieve
//   - value: Placeholder parameter (unused)
//
// Returns:
//   - interface{}: The value associated with the tag
//   - error: ErrorTagDoesNotExist if the tag is not found
func (m *Message) GetTag(tag string, value interface{}) (interface{}, error) {
	if value, ok := m.Tags[tag]; ok {
		return value, nil
	}
	return nil, brokeerrors.ErrorTagDoesNotExist
}
