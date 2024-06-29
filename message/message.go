package message

import (
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/logic"
)

type Message struct {
	ToClient   []*clients.Client
	ToLogic    []logic.Logic
	FromClient *clients.Client
	FromLogic  logic.Logic
	MessageRaw []byte
	Metadata   map[string]any
	UUID       string
}

func NewClientMessage(From *clients.Client, ToClients []*clients.Client, ToLogic []logic.Logic, MessageRaw []byte, opts ...messageOptsFunc) Message {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return Message{
		ToClient:   ToClients,
		ToLogic:    ToLogic,
		FromClient: From,
		MessageRaw: MessageRaw,
		Metadata:   o.metadata,
		UUID:       o.uuid,
	}
}

func NewSimpleClientMessage(From *clients.Client, ToClient *clients.Client, ToLogic logic.Logic, MessageRaw []byte, opts ...messageOptsFunc) Message {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return Message{
		ToClient:   []*clients.Client{ToClient},
		ToLogic:    []logic.Logic{ToLogic},
		FromClient: From,
		MessageRaw: MessageRaw,
		Metadata:   o.metadata,
		UUID:       o.uuid,
	}
}

func NewLogicMessage(From logic.Logic, ToClients []*clients.Client, ToLogic []logic.Logic, MessageRaw []byte, opts ...messageOptsFunc) Message {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return Message{
		ToClient:   ToClients,
		ToLogic:    ToLogic,
		FromLogic:  From,
		MessageRaw: MessageRaw,
		Metadata:   o.metadata,
		UUID:       o.uuid,
	}
}

func NewSimpleLogicMessage(From logic.Logic, ToClient *clients.Client, ToLogic logic.Logic, MessageRaw []byte, opts ...messageOptsFunc) Message {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return Message{
		ToClient:   []*clients.Client{ToClient},
		ToLogic:    []logic.Logic{ToLogic},
		FromLogic:  From,
		MessageRaw: MessageRaw,
		Metadata:   o.metadata,
		UUID:       o.uuid,
	}
}
