package brokeerrors

import "errors"

var (
	ErrorLogicAlreadyExists           = errors.New("logic already exists")
	ErrorCouldNotStartProcessor       = errors.New("could not start processor")
	ErrorProcessorAlreadyExists       = errors.New("processor already exists")
	ErrorClientAlreadyExists          = errors.New("client already exists")
	ErrorClientDoesNotExist           = errors.New("client does not exist")
	ErrorClientCouldNotBeDisconnected = errors.New("client could not be disconnected")
)

var (
	ErrorCouldNotCreateServer = errors.New("could not create server")
	ErrorNoEndpointProvided   = errors.New("no endpoint provided")
)
