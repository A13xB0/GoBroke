// Package brokeerrors provides error definitions for the GoBroke message broker system.
// These errors are used throughout the system to indicate specific failure conditions
// and error states.
package brokeerrors

import "errors"

// Client-related errors
var (
	// ErrorClientAlreadyExists indicates an attempt to register a client that is already registered.
	ErrorClientAlreadyExists = errors.New("client already exists")

	// ErrorClientDoesNotExist indicates an attempt to access a client that is not registered.
	ErrorClientDoesNotExist = errors.New("client does not exist")

	// ErrorClientCouldNotBeDisconnected indicates a failure to properly disconnect a client.
	ErrorClientCouldNotBeDisconnected = errors.New("client could not be disconnected")
)

// Logic and processor-related errors
var (
	// ErrorLogicAlreadyExists indicates an attempt to register logic that is already registered.
	ErrorLogicAlreadyExists = errors.New("logic already exists")

	// ErrorCouldNotStartProcessor indicates a failure to start a message processor.
	ErrorCouldNotStartProcessor = errors.New("could not start processor")

	// ErrorProcessorAlreadyExists indicates an attempt to start a processor that is already running.
	ErrorProcessorAlreadyExists = errors.New("processor already exists")
)

// Server initialization errors
var (
	// ErrorCouldNotCreateServer indicates a failure during server creation.
	ErrorCouldNotCreateServer = errors.New("could not create server")

	// ErrorNoEndpointProvided indicates an attempt to create a server without an endpoint.
	ErrorNoEndpointProvided = errors.New("no endpoint provided")
)

// Message related errors
var (
	// Error tag does not exist
	ErrorTagDoesNotExist = errors.New("tag does not exist")
)
