package types

import (
	"github.com/A13xB0/GoBroke/clients"
)

type Message struct {
	ToClient   []*clients.Client
	ToLogic    []Logic
	FromClient *clients.Client
	FromLogic  Logic
	MessageRaw []byte
	Metadata   map[string]any
	UUID       string
}
