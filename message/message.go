package message

import (
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/logic"
)

type Message struct {
	ToClient   []clients.Client
	ToLogic    []logic.Logic
	From       string
	MessageRaw []byte
	Metadata   map[string]any
}
