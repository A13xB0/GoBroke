package GoBroke

import "errors"

var (
	ErrorLogicAlreadyExists           = errors.New("Logic already exists")
	ErrorCouldNotStartProcessor       = errors.New("Could not start processor")
	ErrorProcessorAlreadyExists       = errors.New("Processor already exists")
	ErrorClientAlreadyExists          = errors.New("Client already exists")
	ErrorClientDoesNotExist           = errors.New("Client does not exist")
	ErrorClientCouldNotBeDisconnected = errors.New("Client could not be disconnected")
)
