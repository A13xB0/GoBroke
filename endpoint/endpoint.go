package endpoint

import (
	"context"
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/message"
)

type Endpoint interface {
	Sender(chan message.Message) error
	Receiver(chan message.Message) error
	Disconnect(client *clients.Client) error
	Start(ctx context.Context)
}
