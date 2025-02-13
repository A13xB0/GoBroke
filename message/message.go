package message

import (
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/types"
)

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
