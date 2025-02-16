// Package message provides message creation and management functionality for the GoBroke system.
// It implements a flexible message creation system that supports various communication patterns:
//   - Client-to-client communication
//   - Client-to-logic communication
//   - Logic-to-client communication
//   - Logic-to-logic communication
//
// Messages can be created with optional metadata, tags, and unique identifiers.
// The package provides both standard and simplified creation functions to accommodate
// different use cases and maintain code clarity.
package message

import (
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/types"
)

// NewClientMessage creates a new message originating from a client.
// It initializes a message with the specified routing information and payload,
// along with optional configuration through the functional options pattern.
//
// Parameters:
//   - From: The client sending the message
//   - ToClients: Slice of client recipients for the message
//   - ToLogic: Slice of logic handlers that should process the message
//   - MessageRaw: Raw message payload as bytes (can be nil for empty messages)
//   - opts: Optional configuration functions for customizing the message
//
// The returned Message includes:
//   - Routing information (sender and recipients)
//   - Message payload
//   - Unique identifier (auto-generated or provided via options)
//   - Empty tags map for custom message tagging
//   - Optional metadata (if provided via options)
func NewClientMessage(From *clients.Client, ToClients []*clients.Client, ToLogic []types.LogicName, MessageRaw []byte, opts ...messageOptsFunc) types.Message {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return types.Message{
		ToClient:   ToClients,
		ToLogic:    ToLogic,
		FromClient: From,
		MessageRaw: MessageRaw,
		Metadata:   o.metadata,
		UUID:       o.uuid,
		Tags:       make(map[string]interface{}),
	}
}

// NewSimpleClientMessage creates a new message from a client to a single recipient.
// This is a convenience wrapper around NewClientMessage that simplifies the creation
// of messages with a single recipient, reducing the need for slice creation.
//
// Parameters:
//   - From: The client sending the message
//   - ToClient: Single client recipient (will be wrapped in a slice)
//   - ToLogic: Single logic handler to process the message (will be wrapped in a slice)
//   - MessageRaw: Raw message payload as bytes (can be nil for empty messages)
//   - opts: Optional configuration functions for customizing the message
//
// This function is equivalent to calling NewClientMessage with single-element slices,
// but provides a more ergonomic API for the common case of single-recipient messages.
func NewSimpleClientMessage(From *clients.Client, ToClient *clients.Client, ToLogic types.LogicName, MessageRaw []byte, opts ...messageOptsFunc) types.Message {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return types.Message{
		ToClient:   []*clients.Client{ToClient},
		ToLogic:    []types.LogicName{ToLogic},
		FromClient: From,
		MessageRaw: MessageRaw,
		Metadata:   o.metadata,
		UUID:       o.uuid,
		Tags:       make(map[string]interface{}),
	}
}

// NewLogicMessage creates a new message originating from a logic handler.
// It supports logic-to-client and logic-to-logic communication patterns,
// allowing logic handlers to send messages to clients or trigger other handlers.
//
// Parameters:
//   - From: The logic handler sending the message (identified by name)
//   - ToClients: Slice of client recipients for the message
//   - ToLogic: Slice of logic handlers that should process the message
//   - MessageRaw: Raw message payload as bytes (can be nil for empty messages)
//   - opts: Optional configuration functions for customizing the message
//
// The returned Message includes:
//   - Routing information (sender and recipients)
//   - Message payload
//   - Unique identifier (auto-generated or provided via options)
//   - Empty tags map for custom message tagging
//   - Optional metadata (if provided via options)
func NewLogicMessage(From types.LogicName, ToClients []*clients.Client, ToLogic []types.LogicName, MessageRaw []byte, opts ...messageOptsFunc) types.Message {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return types.Message{
		ToClient:   ToClients,
		ToLogic:    ToLogic,
		FromLogic:  From,
		MessageRaw: MessageRaw,
		Metadata:   o.metadata,
		UUID:       o.uuid,
		Tags:       make(map[string]interface{}),
	}
}

// NewSimpleLogicMessage creates a new message from a logic handler to a single recipient.
// This is a convenience wrapper around NewLogicMessage that simplifies the creation
// of messages with a single recipient, reducing the need for slice creation.
//
// Parameters:
//   - From: The logic handler sending the message (identified by name)
//   - ToClient: Single client recipient (will be wrapped in a slice)
//   - ToLogic: Single logic handler to process the message (will be wrapped in a slice)
//   - MessageRaw: Raw message payload as bytes (can be nil for empty messages)
//   - opts: Optional configuration functions for customizing the message
//
// This function is equivalent to calling NewLogicMessage with single-element slices,
// but provides a more ergonomic API for the common case of single-recipient messages.
// It's particularly useful for implementing request-response patterns between logic handlers.
func NewSimpleLogicMessage(From types.LogicName, ToClient *clients.Client, ToLogic types.LogicName, MessageRaw []byte, opts ...messageOptsFunc) types.Message {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return types.Message{
		ToClient:   []*clients.Client{ToClient},
		ToLogic:    []types.LogicName{ToLogic},
		FromLogic:  From,
		MessageRaw: MessageRaw,
		Metadata:   o.metadata,
		UUID:       o.uuid,
		Tags:       make(map[string]interface{}),
	}
}
