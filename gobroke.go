package GoBroke

import "github.com/A13xB0/GoBroke/message"

type Broke struct {
	sender   Sender
	receiver Receiver
}

type Sender func(message message.Message)

type Receiver func() message.Message

func New(sender Sender, receiver Receiver) Broke {
	return Broke{
		sender:   sender,
		receiver: receiver,
	}
}
