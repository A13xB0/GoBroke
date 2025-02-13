package endpoint

import (
	"context"

	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/types"
)

type Endpoint interface {
	Sender(chan types.Message) error
	Receiver(chan types.Message) error
	Disconnect(client *clients.Client) error
	Start(ctx context.Context)
}
