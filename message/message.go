// Package message provides functions for creating and managing messages within the GoBroke system.
// It offers both simple and advanced message creation functions for client-to-client,
// client-to-logic, and logic-to-logic communication patterns.
package message

import (
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/types"
)

// NewClientMessage creates a new message originating from a client.
// Parameters:
//   - From: The client sending the message
//   - ToClients: Slice of client recipients
//   - ToLogic: Slice of logic handlers to process the message
//   - MessageRaw: Raw message payload as bytes
//   - opts: Optional message configuration functions
//
// Returns a types.Message configured with the provided parameters and options.
func NewClientMessage(From *clients.Client, ToClients []*clients.Client, ToLogic []types.Logic, MessageRaw []byte, opts ...messageOptsFunc) types.Message {
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
	}
}

// NewSimpleClientMessage creates a new message from a client to a single recipient.
// This is a convenience wrapper around NewClientMessage for single-recipient cases.
//
// Parameters:
//   - From: The client sending the message
//   - ToClient: The single client recipient
//   - ToLogic: Single logic handler to process the message
//   - MessageRaw: Raw message payload as bytes
//   - opts: Optional message configuration functions
//
// Returns a types.Message configured for single-recipient delivery.
func NewSimpleClientMessage(From *clients.Client, ToClient *clients.Client, ToLogic types.Logic, MessageRaw []byte, opts ...messageOptsFunc) types.Message {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return types.Message{
		ToClient:   []*clients.Client{ToClient},
		ToLogic:    []types.Logic{ToLogic},
		FromClient: From,
		MessageRaw: MessageRaw,
		Metadata:   o.metadata,
		UUID:       o.uuid,
	}
}

// NewLogicMessage creates a new message originating from a logic handler.
// Parameters:
//   - From: The logic handler sending the message
//   - ToClients: Slice of client recipients
//   - ToLogic: Slice of logic handlers to process the message
//   - MessageRaw: Raw message payload as bytes
//   - opts: Optional message configuration functions
//
// Returns a types.Message configured with the provided parameters and options.
func NewLogicMessage(From types.Logic, ToClients []*clients.Client, ToLogic []types.Logic, MessageRaw []byte, opts ...messageOptsFunc) types.Message {
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
	}
}

// NewSimpleLogicMessage creates a new message from a logic handler to a single recipient.
// This is a convenience wrapper around NewLogicMessage for single-recipient cases.
//
// Parameters:
//   - From: The logic handler sending the message
//   - ToClient: The single client recipient
//   - ToLogic: Single logic handler to process the message
//   - MessageRaw: Raw message payload as bytes
//   - opts: Optional message configuration functions
//
// Returns a types.Message configured for single-recipient delivery.
func NewSimpleLogicMessage(From types.Logic, ToClient *clients.Client, ToLogic types.Logic, MessageRaw []byte, opts ...messageOptsFunc) types.Message {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return types.Message{
		ToClient:   []*clients.Client{ToClient},
		ToLogic:    []types.Logic{ToLogic},
		FromLogic:  From,
		MessageRaw: MessageRaw,
		Metadata:   o.metadata,
		UUID:       o.uuid,
	}
}
