package message

import (
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/logic"
)

type constraints interface {
	*logic.Logic | *clients.Client
}

type Message struct {
	ToClient   []clients.Client
	ToLogic    []logic.Logic
	From       any
	MessageRaw []byte
	Metadata   map[string]any
	UUID       string
}

func NewMessage[T constraints](From T, ToClients []clients.Client, ToLogic []logic.Logic, MessageRaw []byte, opts ...messageOptsFunc) Message {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return Message{
		ToClient:   ToClients,
		ToLogic:    ToLogic,
		From:       From,
		MessageRaw: MessageRaw,
		Metadata:   o.metadata,
		UUID:       o.uuid,
	}
}

func NewSimpleMessage[T constraints](From T, ToClient clients.Client, ToLogic logic.Logic, MessageRaw []byte, opts ...messageOptsFunc) Message {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return Message{
		ToClient:   []clients.Client{ToClient},
		ToLogic:    []logic.Logic{ToLogic},
		From:       From,
		MessageRaw: MessageRaw,
		Metadata:   o.metadata,
		UUID:       o.uuid,
	}
}
